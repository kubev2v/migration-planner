#!/bin/bash

# Fail on unset variables and errors
set -euo pipefail

# Default values
readonly DEFAULT_WORK_DIR="/tmp/iso_builder"

# Global variables
export DIR_PATH=""
export ISO_FILE_PATH=""
export OUTPUT_FILE_PATH=""
export AGENT_IMAGE=""
export AGENT_TAG=""
export REGISTRY=""
export ORG=""
export AGENT_IMAGE_NAME=""
export DRY_RUN=false

# Colors for output
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly NC='\033[0m' # No Color

#
# Logging functions
#
log_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

#
# Utility functions
#
cleanup() {
    local exit_code=$?

    # Unmount if mounted
    if mountpoint -q "${rhcos_mnt_dir:-}" 2>/dev/null; then
        $SUDO umount "${rhcos_mnt_dir}" || true
    fi

    # Clean up temporary directories if not in dry run mode
    if [[ "$DRY_RUN" != "true" && -n "${work_dir:-}" && -d "${work_dir}" ]]; then
        rm -rf "${work_dir}" || true
    fi

    if [[ $exit_code -ne 0 ]]; then
        log_error "Script failed with exit code $exit_code"
    fi

    exit $exit_code
}

# Set up cleanup trap
trap cleanup EXIT INT TERM

#
# Parse command line arguments
#
parse_arguments() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --iso-file)
                ISO_FILE_PATH="$2"
                shift 2
                ;;
            --dir)
                DIR_PATH="$2"
                shift 2
                ;;
            --output-file)
                OUTPUT_FILE_PATH="$2"
                shift 2
                ;;
            --agent-image)
                AGENT_IMAGE="$2"
                shift 2
                ;;
            --agent-tag)
                AGENT_TAG="$2"
                shift 2
                ;;
            --dry-run)
                DRY_RUN=true
                shift
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                usage
                exit 1
                ;;
        esac
    done
}

#
# Show usage information
#
usage() {
    cat << EOF
Usage: $(basename "$0") [OPTIONS]

Agent-based Installer OVE ISO Builder

This script creates an agent-based installer OVE ISO by extracting RHCOS ISO contents,
adding agent installer artifacts, and creating a bootable hybrid ISO image.

OPTIONS:
    --iso-file PATH         Path to the RHCOS ISO file (required)
    --dir PATH              Working directory path (default: $DEFAULT_WORK_DIR)
    --output-file PATH      Full path to the output ISO file (required)
    --agent-image NAME      Agent image name (required)
    --agent-tag TAG         Agent image tag (required)
    --dry-run               Show what would be done without executing
    -h, --help              Show this help message

ENVIRONMENT VARIABLES:
    ISO_FILE_PATH           Path to the RHCOS ISO file (overridden by --iso-file)
    DIR_PATH                Working directory path (overridden by --dir)
    OUTPUT_FILE_PATH        Full path to the output ISO file (overridden by --output-file)
    AGENT_IMAGE             Agent image name (overridden by --agent-image)
    AGENT_TAG               Agent image tag (overridden by --agent-tag)

EXAMPLES:
    # Basic usage with all required parameters
    $(basename "$0") --iso-file /path/to/rhcos.iso --output-file /path/to/agent.iso --agent-image docker.io/myorg/my-agent --agent-tag latest

    # Specify custom working directory
    $(basename "$0") --iso-file /path/to/rhcos.iso --output-file /path/to/agent.iso --agent-image docker.io/myorg/my-agent --agent-tag latest --dir /tmp/work

    # Dry run to preview operations
    $(basename "$0") --iso-file /path/to/rhcos.iso --output-file /path/to/agent.iso --agent-image docker.io/myorg/my-agent --agent-tag latest --dry-run

OUTPUTS:
    The script will create the ISO file at the specified --output-file path

EOF
}

