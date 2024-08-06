#!/bin/bash

sudo dnf install -y python3-pip vim
sudo dnf install -y https://dl.fedoraproject.org/pub/epel/epel-release-latest-9.noarch.rpm
sudo dnf install -y ansible podman

pip install aiohttp 
pip3 install pyvmomi

mkdir ~/ansible-playbooks
