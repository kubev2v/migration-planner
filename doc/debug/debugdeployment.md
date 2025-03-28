# Deploying Migration Planner in Debug Mode

## Objective  
This guide explains how to deploy the Migration Planner application in debug mode on OpenShift.
Debug mode enables additional logging and debugging features for development purposes.

## 📌 Prerequisites

Before proceeding, ensure you have the following:

* Access to an OpenShift cluster and OpenShift CLI (oc) installed and configured

## 📜 Deployment Steps

## 1️⃣  Set Environment Variables

Export the necessary environment variables:
```
export DEBUG_MODE=true # Using a special debug Container file for both the API and the agent  
export MIGRATION_PLANNER_NAMESPACE=<NAMESPACE>  
export MIGRATION_PLANNER_API_IMAGE=quay.io/<QUAY-USERNAME>/migration-planner-api  
export MIGRATION_PLANNER_AGENT_IMAGE=quay.io/<QUAY-USERNAME>/migration-planner-agent  
```

## 2️⃣  Build and Push Containers

### Log in to Quay

Navigate to: https://quay.io/user/YOUR_QUAY_USERNAME/?tab=settings

Click on "Generate Encrypted Password", then enter your password.

Copy the login command

Run the copied command to authenticate with Quay:

It should look like: `podman login -u='<QUAY_USERNAME>' -p='<SOME_GENERATED_PASSWORD>' quay.io`

### Build and Push

Run the following command to build and push the container images:

`make push-api-container`

## 3️⃣  Deploy the Application on OpenShift
Please follow the instructions in **deployment.md** for how to deploy migration-planner on OpenShift.  
Note: please create a new oc project with `MIGRATION_PLANNER_NAMESPACE` value.  

## 4️⃣  Verifying the Deployment

Check if the pods are running:

`oc get pods`

## 5️⃣ Forward a Port for Debugging

Once the deployment is running, forward a local port to the application for debugging:

`oc port-forward deploy/migration-planner 40000`

Then, you can set up your IDE debugger to connect to localhost:40000 for debugging the API.

## 🖥️ Connect an IDE 
To debug the application using an IDE, follow the setup instructions in the 
[goland.md](https://github.com/kubev2v/migration-planner/blob/main/doc/debug/goland.md) 
or in the [vscode.md](https://github.com/kubev2v/migration-planner/blob/main/doc/debug/vscode.md) files 
based on you preferred IDE.
