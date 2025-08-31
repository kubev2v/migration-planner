# 🛠️ Prerequisites

* **Ensure the OpenShift internal registry is initialized (steps below).**

---

### 🔑 Login to your cluster

```bash
+oc login https://api.<cluster>.<baseDomain>:6443 \
  --username=<user> --password=<password>
   # or with a token:
   # oc login --token=<token> --server=https://api.<cluster>.<baseDomain>:6443
```
---

### [📦 Install the MTV Operator (if not installed) ](https://docs.redhat.com/en/documentation/migration_toolkit_for_virtualization/2.0/html/installing_and_using_the_migration_toolkit_for_virtualization/installing-mtv_mtv#installing-the-operator_mtv)

---

### [📦 Install the OpenShift Virtualization Operator (if not installed)](https://docs.redhat.com/en/documentation/red_hat_openshift_service_on_aws/4/html/virtualization/installing#installing-virt-operator_installing-virt)

---

### 🌐 Configure the Provider

1. Go to **Migration for Virtualization → Providers**
2. Click **Create provider**
3. Select **VMware**
4. Assign a name and provide the URL
5. Download the **VDDK file** that matches your ESXi/vCenter version from:  
   👉 https://developer.broadcom.com/sdks/vmware-virtual-disk-development-kit-vddk/latest
6. Upload the downloaded file in the provider wizard
7. Enter the username and password for the VMware environment
8. Click Create provider
---

### [🗂️ Create migration plan](https://docs.redhat.com/en/documentation/migration_toolkit_for_virtualization/2.0/html/installing_and_using_the_migration_toolkit_for_virtualization/migrating-virtual-machines-to-virt_mtv#creating-migration-plan_mtv)


## 🏗️ Ensure the OpenShift Internal Registry Is Initialized

Run:

```bash
oc get configs.imageregistry.operator.openshift.io/cluster -o jsonpath='{.spec.managementState}'
```

If the returned value is Removed, it means the cluster does not have the internal registry initialized.

Two options are available:

1️⃣ Recommended: Initialize with PVC

Save the following to a file named registry-pvc.yaml:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
    name: registry-storage
    namespace: openshift-image-registry
spec:
    accessModes:
        - ReadWriteOnce
    resources:
        requests:
            storage: 5Gi
    storageClassName: lvms-vg1  # Replace with your StorageClass name
```

Apply the PVC:

```bash
oc apply -f registry-pvc.yaml
```

Patch the registry config:

```bash
oc patch configs.imageregistry.operator.openshift.io/cluster \
--type=merge \
-p '{
    "spec": {
        "managementState": "Managed",
        "rolloutStrategy": "Recreate",
        "storage": {
            "pvc": {
                "claim": "registry-storage"
            }
        }
    }
}'
```

2️⃣ Alternative: Initialize with EmptyDir (NOT PERSISTENT; NOT FOR PRODUCTION)  
Warning: All images are lost on pod restart/reschedule. Use only for ephemeral, non-critical testing.

```bash
oc patch configs.imageregistry.operator.openshift.io/cluster \
--type=merge \
-p '{
    "spec": {
        "managementState": "Managed",
        "storage": {
        "emptyDir": {}
        }
    }
}'
```

**Note:**  
The target OpenShift project for the migration must have permissions to pull the image from openshift-image-registry project