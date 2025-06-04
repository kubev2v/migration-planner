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
oc create -f https://raw.githubusercontent.com/kubev2v/migration-planner/refs/heads/main/migration-cluster-day2/manifest.yaml
```

> [!Note]
> The argo application is referencing the HEAD of the main branch of the helm chart, and not a version, 
> because it is quicker and easier to publish changes. when things get stable enough we will shall move to versions.

Add admin role to mtv service account once the openshift-mtv namespace is automatically created
```console
oc adm policy add-role-to-user admin system:serviceaccount:openshift-gitops:openshift-gitops-argocd-application-controller -n openshift-mtv
```




