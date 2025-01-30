# Development Instructions

## Image Creation and Push to Quay.io

These instructions will guide you on how to build images for the API, agent, and UI containers and then push them to Quay manually.

### 1. Create Dedicated Repositories in Your Quay Environment

For debugging purposes you may need to update the containers and push them to your Quay account. 
To do this, start by creating dedicated repositories in your Quay environment to serve as the new destinations for 
the `migration-planner-agent`, `migration-planner-ui`, and `migration-planner-api` images.

**If you're acting on behalf of the team and need to manually push images under the `kubev2v` domain, please proceed directly to step 3.**

Finally, please replace `<QUAY-USERNAME>` with the desired Quay username that hosts the destination repositories in the commands below.

### 2. Create a Quay Bot Account

Create a bot account in Quay and grant it **admin permissions** for the destination repositories. 
This bot account will be responsible for pushing images to your custom Quay repositories.

### 3. Login to Quay Using Bot Credentials

Run the following command to authenticate with Quay:

RUN:

`podman login -u='<REPLACE_WITH_QUAY_BOT_USERNAME>' -p='<REPLACE_WITH_QUAY_BOT_PASSWORD>' quay.io`

These credentials will allow to authenticate with Quay and push images to the custom repositories.

### 4. Build and push Images

### migration-planner-agent and migration-planner-api:

RUN: 

`cd migration-planner` (path to migration planner backend cloned repo)

`make build-containers`

`make push-containers`

Optional:

By default, the registry tag is "latest". It's possible to customize registry tag by provide REGISTRY_TAG=<REPLACE_WITH_CUSTOM_TAG_VALUE>
For example: `make build-containers REGISTRY_TAG=v1` and then push with `make push-containers REGISTRY_TAG=v1`

### migration-planner-ui:

Please clone the [frontend repository](https://github.com/kubev2v/migration-planner-ui)

RUN:

`cd migration-planner-ui` (navigate to the cloned frontend repository)

`export MIGRATION_PLANNER_UI_IMAGE="quay.io/<QUAY-USERNAME>/migration-planner-ui"`

`podman build . -f Containerfile -t ${MIGRATION_PLANNER_UI_IMAGE}:latest`

`podman push ${MIGRATION_PLANNER_UI_IMAGE}:latest`

