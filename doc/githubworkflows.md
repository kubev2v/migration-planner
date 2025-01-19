# GitHub Workflows for Assisted Migration

## `build-push-images.yml` - Automate Image Creation and Push to Quay.io

### Objective:

The primary goal of this workflow is to automate the process of building images and pushing them to Quay.io. 
This ensures that the latest images are always available for deployment.

In addition to the latest tag, images are tagged with the commit SHA when merged from the main branch, 
or with the branch name if merged from any other branches. These tags are configured to expire after 10 days.

Please be aware that there is almost a cloned workflow running in the [assisted migration UI repo](https://github.com/kubev2v/migration-planner-ui/blob/main/.github/workflows/build-push-images.yaml),
which is responsible for pushing the UI image to Quay.

### Triggers:

- **Merge PR into the `main` branch**: This workflow is automatically triggered when a pull request is merged into the `main` branch.
- **Manual Trigger (Optional)**: The workflow can also be manually triggered if needed (permission needed for the action).

### How It Works:

1. The workflow initiates a GitHub runner, which creates an instance of Ubuntu.
2. The machine created logs into Quay.io using the GitHub secrets configured at the repository level. 
3. Using the `Makefile` defined in the repository, the runner executes the necessary commands:
    - **`make build-containers` - Build the Images**: The images are built based on the defined specifications.
    - **`make push-containers` - Push the Images**: After building, the images are pushed to the Quay.io repository, making them available 
   for use in the environment.

### How to Customize the Quay Repo Destination

This step may be crucial for testing or debugging purposes.

Customizing the repository destination for the UI image follows similar steps to the process outlined below.

1. Create Dedicated Repositories in Your Quay Environment

First, create dedicated repositories in your Quay environment that will replace the destination for the `migration-planner-agent` and `migration-planner-api` images. The replacements will be as follows:

- **Old Destination**: `quay.io/kubev2v/migration-planner-agent`  
  **New Destination**: `quay.io/<YOUR-QUAY-USERNAME>/migration-planner-agent`

- **Old Destination**: `quay.io/kubev2v/migration-planner-api`  
  **New Destination**: `quay.io/<YOUR-QUAY-USERNAME>/migration-planner-api`

2. Create a Quay Bot Account

Create a bot account in Quay and grant it **admin permissions** for the repositories you created in the previous step. This bot account will be responsible for pushing images to your custom Quay repositories.

3. Add Bot Credentials to Your GitHub Fork

Next, add the bot account credentials to your GitHub fork of this repository. Use the key pair defined in the workflow: `QUAY_USERNAME` and `QUAY_TOKEN`. Set the values as follows:

- `QUAY_USERNAME`: The username of the bot account.
- `QUAY_TOKEN`: The password/token of the bot account.

These credentials will allow GitHub Actions to authenticate with Quay and push images to the custom repositories.

4. Modify the Makefile to Override Image Definitions

To ensure that the workflow pushes the images to your custom Quay repositories, you’ll need to override the definitions in the `Makefile`.

Before modification, the `Makefile` contains:

    `MIGRATION_PLANNER_AGENT_IMAGE ?= quay.io/kubev2v/migration-planner-agent`
    `MIGRATION_PLANNER_API_IMAGE ?= quay.io/kubev2v/migration-planner-api`

After modification, update the repository destination to your custom Quay repositories:

    `MIGRATION_PLANNER_AGENT_IMAGE ?= quay.io/<YOUR-QUAY-USERNAME>/migration-planner-agent`
    `MIGRATION_PLANNER_API_IMAGE ?= quay.io/<YOUR-QUAY-USERNAME>/migration-planner-api`

5. Modify the Workflow Trigger for Custom Branches (Optional)

If you'd like the workflow to trigger automatically when pushing changes to a custom branch, you can modify the trigger configuration in the workflow file.

By default, the workflow is triggered on push to the main branch only:

    on:
        push:
            branches:
                - main
        workflow_dispatch:

To also trigger the workflow on a custom branch, add your branch name like this:

    on:
        push:
            branches:
                - main
                - <MY-CUSTOM-BRANCH>
        workflow_dispatch:

---

### `kind.yml` - Running End-to-End (E2E) Tests

**Objective:**

This workflow is designed to execute end-to-end (E2E) tests to ensure the system works as expected from start to finish, 
simulating real user interactions and verifying critical functionalities.

**Triggers:**

**How It Works:**

