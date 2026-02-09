# Local Development Guide

This guide provides instructions for setting up and running the Migration Planner locally for development purposes.

## Prerequisites

- Go 1.19 or later
- Make
- Docker or Podman
- Git

## Clone the Migration-Planner repository

```bash
# Clone the migration-planner repository
git clone https://github.com/kubev2v/migration-planner.git
cd migration-planner
```

## Run the Migration Planner API locally

Follow these steps to get the Migration Planner running locally:

### 1. Build the Project

Build the project binaries:

```bash
make build
```

### 2. Set Up the Database

Stop any existing database containers and deploy a fresh database:

```bash
make kill-db
make deploy-db
```

### 3. Download OPA Policies

Download the OPA (Open Policy Agent) policies required for validation. These policies are used to assess and validate the source environment.

```bash
make setup-opa-policies
```

> **Note:** Once you are done with your work, you can run `make clean-opa-policies` to remove the downloaded policies from your local machine.

### 4. Configure Environment Variables

Set the required environment variables for local development:

```bash
export MIGRATION_PLANNER_AGENT_AUTH_ENABLED=false
export MIGRATION_PLANNER_AUTH=none
```

These settings disable authentication for local development, making it easier to test and develop the application.

### 5. Run the Application

Start the Migration Planner API server:

```bash
make run
```

### 6. Verify API is Running

The migration-planner API should be running on `http://localhost:3443`. Verify it's working:

```bash
# Test the API endpoints
curl http://localhost:3443/api/v1/sources
```

Expected response: JSON data or empty arrays (not 404 errors).

## Use the Migration Planner API

There are different ways to use the Migration Planner API:

### 1. Run the UI

You can install the UI project and use it.
Please follow the instructions in this guide:
https://github.com/kubev2v/migration-planner-ui-app/blob/master/docs/standalone-run-locally.md

### 2. Use the planner CLI

You can use the CLI from terminal to complete several actions.
For example:

#### a. Create source:

```bash
bin/planner create <source-name>
```

#### b. List all sources:

```bash
bin/planner get sources
```

#### c. Get source details:

```bash
bin/planner get sources/<source-id>
```

#### d. Delete a source:

```bash
bin/planner delete sources/<source-id>
```

#### e. Help for more planner CLI options

```bash
bin/planner --help
```

#### f. Get info

```bash
bin/planner info
```

### 3. Use curl to access the API

You can use curl to perform API calls directly.
For example:

#### a. List all sources:

```bash
curl http://localhost:3443/api/v1/sources
```

#### b. Create a new source:

```bash
curl -i -X POST 'http://localhost:3443/api/v1/sources' \
  -H "Content-type: application/json" \
  --data '{
  "name": "test2"
}'
```

#### c. Get a specific source:

```bash
curl http://localhost:3443/api/v1/sources/{source-id}
```

#### d. Get source download URL:

```bash
curl http://localhost:3443/api/v1/sources/{source-id}/image-url
```

#### e. Delete a source:

```bash
curl -X DELETE http://localhost:3443/api/v1/sources/{source-id}
```

#### f. Get info:

```bash
curl http://localhost:3443/api/v1/info
```

#### g. List all assessments:

```bash
curl http://localhost:3443/api/v1/assessments
```

#### h. List all assessments with specific SourceID:

```bash
curl http://localhost:3443/api/v1/assessments?sourceId={source-id}
```

#### i. Create new assessment of type agent:

```bash
curl -X POST "http://localhost:3443/api/v1/assessments" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "test-from-source",
    "sourceType": "agent",
    "sourceId": "{source-id}"
  }'
curl http://localhost:3443/api/v1/assessments
```

## Run the Sizer Service locally

The sizer service is used for cluster sizing calculations. It runs as a separate containerized service.

### 1. Start the Sizer Service

Run the sizer service container:

```bash
make run-sizer
```

This will:

- Pull the latest sizer image from Quay
- Start the container on port 9200
- Display helpful URLs and commands

### 2. Verify Sizer is Running

