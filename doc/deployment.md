# Deployment of the Assisted Migration service on OpenShift
The project contains yaml files for deploying the Assisted Migration service on OpenShift. This document describes the deployment process.
By default, images are deployed from the `quay.io/kubev2v` namespace. New images are built and pushed to quay after each PR is merged in this repo.

## Deploy on OpenShift
In order to deploy the Assisted Migration service on top of OpenShift there is Makefile target called `deploy-on-openshift`.

```
$ oc login --token=$TOKEN --server=$SERVER
$ oc new-project assisted-migration
$ make deploy-on-openshift MIGRATION_PLANNER_NAMESPACE=assisted-migration
```

The deployment process deploys all relevant parts of the project, including the UI and database.

To undeploy the project, which removes all the relevant parts, run:
```
make delete-from-openshift
```

## Using custom images for the Assisted Migration Service API, UI and agent
If you want to deploy the project with your own images you can specify custom environment variables:

```
export MIGRATION_PLANNER_API_IMAGE=quay.io/$USER/migration-planner-api
export MIGRATION_PLANNER_UI_IMAGE=quay.io/$USER/migration-planner-ui
export MIGRATION_PLANNER_AGENT_IMAGE=quay.io/$USER/migration-planner-agent 
make deploy-on-openshift
```