# Deploying OpenShift Migration Advisor in Debug Mode

## Objective  
This guide explains how to deploy the OpenShift Migration Advisor application in debug mode on OpenShift.
Debug mode enables additional logging and debugging features for development purposes.

## 📌 Prerequisites

Before proceeding, ensure you have the following:

* Access to an OpenShift cluster and OpenShift CLI (oc) installed and configured

## 📜 Deployment Steps

## 1️⃣  Set Environment Variables

Export the necessary environment variables:
```
export DEBUG_MODE=true # Using a special debug Container file for both the API and the agent  
export MIGRATION_PLANNER_API_IMAGE=quay.io/<QUAY-USERNAME>/migration-planner-api  
export MIGRATION_PLANNER_AGENT_IMAGE=quay.io/<QUAY-USERNAME>/migration-planner-agent 
export QUAY_USER="YOUR_QUAY_USERNAME"
export QUAY_TOKEN="YOUR_QUAY_TOKEN"
```

## 2️⃣  Build and Push Containers

Run the following command to build and push the container images:

`make push-containers`

## 3️⃣  Deploy the Application on OpenShift
Please follow the instructions in **[deployment.md](../deployment.md)** for how to deploy migration-planner on OpenShift.  

## 4️⃣  Verifying the Deployment

Check if the pods are running:

`oc get pods`

## 5️⃣ Forward a Port for Debugging

Once the deployment is running, forward a local port to the application for debugging:

`oc port-forward deploy/migration-planner 40000`

Then, you can set up your IDE debugger to connect to localhost:40000 for debugging the API.

## 🖥️ Connect an IDE 
To debug the application using an IDE, follow the setup instructions in the 
[goland.md](goland.md) 
or in the [vscode.md](vscode.md) files 
based on you preferred IDE.
