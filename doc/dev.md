# Development Instructions

## Image Creation and Push to Quay.io

These instructions will guide you on how to build images for the API, agent, and UI containers and then push them to Quay manually.

### 1. Create Dedicated Repositories in Your Quay Environment

For debugging purposes, you may need to update the containers and push them to your own Quay registry.
To do this, start by creating dedicated repositories in your Quay environment to serve as the new destinations for 
the `migration-planner-agent`, `migration-planner-ui`, and `migration-planner-api` images.

### 2. Log in to Quay

Navigate to: https://quay.io/user/YOUR_QUAY_USERNAME/?tab=settings

Click on "Generate Encrypted Password", then enter your password.

Copy the login command

Run the copied command to authenticate with Quay:

It should look like: `podman login -u='<QUAY_USERNAME>' -p='<SOME_GENERATED_PASSWORD>' quay.io`

### 3. Build and Push Images

### migration-planner-agent and migration-planner-api:

Create a directory named: **migration-planner**

Clone the [backend repository](https://github.com/kubev2v/migration-planner) into the migration-planner directory.

Open a terminal session and navigate to the migration-planner directory

Run: 

```
export MIGRATION_PLANNER_AGENT_IMAGE=quay.io/<REPLACE_WITH_YOUR_QUAY_USERNAME>/migration-planner-agent

export MIGRATION_PLANNER_API_IMAGE=quay.io/<REPLACE_WITH_YOUR_QUAY_USERNAME>/migration-planner-api

make build-containers

make push-containers
```

Optional:

By default, the registry tag is "latest." You can customize the registry tag by setting REGISTRY_TAG=<REPLACE_WITH_CUSTOM_TAG_VALUE>
For example: `make build-containers REGISTRY_TAG=v1` and then push with `make push-containers REGISTRY_TAG=v1`

### migration-planner-ui:

Create a directory named: **migration-planner-ui**

Clone the [frontend repository](https://github.com/kubev2v/migration-planner-ui) into the migration-planner-ui directory.

Open a terminal session and navigate to the migration-planner-ui directory

Run: 

```
export MIGRATION_PLANNER_UI_IMAGE="quay.io/<REPLACE_WITH_YOUR_QUAY_USERNAME>/migration-planner-ui"

podman build . -f Containerfile -t ${MIGRATION_PLANNER_UI_IMAGE}:latest

podman push ${MIGRATION_PLANNER_UI_IMAGE}:latest
```