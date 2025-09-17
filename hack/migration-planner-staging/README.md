# Migration Planner Debug Environment

A comprehensive debugging environment for Migration Planner with staging data and multiple authentication modes.

## Quick Setup

1. **Configure environment variables**:

   ```bash
   # Review ./.env-debug (shared for both modes). It already contains safe defaults
   # Export repository paths for your system (if different from defaults):
   export MIGRATION_PLANNER_REPO=/your/path/to/migration-planner
   export MIGRATION_PLANNER_UI_REPO=/your/path/to/migration-planner-ui-app
   ```

2. **Run the setup script**:

   ```bash
   ./debug-with-staging-complete.sh
   ```

3. **Choose authentication mode** when prompted:
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
migration-planner-staging/
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

- **Migration Planner API**: `~/code/migration-planner`
- **Migration Planner UI**: `~/code/migration-planner-ui-app`

### Environment Variables

- **GABI_TOKEN**: Valid token for staging data access
- **MIGRATION_PLANNER_REPO**: Path to API repo (default: `~/code/migration-planner`)
- **MIGRATION_PLANNER_UI_REPO**: Path to UI repo (default: `~/code/migration-planner-ui-app`)

## Quick Start

### 1. Setup Environment

```bash
# Clone this repository to the standard location
mkdir -p ~/tools
cd ~/tools
# ... copy migration-planner-staging files here ...

# Set required environment variables
export GABI_TOKEN="your-gabi-token"
export MIGRATION_PLANNER_REPO="~/code/migration-planner"
export MIGRATION_PLANNER_UI_REPO="~/code/migration-planner-ui-app"
```

### 2. Run Setup Script

```bash
cd ~/tools/migration-planner-staging
./debug-with-staging-complete.sh
```

### 3. Choose Authentication Mode

```
🔐 Choose authentication mode:
  1) none   - No authentication (single user as 'internal' org)
  2) local  - Local JWT authentication (multi-user simulation)

Select authentication mode (1 or 2):
```

## Usage Instructions

### Option 1: No Authentication (auth=none)

**When to use**: Simple debugging, single-user scenarios, API development

**Setup**:

1. Run setup script and select option 1
2. Wait for containers to start

**Access**:

- **UI**: http://localhost:3000
- **API**: http://localhost:3443
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
4. Navigate to http://localhost:3000

**Access**:

- **UI**: http://localhost:3000 (requires token)
- **Token Page**: http://localhost:3000/set-ui-token.html
- **API**: http://localhost:3443 (requires authentication)
- **Debugger**: localhost:40000

**Available Organizations**:

- `11009103` (numeric org)
- `example` (text org)
- `redhat.com` (domain org)

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

## Development Workflow

### Making Code Changes

1. **Edit code** in your local repositories
2. **Rebuild containers** if needed:
   ```bash
   ./debug-with-staging-complete.sh
   ```
3. **Restart environment** to pick up changes

### Adding New Organizations (auth=local)

Edit `generate-user-tokens.sh` and add new token generation blocks:

```bash
echo "📄 ORG: new-org (description)"
TOKEN_NEW=$(cd "$API_REPO" && ./bin/planner sso token --private-key "$PRIVATE_KEY" --username testuser --org new-org)
echo "export TOKEN_NEW='$TOKEN_NEW'"
```

## Files Reference

### Scripts

- **debug-with-staging-complete.sh**: Main setup and orchestration
- **generate-user-tokens.sh**: Creates JWT tokens for different organizations
- **restart-api.sh**: Quickly restart API container after debugging
- **sync-staging-via-gabi.sh**: Download staging data from GABI
- **test-gabi-response.sh**: Validate GABI token access

### Configuration Files

– **.env-debug (root)**: Shared DB/backend configuration for both modes
– **auth_none/debug-compose.yaml**: No-auth Docker Compose (reads ../.env-debug)
– **auth_local/debug-compose-local-file.yaml**: Local-auth Docker Compose (reads ../.env-debug)
– **auth\_\*/Dockerfile.ui**: UI container definitions
– **authFetch.ts**: JWT authentication wrapper for UI (local auth only)
– **webpack-proxy.patch**: UI-API proxy configuration

## Contributing

When making changes:

1. Test both authentication modes
2. Verify debugger functionality
3. Update README.md if adding new features
4. Ensure backward compatibility with existing workflows

## Support

For issues related to:

- **Migration Planner API**: Check the main repository
- **Migration Planner UI**: Check the UI repository
- **This debug environment**: Check container logs and verify prerequisites
