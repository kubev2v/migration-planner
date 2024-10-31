# Deployment of the Agent service on OpenShift
The project contains yaml files for deploying the Agent service on OpenShift. This document describes the deployment process.
By default images are deployed from the `quay.io/kubev2v` namespace. New images are built and pushed to quay after each PR is merged in this repo.

## Deploy on OpenShift
In order to deploy the Agent service on top of OpenShift there is Makefile target called `deploy-on-openshift`.

```
$ oc login --token=$TOKEN --server=$SERVER
$ oc new-project assisted-migration
$ make deploy-on-openshift MIGRATION_PLANNER_NAMESPACE=assisted-migration
```

The deployment process deploys all relevant parts of the project, including the UI and database.

To undeploy the project, which removes all the relevent parts, run:
```
make undeploy-on-openshift
```

## Using custom images for the Agent Service API and UI
If you want to deploy the project with your own images you can specify custom enviroment variables:

```
export MIGRATION_PLANNER_API_IMAGE=quay.io/$USER/migration-planner-api
export MIGRATION_PLANNER_UI_IMAGE=quay.io/$USER/migration-planner-ui
make deploy-on-openshift
```

## Using custom Agent Images used in the Agent OVA
Agent images are defined in the ignition file. In order to modify the images of the Agent you need to pass the specific environment variables to the deployment of the API service. Modify `deploy/k8s/migration-planner.yaml` and add relevant environment variables to the deployment manifest. For example:

```
env:
  - name: MIGRATION_PLANNER_AGENT_IMAGE
    value: quay.io/$USER/migration-planner-agent
```
