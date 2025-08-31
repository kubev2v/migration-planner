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

## `kind.yml` - 📦 Migration Planner E2E (end-to-end) Test Suite

### 🔍 Overview

This repository includes a powerful **End-to-End (E2E) test suite** designed to validate the **Migration Planner** system.   
It simulates scenarios where agents interact with a central service to give assessment for virtual machine migration from vCenter to openshift.

The suite verifies the behavior of agents and services, handles image operations, manages credentials, monitors agent statuses, and enforces access boundaries.

✅ **Supports execution both locally (via CLI)** and **remotely (via GitHub Actions)**.

---

### ✅ What It Tests

| Area                   | Details                                                                  |
|------------------------|--------------------------------------------------------------------------|
| 🧠 Agent Behavior      | Lifecycle, vCenter login, inventory handling, reboot handling, logs      |
| 🔄 Source Management   | Creation, deletion, updates, org-based user isolation                    |
| 🌐 Image Handling      | Download via planner service or directly from a URL                      |
| ⚡️ Environment         | Connected and disconnected network scenarios                             |
| 🔐 Security            | Auth and JWT-based authentication and access control by user/org         |
| 🧪 Edge Cases          | VM reboots, failed logins, Disconnected environment, invalid credentials |

---

### 🚀 Running the Tests

---

### ⚙️ GitHub Actions Workflow

**Tests automatically run on:**

- Push to `main`
- Pull requests targeting `main`
- Manual dispatch via GitHub UI

📁 **Workflow config: [kind.yml](../.github/workflows/kind.yml)**

---

### 🧑‍💻 Running Locally via CLI

1. **Install prerequisites:**

    - `docker`, `kind`, `sshpass`
    - Build the planner binary:  
      `make build-cli`

2. **Run all tests:**

Please follow the instruction in: [cli.md](./cli.md)

---

### 🔧 CLI Features

The CLI (defined in [e2e.go](../internal/cli/e2e.go)) provides:

- Dynamic environment variable configuration
- Kind cluster provisioning and teardown
- Port-forwarding to essential services (registry, API, VCenter simulators) if needed
- Libvirt-based VM cleanup logic if needed
- Optional environment retention (`--keep-env` or shorthand `-k`)
- Subcommand for environment cleanup

For getting some help and getting start, Run:

```bash
planner e2e --help
```

---

### 🗂 Key Components

| File                                                                                                       | Description                                                       |
|------------------------------------------------------------------------------------------------------------|-------------------------------------------------------------------|
| [test/e2e](../test/e2e/)/*_test.go                                                                         | Directory containing core E2E test files (`*_test.go`)            |
| [e2e.go](../internal/cli/e2e.go)                                                                           | Local CLI command for running the test locally                    |
| [kind.yml](../.github/workflows/kind.yml)                                                                  | GitHub Actions CI workflow configuration                          |
| [agent.go](../test/e2e/e2e_agent/agent.go), [agent_api.go](../test/e2e/e2e_agent/agent_api.go)             | Libvirt-based VM agent control and API communication              |
| [service.go](../test/e2e/e2e_service/service.go), [service_api.go](../test/e2e/e2e_service/service_api.go) | Planner service API client implementation                         |
| [file.go](../test/e2e/e2e_utils/file.go), [command.go](../test/e2e/e2e_utils/command.go)                   | OVA unpacking, disk conversion, command running helpers           |
| [test_helpers.go](../test/e2e/e2e_helpers/test_helpers.go)                                                 | Test helper functions                                             |
| [log.go](../test/e2e/e2e_utils/log.go)                                                                     | Print and handle logs                                             |
| [auth.go](../test/e2e/e2e_utils/auth.go)                                                                   | Auth and JWT token generation using private key                   |
| [Makefile](../Makefile), [e2e.mk](../deploy/e2e.mk)                                                        | Build and test orchestration, Steps for creating test environment |

---

### 🪵 Logs & Debugging
The E2E test suite includes logging for each phase of the testing process. Logs are printed to standard output
to help identify and diagnose test failures efficiently.

- ### **During test**  

Logged directly during test runs using the zap logger. These include timestamps, module identifiers,  
and key lifecycle actions.

**🔎 Example Log Flow**  
A typical test execution will generate logs like this:

```bash
2025-04-22T14:28:12.542Z	INFO	Initializing PlannerService...
2025-04-22T14:28:12.543Z	INFO	[PlannerService] Creating source: user: <USER>, organization: <ORG>
2025-04-22T14:28:12.543Z	INFO	[Service-API] http://10.1.0.70:3443/api/v1/sources [Method: POST]
...
2025-04-22T14:29:06.357Z	INFO	============Successfully Passed: <TEST_NAME>=====
2025-04-22T14:29:47.944Z	INFO	Cleaning up after test...
...
2025-04-22T14:29:48.471Z	INFO	[<TEST_NAME>] finished after: XXXs
...
```

**📋 What’s Included**  

Logs provide visibility into:  
1. Source creation, update and deletion  
2. Agent image retrieval and unpacking  
3. VM boot and planner-agent startup  
4. Agent status both from the agent and service perspective  
5. API requests and results (GET, POST, PUT, DELETE)  
6. Credential flow and vCenter login  
7. Execution summary (with duration per test)  

- ### **On failure**  

If a test fails:

The agent’s full journal output is printed to the Ginkgo test output (DumpLogs).

The system automatically calls AfterFailed() to trigger log dumping.

---