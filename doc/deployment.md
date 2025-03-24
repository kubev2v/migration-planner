# Deployment of the Assisted Migration Service on OpenShift

This project provides YAML template files for deploying the **Assisted Migration Service** on OpenShift. This document outlines the deployment process.

By default, images are deployed from the `quay.io/kubev2v` namespace. New images are built and pushed to Quay after each PR is merged into this repository.

### Notice
This deployment also includes the **UI** and uses the following template:  
[Migration Planner UI Template](https://raw.githubusercontent.com/kubev2v/migration-planner-ui/refs/heads/main/deploy/templates/ui-template.yml)

## Deploying on OpenShift

The deployment process is automated via a **Makefile target** called `deploy-on-openshift`.

### 1. Log in to OpenShift and Set the Project

Ensure you are logged into OpenShift and have the correct project selected:

```sh  
oc login --token=$TOKEN --server=$SERVER  
oc new-project assisted-migration 
```

### 2. Configure Deployment (Optional)
You can override the default image sources by exporting the following environment variables before deployment:
```sh
export MIGRATION_PLANNER_API_IMAGE=<api_image_source>           # Default: quay.io/kubev2v/migration-planner-api  
export MIGRATION_PLANNER_AGENT_IMAGE=<agent_image_source>       # Default: quay.io/kubev2v/migration-planner-agent  
export MIGRATION_PLANNER_IMAGE_TAG=<agent_and_api_image_tag>    # Default: latest  
export MIGRATION_PLANNER_UI_IMAGE=<ui_image_source>             # Default: quay.io/kubev2v/migration-planner-ui  
export MIGRATION_PLANNER_UI_IMAGE_TAG=<ui_image_tag>            # Default: latest  
export MIGRATION_PLANNER_REPLICAS=<replica_count>               # Default: 1  
```

### 3. Deploy to OpenShift
Run the following command to deploy the Assisted Migration Service and its dependencies (including the UI and database):
```sh
make deploy-on-openshift
```

The deployment process deploys all relevant parts of the project, including the UI and database.

### 4. Remove the Deployment
To remove the deployment from OpenShift, including all related components, run:
```sh
make delete-from-openshift
```
