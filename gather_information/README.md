gather_information Role
=========

This role is intended to gather the vCenter information and store the information into an output local json file.

The overall flow of this role is as follows:
- Get a list of ESXi hosts 
- Get information
- Get vm information
- Get vm detailed information
- Get vm_tools information
- Get vm network information
- Check vm tools is installed on each vm
- Get datacenter information 
- Get folders information
- Get cluster information
- Gather info about ESXi Host for each cluster
- Get network information from VMWare
- Gather all registered dvswitch
- Get datastore information from VMWare
- Get resourcepool information
- Write all the information into an output file.

Requirements
------------
Collecitons:
  - name: community.general
  - name: vmware.vmware_rest
  - name: community.vmware

Role Variables
--------------

A description of the settable variables for this role should go here, including any variables that are in defaults/main.yml, vars/main.yml, and any variables that can/should be set via parameters to the role. Any variables that are read from other roles and/or the global scope (ie. hostvars, group vars, etc.) should be mentioned here as well.


| Variable | Type    | Description                                                                                                                                      | Default |
| --- |---------|--------------------------------------------------------------------------------------------------------------------------------------------------| ---|
`gather_information_vsphere_hostname` | string  | vCenter hostname                                                                                                                                 | 
`gather_information_vsphere_username` | string  | vCenter username                                                                                                                                 | 
`gather_information_vsphere_password` | string  | vCenter password                                                                                                                                 | 
`gather_information_vsphere_validate_certs` | boolean | vCenter validate certs                                                                                                                           | false
`gather_information_store_in_a_file` | boolean | This is used to determing if an output file is needed                                                                                           | true
`gather_information_output_file_name` | string | The output file name | "output_file.txt"
`gather_information_vsphere_connected_states` | list    | This is used to determine if only a certain type of hypervisor should be collected. This can be blank and all listed in vcenter will be gathered | "[DISCONNECTED, CONNECTED]"



Example Playbook
--------------
```yaml
---
- name: Manage VMWare Content Library
  hosts: all
  gather_facts: false

  roles:
    - role: gather_information
      vars:
        content_library_hostname: <>
        content_library_username: <>
        content_library_password: <>
```

License
--------------

GNU General Public License v3.0 or later

See [LICENCE](https://github.com/ansible-collections/cloud.aws_troubleshooting/blob/main/LICENSE) to see the full text.

Author Information
--------------
- Ansible Cloud Content Team