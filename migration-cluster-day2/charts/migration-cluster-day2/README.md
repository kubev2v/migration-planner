# Migration Cluster Day2

This repository is meant to provide the means for Day-2 operations on a target cluster for VM migrations.
It should be used in this milestone only. Next milestone we would like to push most of this functionality into MTV.

Some or all the items here may end up as an integral part of MTV, and for the time being we will make all efforts
to automate and smooth the experience to the maximum we can.

## Prerequisites
- Installed cluster
    - 3 node baremetals that can run virtualization
    - each node has extra disk or more for storage 
- Operators
    - OpenShift Virtualization
    - OpenShift GitOps 
- `oc` client installed

## Install the ArgoCD application

After ArgoCD (OpenShift GitOps) is ready, apply this manifest that will create the applications which will
be responsible for the Day-2:

```console
oc create -f https://raw.githubusercontent.com/rgolangh/migration-cluster-day2/refs/heads/main/manifest.yaml
```

> [!Note]
> The argo application is referencing the HEAD of the main branch of the helm chart, and not a version, 
> because it is quicker and easier to publish changes. when things get stable enough we will shall move to versions.

Initialize the MTV provider

Navigate to the mtv-init application route
```console
oc get route -n default mtv-init -o jsonpath={.status.ingress[0].host}
```

Fill in the details of form:
- vddk image: go and download the vddk.tar.gz from broadcom site. The link is part of the form
- vcenter username: use the admin username, or a user which is the most credentials you can get
- vcenter password
- vcenter url: the url of vcenter, no need to add /sdk in the end

When the form is submitted, follow the job in the default namespace that creates the vddk image and updates the existing
vmware-credentials

Check the vmware provider status, if it is ready we can start migrating VMs






