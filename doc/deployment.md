# Deployment of the OpenShift Migration Advisor on OpenShift

This project provides YAML template files for deploying the **OpenShift Migration Advisor** on OpenShift. This document outlines the deployment process.

By default, images are deployed from the `quay.io/kubev2v` namespace. New images are built and pushed to Quay after each PR is merged into this repository.

### Notice
This deployment also includes the **UI** and uses the following template:  
[OpenShift Migration Advisor UI Template](https://raw.githubusercontent.com/kubev2v/migration-planner-ui/refs/heads/main/deploy/templates/ui-template.yml)

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
export MIGRATION_PLANNER_API_IMAGE=<api_image_source>           # Default: quay.io/redhat-user-workloads/assisted-migration-tenant/migration-planner-api  
export MIGRATION_PLANNER_AGENT_IMAGE=<agent_image_source>       # Default: quay.io/redhat-user-workloads/assisted-migration-tenant/migration-planner-agent  
export MIGRATION_PLANNER_IMAGE_TAG=<agent_and_api_image_tag>    # Default: latest  
export MIGRATION_PLANNER_UI_IMAGE=<ui_image_source>             # Default: quay.io/kubev2v/migration-planner-ui  
export MIGRATION_PLANNER_UI_IMAGE_TAG=<ui_image_tag>            # Default: latest  
export MIGRATION_PLANNER_REPLICAS=<replica_count>               # Default: 1  
export SERVICE_API_PATH=<api_path_prefix>                       # Default: /api/migration-assessment (allows multiple instances in same namespace)
export COST_ESTIMATION_IMAGE=<cost_estimation_image_source>                               # Default: quay.io/redhat-user-workloads/assisted-migration-tenant/cost-estimation
export COST_ESTIMATION_IMAGE_TAG=<cost_estimation_image_tag>                              # Default: latest
export COST_ESTIMATION_PORT=<cost_estimation_container_port>                              # Default: 9205
export COST_ESTIMATION_MIGRATION_PLANNER_TIMEOUT=<planner_timeout>                        # Default: 30s
```


### 3. Deploy to OpenShift
Run the following command to deploy the OpenShift Migration Advisor and its dependencies (including the UI and database):
```sh
make deploy-on-openshift
```

The deployment process deploys all relevant parts of the project, including the UI and database.

Cost-estimation is deployed as a decoupled service (`migration-planner-cost-estimation`) and calls migration-planner using the internal URL configured by `COST_ESTIMATION_MIGRATION_PLANNER_URL`.

### 4. Remove the Deployment
To remove the deployment from OpenShift, including all related components, run:
```sh
make delete-from-openshift
```