Test the health endpoint:

```bash
curl http://localhost:9200/health
```

Expected response:

```json
{
  "status": "ok",
  "service": "sizer-library",
  "version": "1.0.0"
}
```

### 3. Test the Sizing API

Example request to the sizing API:

```bash
curl -X POST http://localhost:9200/api/v1/size/custom \
  -H "Content-Type: application/json" \
  -d '{
    "platform": "BareMetal",
    "detailed": true,
    "machineSets": [
      {
        "name": "worker",
        "cpu": 64,
        "memory": 256,
        "instanceName": "worker-large",
        "numberOfDisks": 24,
        "onlyFor": [],
        "label": "Worker"
      }
    ],
    "workloads": [
      {
        "name": "test-workload",
        "count": 1,
        "usesMachines": [],
        "services": [
          {
            "name": "test-service",
            "requiredCPU": 10,
            "requiredMemory": 20,
            "zones": 1,
            "runsWith": [],
            "avoid": []
          }
        ]
      }
    ]
  }' | jq .
```

## Run the Migration Planner Agent locally

Follow these steps to get the agent running locally:

### 1. Create a source and get the source ID

First, create a source using the API and note the source ID:

```bash
# Create a source
SOURCE_RESPONSE=$(curl -s -X POST 'http://localhost:3443/api/v1/sources' \
  -H "Content-type: application/json" \
  --data '{
  "name": "local-test-source"
}')

# Extract the source ID (you'll need jq installed, or manually copy the ID from the response)
export SOURCE_ID=$(echo $SOURCE_RESPONSE | jq -r '.id')
echo "Source ID: $SOURCE_ID"
```

### 2. Create Agent ID

Generate a unique agent ID:

```bash
# Generate a random UUID for the agent
export AGENT_ID=$(uuidgen)
echo "Agent ID: $AGENT_ID"
```

### 3. Run the Agent Application

Start the Migration Planner Agent BE:

```bash
# Build and run the agent from the agent-v2 submodule
make build-agent
agent-v2/bin/agent run --agent-id $AGENT_ID --source-id $SOURCE_ID
```

### 3. Run the Agent UI

Start the Migration Planner Agent UI:

```bash
make agent-image
make run-agent-ui
```

### 7. Verify Agent Status

Check that the agent is connecting properly:

```bash
# Check agent status via the agent's local endpoint (using HTTPS with self-signed cert)
curl -k https://localhost:3333/api/v1/status

# Check if the source shows the agent as connected
curl http://localhost:3443/api/v1/sources/$SOURCE_ID
```

### 8. Add Credentials to the Agent

The agent needs VMware vCenter credentials to collect inventory data. You can provide credentials in the following ways:

#### Option 1: Using the API directly

The web UI is not available in local development, so use the REST API to add credentials:

```bash
curl -k -X PUT https://localhost:3333/api/v1/credentials \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://your-vcenter-server.com/sdk",
    "username": "your-vcenter-username",
    "password": "your-vcenter-password",
    "isDataSharingAllowed": true
  }'
```

#### Option 2: Manual credentials file

For local testing, you can manually create the credentials file:

```bash
cat > $HOME/tools/migration-planner/data/credentials.json << EOF
{
  "url": "https://your-vcenter-server.com/sdk",
  "username": "your-vcenter-username",
  "password": "your-vcenter-password",
  "isDataSharingAllowed": true
}
EOF
```

**Note**: The web UI at `https://localhost:3333` is not available in local development as it requires the static web files to be built and placed in the agent's www directory. For local development, use the API method above.

### 9. Verify Inventory Collection

After adding credentials, the agent will start collecting inventory data. You can check the progress:

```bash
# Check agent status - should show "gathering-initial-inventory" or "up-to-date"
curl -k https://localhost:3333/api/v1/status

# Check for inventory file (created after successful collection)
ls -la $HOME/tools/migration-planner/data/inventory.json

# Download the inventory via the agent API
curl -k https://localhost:3333/api/v1/inventory > local-inventory.json
```
