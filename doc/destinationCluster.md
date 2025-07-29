# Destination Cluster Overview

Your destination OpenShift cluster must have a set of operators installed and configured before migration can proceed.  
These operators provide networking, storage, and migration control. Ensure you’re running on a supported OpenShift version
and that your cluster has at least:

* 3 bare-metal nodes capable of running virtualization
* Each node has one or more extra disks for storage

## Operators

### OpenShift Virtualization Operator
The OpenShift Virtualization Operator brings full VM lifecycle management into your Kubernetes cluster.  
It provides VirtualMachine and DataVolume CRDs, hands off VM scheduling to the OpenShift scheduler, and leverages Container-native  
virtualization so you can manage VMs alongside containers—complete with live-migration, snapshots, and policy-driven resource control.

### MTV Operator
(Migration Toolkit for Virtualization Operator)

The MTV Operator orchestrates end-to-end VM migrations from source platforms (VMware, RHV, etc.) into OpenShift Virtualization (KubeVirt),  
automating VM discovery, migration plan scheduling, and integration with KubeVirt to provision target VMs seamlessly.

### NMState Operator
(NetworkManager State Operator)

The NMState Operator provides a Kubernetes-native API (NodeNetworkConfigurationPolicy) to declaratively configure L2 networking—VLANs,  
bonds, bridges, MAC-spoofing, SR-IOV, etc.—across your nodes, continuously reconciling network state, handling link flaps, and enabling  
consistent network isolation for migrations.

### LVM Storage Operator
(Local Volume Manager Operator)

The LVM Storage Operator dynamically provisions storage by discovering block devices, carving them into LVM thin-pool   
physical volumes via LocalVolume CRs, and exposing a StorageClass for both pods and KubeVirt DataVolumes, with support  
for thin provisioning and both static or dynamic management.

### NHC Operator 
(Node Health Check Operator)

The NHC Operator continuously evaluates node health via a NodeHealthCheck CR—probing conditions like Ready, kubelet  
status, and API reachability—and, upon detecting failure, triggers fencing and remediation actions (SSH, BMC) in coordination  
with the SNR operator for automated recovery.

### SNR Operator (Future)
(Self Node Remediation Operator)

The SNR Operator implements the remediation contract by listening for fence requests from NHC, orchestrating node cordon,  
drain, and reboot workflows using multiple fence strategies, and gracefully evacuating workloads before reboot—reporting  
progress and status back to NHC.