function setup_vars() {
    # Set default directory if not provided via parameter
    if [[ -z "$DIR_PATH" ]]; then
        DIR_PATH="$DEFAULT_WORK_DIR"
    fi

    # Validate required parameters
    if [[ -z "$ISO_FILE_PATH" ]]; then
        log_error "ISO_FILE_PATH is required. Please provide --iso-file parameter."
        log_error "Use --help for usage information."
        exit 1
    fi

    if [[ -z "$OUTPUT_FILE_PATH" ]]; then
        log_error "OUTPUT_FILE_PATH is required. Please provide --output-file parameter."
        log_error "Use --help for usage information."
        exit 1
    fi

    if [[ -z "$AGENT_IMAGE" ]]; then
        log_error "AGENT_IMAGE is required. Please provide --agent-image parameter."
        log_error "Use --help for usage information."
        exit 1
    fi

    if [[ -z "$AGENT_TAG" ]]; then
        log_error "AGENT_TAG is required. Please provide --agent-tag parameter."
        log_error "Use --help for usage information."
        exit 1
    fi


    if [[ ! -f "$ISO_FILE_PATH" ]]; then
        log_error "ISO file not found at: $ISO_FILE_PATH"
        exit 1
    fi

    # Validate ISO file is readable
    if [[ ! -r "$ISO_FILE_PATH" ]]; then
        log_error "ISO file is not readable: $ISO_FILE_PATH"
        exit 1
    fi

    # Get absolute paths
    ISO_FILE_PATH="$(realpath "$ISO_FILE_PATH")"
    DIR_PATH="$(realpath "$DIR_PATH")"
    OUTPUT_FILE_PATH="$(realpath "$OUTPUT_FILE_PATH")"

    # Create output directory if it doesn't exist
    local output_dir="$(dirname "$OUTPUT_FILE_PATH")"
    mkdir -p "$output_dir"

    ove_dir="${DIR_PATH}/ove"
    rhcos_work_dir="${DIR_PATH}"
    rhcos_mnt_dir="${rhcos_work_dir}/isomnt"

    # Create directories
    mkdir -p "${DIR_PATH}" "${rhcos_work_dir}"

    work_dir="${ove_dir}/work"
    output_dir="${ove_dir}/output"
    agent_ove_iso="${output_dir}/agent.iso"

    mkdir -p "${output_dir}"

    log_info "Working directory: $DIR_PATH"
    log_info "Output file: $OUTPUT_FILE_PATH"
    log_info "ISO file: $ISO_FILE_PATH"
    log_info "Agent image: ${AGENT_IMAGE}:${AGENT_TAG}"
}

function extract_live_iso() {
    if [[ -d "${rhcos_mnt_dir}" ]]; then
        log_info "Reusing existing extracted ISO contents at: ${rhcos_mnt_dir}"
    else
        log_info "Extracting ISO contents from: ${ISO_FILE_PATH}"
        mkdir -p "${rhcos_mnt_dir}"

        # Mount the ISO when not in a container
        if [[ "$rhcos_work_dir" != "/" ]]; then
            $SUDO mount -o loop "${ISO_FILE_PATH}" "${rhcos_mnt_dir}"
        fi
    fi

    if [[ -d "${work_dir}" ]]; then
        log_info "Reusing existing work directory: ${work_dir}"
    else
        mkdir -p "${work_dir}"
        if [[ "$rhcos_work_dir" == "/" ]]; then
            # Use osirrox to extract the ISO without mounting it
            $SUDO osirrox -indev "${ISO_FILE_PATH}" -extract / "${rhcos_mnt_dir}"
        fi
        log_info "Copying extracted RHCOS ISO contents to work directory"
        $SUDO rsync -aH --info=progress2 "${rhcos_mnt_dir}/" "${work_dir}/"
        $SUDO chown -R "$(whoami):$(whoami)" "${work_dir}/"

        if mountpoint -q "${rhcos_mnt_dir}"; then
            $SUDO umount "${rhcos_mnt_dir}"
        fi
    fi

    # Extract volume label
    volume_label=$(xorriso -indev "${ISO_FILE_PATH}" -toc 2>/dev/null | awk -F',' '/ISO session/ {print $4}' | xargs)
}

function wait_for_image_availability() {
    local pull_spec="$1"
    local timeout=1800  # 30 minutes in seconds
    local check_interval=5  # 5 seconds
    local elapsed=0

    log_info "Waiting for image to be available: ${pull_spec}"
    log_info "Timeout: ${timeout}s (30 minutes), checking every ${check_interval}s"

    while [[ $elapsed -lt $timeout ]]; do
        if $SUDO skopeo inspect docker://"${pull_spec}" >/dev/null 2>&1; then
            log_success "Image is available: ${pull_spec}"
            return 0
        fi

        log_info "Image not yet available, retrying in ${check_interval}s... (elapsed: ${elapsed}s/${timeout}s)"
        sleep "${check_interval}"
        elapsed=$((elapsed + check_interval))
    done

    log_error "Timeout reached (30 minutes). Image not available: ${pull_spec}"
    exit 1
}

