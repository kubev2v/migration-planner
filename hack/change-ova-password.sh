#!/bin/bash

set -euo pipefail

if [[ $# -ne 1 ]]; then
	echo "Usage: $0 <path to discovery .ova>"
	exit 1
fi

if [[ ! -f $1 ]]; then
	echo "ERROR: Discovery OVA not found at $1"
	exit 1
fi

# Convert to absolute path
DISCOVERY_OVA_HOST_PATH="$(realpath "$1")"
DISCOVERY_OVA_HOST_DIR=$(dirname "$DISCOVERY_OVA_HOST_PATH")
WORK_DIR=$(mktemp -d --tmpdir="$DISCOVERY_OVA_HOST_DIR")

function cleanup() {
	if [[ -d "$WORK_DIR" ]]; then
		echo "Cleaning up temporary files..."
		rm -rf "$WORK_DIR"
		echo "Cleanup complete"
	fi
}
trap cleanup EXIT

function COREOS_INSTALLER() {
	podman run -v "$WORK_DIR":/data:Z --rm quay.io/coreos/coreos-installer:release "$@"
}

OVA_NAME=$(basename "$DISCOVERY_OVA_HOST_PATH" .ova)

# Extract OVA
echo "Extracting OVA file..."
tar -xf "$DISCOVERY_OVA_HOST_PATH" -C "$WORK_DIR"

# Verify required files exist
if [[ ! -f "$WORK_DIR/MigrationAssessment.iso" ]]; then
	echo "ERROR: MigrationAssessment.iso not found in OVA"
	exit 1
fi

# Container paths
DISCOVERY_ISO_PATH=/data/MigrationAssessment.iso
DISCOVERY_ISO_WITH_PASSWORD=/data/MigrationAssessment_modified.iso

# Prompt
read -rsp 'Please enter the password to be used by the "core" user: ' pw
echo ''
USER_PASSWORD=$(openssl passwd -6 --stdin <<<"$pw")
unset pw

# Transform original ignition
TRANSFORMED_IGNITION_PATH=$(mktemp --tmpdir="$WORK_DIR")
TRANSFORMED_IGNITION_NAME=$(basename "$TRANSFORMED_IGNITION_PATH")
COREOS_INSTALLER iso ignition show "$DISCOVERY_ISO_PATH" | jq --arg pass "$USER_PASSWORD" '.passwd.users[0].passwordHash = $pass' >"$TRANSFORMED_IGNITION_PATH"

# Generate new ISO
echo "Modifying ISO with new password..."
COREOS_INSTALLER iso customize --output "$DISCOVERY_ISO_WITH_PASSWORD" --force "$DISCOVERY_ISO_PATH" --live-ignition /data/"$TRANSFORMED_IGNITION_NAME"

# Replace original ISO with modified one
mv "$WORK_DIR/MigrationAssessment_modified.iso" "$WORK_DIR/MigrationAssessment.iso"

# Output path
DISCOVERY_OVA_WITH_PASSWORD_HOST="$DISCOVERY_OVA_HOST_DIR/${OVA_NAME}_with_password.ova"

if [[ -f "$DISCOVERY_OVA_WITH_PASSWORD_HOST" ]]; then
	echo "ERROR: $DISCOVERY_OVA_WITH_PASSWORD_HOST already exists"
	echo "Would you like to overwrite it? [y/N]"
	read -r SHOULD_OVERWRITE
	if [[ "$SHOULD_OVERWRITE" != "y" ]]; then
		echo "Exiting"
		exit 1
	fi
	rm -f "$DISCOVERY_OVA_WITH_PASSWORD_HOST"
fi

# Repackage OVA
echo "Repackaging OVA..."
cd "$WORK_DIR"
tar -cf "$DISCOVERY_OVA_WITH_PASSWORD_HOST" MigrationAssessment.ovf MigrationAssessment.iso persistence-disk.vmdk

echo ""
echo "Success! Created OVA with your password in \"$DISCOVERY_OVA_WITH_PASSWORD_HOST\""
echo "Login username is \"core\""
