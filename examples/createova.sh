#!/bin/bash

# Example of usage:
#./createova.sh 10.0.0.8 ~/.ssh/id_rsa.pub

# Check the arguments
if [ $# -lt 2 ]; then
    echo "Error: Two arguments must be passed. IP address of Agent Service and Public key path."
    exit 1
fi

# Generate config.yaml
sed -e "s|@CONFIG_IP@|$1|g" config.yaml.template > config.yaml

# Generate config.ign file from template
sed -e "s|@CONFIG_DATA@|$(cat config.yaml | base64)|g" -e "s|@CONFIG_SSH_KEY@|$(tr -d '[:space:]' < $2)|g" config.ign.template > config.ign

# Download RHCOS live ISO
if [ ! -f "rhcos-live.x86_64.iso" ]; then
    curl -C - -O https://mirror.openshift.com/pub/openshift-v4/dependencies/rhcos/latest/rhcos-live.x86_64.iso
else
    echo "rhcos-live.x86_64.iso already exists, skipping download."
fi

# Bundle ignition
podman run --privileged --rm -e PWD=$PWD -v /dev:/dev -v /run/udev:/run/udev -v $PWD:/data -w /data quay.io/coreos/coreos-installer:release iso ignition embed -fi config.ign -o /data/AgentVM-1.iso rhcos-live.x86_64.iso

# Create OVA file
tar -cvf AgentVM.ova AgentVM-1.iso AgentVM.ovf

rm -f AgentVM-1.iso
