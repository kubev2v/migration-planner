# Migration Planner Debug Environment

A comprehensive debugging environment for Migration Planner with staging data and multiple authentication modes.

## Quick Setup

1. **Obtain GABI access and token**:

   - Request access by creating a merge request to add your user in the GABI app-interface config: [gabi-assisted-migration.yml](https://gitlab.cee.redhat.com/service/app-interface/-/blob/master/data/services/gabi/gabi-instances/gabi-assisted-migration.yml?ref_type=heads)
   - After access is granted, retrieve your token from the stage cluster: [Assisted Migration (stage)](https://console-openshift-console.apps.crcs02ue1.urby.p1.openshiftapps.com/add/all-namespaces)
   - Export the token (format starts with `sha256~`):

     ```bash
     export GABI_TOKEN='sha256~DyR9X...'
     ```

2. **Configure environment**:

   ```bash
   # Create your local env file and set repo paths inside it
   cp ./.env-debug.example ./.env-debug
   # Edit ./.env-debug and update MIGRATION_PLANNER_REPO and MIGRATION_PLANNER_UI_REPO
   ```

3. **Run the setup script**:

   ```bash
   ./debug-with-staging-complete.sh
   ```

4. **Choose authentication mode** when prompted:
   - Option 1: `none` (no authentication, single user)
   - Option 2: `local` (JWT authentication, multi-user simulation)

## Purpose

This project provides a complete debugging setup for Migration Planner development with:

- **Real staging data** from production environments
- **Two authentication modes** for different development scenarios
- **Containerized environment** with proper networking and debugging support
- **Easy setup and management** through automated scripts

## Features

### 🔐 **Dual Authentication Modes**

- **Option 1 (auth=none)**: No authentication required - single user debugging
- **Option 2 (auth=local)**: JWT-based local authentication - multi-tenant simulation

### 📊 **Real Staging Data**

- Automatically syncs fresh staging data via GABI
- Preserves original organization IDs for multi-tenant testing
- Complete database with sources, assessments, and inventory data

### 🐛 **Full Debugging Support**

- Delve debugger integration on port 40000
- VS Code compatible remote debugging
- API restart scripts for debugging sessions

### 🌐 **Complete Stack**

- Migration Planner API with debugging enabled
- Migration Planner UI with authentication support
- PostgreSQL database with staging data
- Proper container networking and proxying

## Directory Structure

```
staging-debug-tool/
├── README.md                           # This file
├── .env-debug                          # Shared env for both modes (DB/backend)
├── debug-with-staging-complete.sh      # Main setup script
├── generate-user-tokens.sh             # JWT token generator
├── restart-api.sh                      # Manual API restart utility
├── sync-staging-via-gabi.sh            # Staging data downloader
├── load-data.sh                        # Database loader
├── test-gabi-response.sh               # GABI token validator
├── webpack-proxy.patch                 # UI proxy configuration
├── staging-data/                       # Downloaded staging data
├── auth_none/                          # Auth=none specific files
│   ├── debug-compose.yaml              # Docker compose (uses ../.env-debug)
│   └── Dockerfile.ui                   # UI container without auth
└── auth_local/                         # Auth=local specific files
    ├── debug-compose-local-file.yaml   # Docker compose (uses ../.env-debug)
    ├── Dockerfile.ui-local             # UI container with auth support
    ├── authFetch.ts                    # JWT authentication handler
    ├── set-ui-token.html               # Token management page
    └── private-key.txt                 # RSA private key (generated)
```

## Prerequisites

### Required Software

- **Podman** or Docker with compose
- **Go toolchain** (for Migration Planner API building)
- **Node.js 18+** (for UI building)

### Required Repositories

- **Migration Planner API**: `migration-planner`
- **Migration Planner UI**: `migration-planner-ui-app`

### Environment Variables

- **GABI_TOKEN**: Valid token for staging data access
- **MIGRATION_PLANNER_REPO**: Path to API repo (default: `migration-planner`)
- **MIGRATION_PLANNER_UI_REPO**: Path to UI repo (default: `migration-planner-ui-app`)

## Usage Instructions

### Option 1: No Authentication (auth=none)

**When to use**: Simple debugging, single-user scenarios, API development

**Setup**:

1. Run setup script and select option 1
2. Wait for containers to start

**Access**:

- **UI**: http://localhost:3000/openshift/migration-assessment
- **API**: http://localhost:3443
- **Database**: localhost:5432 (use `demouser`/`demopass` to connect)
- **Debugger**: localhost:40000

**Debugging**:

- All data appears under 'internal' organization
- No tokens required for API calls
- Direct debugging with breakpoints in IDE

**Example API call**:

```bash
curl http://localhost:3443/api/v1/sources
```

### Option 2: JWT Authentication (auth=local)

**When to use**: Multi-tenant testing, authentication flow development, organization-specific data

**Setup**:

1. Run setup script and select option 2
2. Wait for containers to start

**Generate Tokens**:

```bash
./generate-user-tokens.sh
```

**Set UI Token**:

1. Open http://localhost:3000/set-ui-token.html
2. Copy a token from the generator output
3. Paste and set the token
4. Navigate to http://localhost:3000/openshift/migration-assessment

**Access**:

- **UI**: http://localhost:3000/openshift/migration-assessment (requires token)
- **Token Page**: http://localhost:3000/set-ui-token.html
- **API**: http://localhost:3443 (requires authentication)
- **Database**: localhost:5432 (use `demouser`/`demopass` to connect)
- **Debugger**: localhost:40000

**Example API calls**:

```bash
# Set token first
export TOKEN_11009103='<your-token-here>'

# Make authenticated requests
curl -H "X-Authorization: Bearer $TOKEN_11009103" http://localhost:3443/api/v1/sources
curl -H "X-Authorization: Bearer $TOKEN_11009103" http://localhost:3443/api/v1/assessments
```

## Debugging Guide

### IDE Configuration (VS Code)

Create `.vscode/launch.json`:

```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Connect to Migration Planner",
      "type": "go",
      "request": "attach",
      "mode": "remote",
      "remotePath": "/app",
      "port": 40000,
      "host": "127.0.0.1"
    }
  ]
}
```

### Common Debugging Workflow

1. **Set breakpoints** in your Go code
2. **Start debugging** in IDE (connect to localhost:40000)
3. **Make API requests** to trigger breakpoints
4. **When debugger stops** and container exits:
   ```bash
   ./restart-api.sh
   ```
5. **Reconnect debugger** and continue

### Container Management

```bash
# View running containers
podman-compose -f auth_none/debug-compose.yaml ps
# or
podman-compose -f auth_local/debug-compose-local-file.yaml ps

# View logs
podman-compose -f auth_none/debug-compose.yaml logs planner-api-debug

# Stop environment
podman-compose -f auth_none/debug-compose.yaml down

# Restart just the API
./restart-api.sh
```

## Troubleshooting

### Common Issues

**"Private key file not found"** (auth=local):

- Run the main setup script first to generate the private key

**"GABI token validation failed"**:

- Check your GABI_TOKEN environment variable
- Ensure the token is not expired

**"Repository not found"**:

- Verify MIGRATION_PLANNER_REPO and MIGRATION_PLANNER_UI_REPO paths
- Ensure repositories are cloned and accessible

**"Permission denied for table sources"** (Database connection):

- Use `demouser`/`demopass` credentials, not `admin` (admin is for local setup)
- Connect to database `planner` on `localhost:5432`
- Connection string: `postgresql://demouser:demopass@localhost:5432/planner`

**UI shows "An error occurred while attempting to detect existing discovery sources"**:

- For auth=local: Set a token first via http://localhost:3000/set-ui-token.html
- Check container logs for API connectivity issues

**Breakpoints not working**:

- Verify debugger is connected to localhost:40000
- Check that source paths match between IDE and container
- Ensure API container is running with debug enabled

### Container Logs

```bash
# API logs
podman logs planner-api-debug

# UI logs
podman logs planner-ui

# Database logs
podman logs planner-db-staging
```
