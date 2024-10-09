# Deployment of the agent service
The project contains yaml files for Openshift deployment. This document describes the deployment process.
By default we deploy images from `quay.io/kubev2v` namespace. We push latest images after every merge of the PRs.

## Deploy on openshift
In order to deploy the Agent service on top of Openshift there is Makefile target called `deploy-on-openshift`.

```
$ oc login --token=$TOKEN --server=$SERVER
$ make deploy-on-openshift
```

The deployment process deploys all relevant parts of the project including the UI and database.

To undeploy the project, which removes all the relevent parts run:
```
make undeploy-on-openshift
```

## Using custom images of API/UI
If you want to deploy the project with your own images you can specify custom enviroment variables:

```
export MIGRATION_PLANNER_API_IMAGE=quay.io/$USER/migration-planner-api
export MIGRATION_PLANNER_UI_IMAGE=quay.io/$USER/migration-planner-ui
make deploy-on-openshift
```

## Using custom images of Agent
Agent images are defined in the ignition file. So in order to modify the images of the Agent you need to pass the specific environment variables to the deployment of API service. Modify `deploy/k8s/migration-planner.yaml` and add relevant env variable for example:

```
env:
  - name: MIGRATION_PLANNER_COLLECTOR_IMAGE
    value: quay.io/$USER/migration-planner-collector
  - name: MIGRATION_PLANNER_AGENT_IMAGE
    value: quay.io/$USER/migration-planner-agent
```