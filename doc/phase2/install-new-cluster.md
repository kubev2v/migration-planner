# 🚀 Install a New Cluster Using Assisted Installer

Navigate to 👉 https://console.redhat.com/openshift/assisted-installer/clusters/~new?source=assisted_migration

---

### 1. 🏷️ Cluster details

1. Assign a cluster name of your choice
2. Provide the base domain (e.g., `example.com`)
3. Set the number of control plane nodes to: **3 (high availability cluster)**

---

### 2. ⚙️ Operators

- Select the virtualization bundle

---

### 3. 🖥️ Host discovery

1. Click **Add host**
2. (Optional) Provide an **SSH public key** (lets you log in as user `core`)
3. Click **Generate Discovery ISO** → an ISO file will be downloaded

**Now, navigate to your vCenter environment:**

4. Create the control-plane VMs

    - Using the downloaded ISO, set up the VMs that will host the cluster.
    - If using vCenter, follow these instructions:

    1. Upload the ISO to the vCenter datastore
    2. For each VM (3 control plane nodes configured earlier):
        - Name the VM as you wish
        - Configure the VM with:
            - 💾 Storage: 100GB for the main disk, 30GB for additional disk
            - 🖥️ CPU: 16 cores
            - 🧠 RAM: 40GB
            - 🌐 NIC: Ensure network access to `console.redhat.com`
        - In **Customize hardware → Advanced parameters**, add:
            - **Attribute**: `disk.EnableUUID`
            - **Value**: `True`
        - Click **Add**

5. Boot the VMs and wait until they appear as **Ready** in the wizard. Then proceed.

---

### 4. 💽 Storage

Configure cluster storage

---

### 5. 🌐 Networking

Configure cluster networking

---

### ✅ Summary

Finally, you’ve reached this step 🎉  
Review the settings and click **Install cluster**.

This will take a while - time to grab a coffee ☕  
