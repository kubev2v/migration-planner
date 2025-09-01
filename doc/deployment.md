# Deployment of the Assisted Migration Service on OpenShift

This project provides YAML template files for deploying the **Assisted Migration Service** on OpenShift. This document outlines the deployment process.

By default, images are deployed from the `quay.io/redhat-user-workloads/assisted-migration-tenant` namespace. New images are built and pushed to Quay after each PR is merged into this repository.

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
export MIGRATION_PLANNER_API_IMAGE=<api_image_source>           # Default: quay.io/redhat-user-workloads/assisted-migration-tenant/migration-planner-api  
export MIGRATION_PLANNER_AGENT_IMAGE=<agent_image_source>       # Default: quay.io/redhat-user-workloads/assisted-migration-tenant/migration-planner-agent  
export MIGRATION_PLANNER_IMAGE_TAG=<agent_and_api_image_tag>    # Default: latest  
export MIGRATION_PLANNER_UI_IMAGE=<ui_image_source>             # Default: quay.io/redhat-user-workloads/assisted-migration-tenant/migration-planner-ui  
export MIGRATION_PLANNER_UI_IMAGE_TAG=<ui_image_tag>            # Default: latest  
export MIGRATION_PLANNER_REPLICAS=<replica_count>               # Default: 1  
```


To use custom rhcos image stored in S3 bucket, you must supply s3 credentials in the secret created by the `s3-secret-template.yml` template.
The s3 endpoint and bucket are set via env variables:
```sh
export MIGRATION_PLANNER_S3_ENDPOINT=<s3_endpoint>
export MIGRATION_PLANNER_S3_BUCKET=<s3_bucket>
export MIGRATION_PLANNER_S3_ISO_FILENAME=<iso_filename>
```

> MIGRATION_PLANNER_S3_ISO_FILENAME defaults to `custom-rhcos-live-iso.x86_64.iso`

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