function setup_agent_artifacts() {
    local image_dir="${work_dir}/images"
    local pull_spec="${AGENT_IMAGE}:${AGENT_TAG}"
    local image_tar="${image_dir}/migration-planner-agent.tar"

    if [[ -f "${image_tar}" ]]; then
        log_info "Reusing existing agent image: ${image_tar}"
    else
        mkdir -p "${image_dir}"

        if [[ "$DRY_RUN" == "true" ]]; then
            log_info "[DRY RUN] Would wait for and pull: ${pull_spec}"
            return 0
        fi

        # Wait for image to be available with timeout
        wait_for_image_availability "${pull_spec}"

        log_info "Pulling agent image: ${pull_spec}"
        if ! $SUDO skopeo copy -q docker://"${pull_spec}" oci-archive:"${image_tar}"; then
            log_error "Failed to pull agent image: ${pull_spec}"
            exit 1
        fi

        log_success "Successfully pulled agent image"
    fi
}

function create_ove_iso() {
    if [[ -f "${agent_ove_iso}" ]]; then
        log_info "Reusing existing OVE ISO: ${agent_ove_iso}"
        return 0
    fi

    local boot_image="${work_dir}/images/efiboot.img"
    if [[ ! -f "${boot_image}" ]]; then
        log_error "Boot image not found: ${boot_image}"
        log_error "This might indicate an issue with the RHCOS ISO extraction"
        exit 1
    fi

    local size=$(stat --format="%s" "${boot_image}")
    local boot_load_size=$(( (size + 2047) / 2048 ))

    log_info "Creating OVE ISO: ${agent_ove_iso}"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY RUN] Would create ISO with xorriso"
        return 0
    fi

    if ! xorriso -as mkisofs \
        -o "${agent_ove_iso}" \
        -J -R -V "${volume_label}" \
        -b isolinux/isolinux.bin \
        -c isolinux/boot.cat \
        -no-emul-boot -boot-load-size 4 -boot-info-table \
        -eltorito-alt-boot \
        -e images/efiboot.img \
        -no-emul-boot -boot-load-size "${boot_load_size}" \
        "${work_dir}"; then
        log_error "Failed to create OVE ISO"
        exit 1
    fi

    log_success "Successfully created OVE ISO"
}

function finalize() {
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY RUN] Would finalize ISO with isohybrid"
        return 0
    fi

    log_info "Making ISO UEFI bootable"
    if ! /usr/bin/isohybrid --uefi "$agent_ove_iso"; then
        log_error "Failed to make ISO UEFI bootable"
        exit 1
    fi

    log_success "Generated agent OVE ISO at: $agent_ove_iso"

    # Copy to final output file path
    log_info "Copying ISO to final location: ${OUTPUT_FILE_PATH}"
    cp -v "${agent_ove_iso}" "${OUTPUT_FILE_PATH}"
    log_success "Copied ISO to: ${OUTPUT_FILE_PATH}"

    # Show file size
    local iso_size=$(du -h "${OUTPUT_FILE_PATH}" | cut -f1)
    log_info "Final ISO size: ${iso_size}"

    # Calculate and display execution time
    end_time=$(date +%s)
    elapsed_time=$((end_time - start_time))
    minutes=$((elapsed_time / 60))
    seconds=$((elapsed_time % 60))

    if [[ $minutes -gt 0 ]]; then
        log_success "Execution time: ${minutes}m ${seconds}s"
    else
        log_success "Execution time: ${seconds}s"
    fi
}

function build() {
    # Parse command line arguments
    parse_arguments "$@"

    start_time=$(date +%s)
    log_info "Starting $(basename "$0")"

    # Check if running as root
    if [[ "$(id -u)" -eq 0 ]]; then
        SUDO=""
        log_warning "Running as root - some operations may not work as expected"
    else
        SUDO="sudo"
    fi

    # Setup and validate
    setup_vars

    # Main build process
    extract_live_iso
    setup_agent_artifacts
    create_ove_iso
    finalize

    # Cleanup work directory
    if [[ "$DRY_RUN" != "true" ]]; then
        rm -rf "${work_dir}"
    fi
}

# Main execution
main() {
    # Handle script interruption gracefully
    trap cleanup EXIT INT TERM

    # Run the build process
    build "$@"
}

# Only run main if script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi