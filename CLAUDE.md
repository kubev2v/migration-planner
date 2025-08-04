# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Migration Planner is a service that helps assess VMware infrastructure and provides suggestions for migration to OpenShift Virtualization. It consists of:

- **API Service**: Manages assessment reports and generates OVA images
- **Agent**: Deployed as OVA in VMware environments to collect anonymous infrastructure data
- **CLI Tool**: Command-line interface for API operations

## Technology Stack

- **Language**: Go 1.24.0+
- **Database**: PostgreSQL (primary), SQLite (alternative)
- **Web Framework**: Chi router
- **ORM**: GORM
- **Authentication**: JWT tokens, Red Hat SSO
- **API**: OpenAPI 3.0 with code generation
- **Containers**: Podman/Docker with Red Hat UBI9 base images

## Common Development Commands

```bash
# Build all components
make build

# Build specific components
make build-api      # API service
make build-agent    # Migration agent
make build-cli      # CLI tool

# Development workflow
make deploy-db      # Start local PostgreSQL
make migrate        # Run database migrations
make run           # Start API service locally

# Testing
make test                # Unit tests
make integration-test    # Integration tests

# Code generation and linting
make generate      # Generate OpenAPI code and mocks
make lint         # Run linters

# Container operations
make build-containers
make push-containers
```

## Development Environment Setup

1. Set required environment variables:
   ```bash
   export MIGRATION_PLANNER_AUTH=none
   export MIGRATION_PLANNER_AGENT_AUTH_ENABLED=false
   ```

2. Database configuration (localhost:5432):
   - Database: `migration_planner`
   - User: `migration_planner`
   - Password: `migration_planner`

## Architecture

### Service Architecture
```
Frontend UI ──► API Service (port 3443) ──► PostgreSQL
                    │
                    ▼ Agent Communication (port 7443)
              Migration Agent (port 3333)
                    │
                    ▼
              VMware vCenter
```

### Key Directories
- `/cmd/` - Main applications (planner-api, planner-agent, planner CLI)
- `/internal/` - Internal packages (agent, api_server, auth, handlers, store)
- `/api/v1alpha1/` - OpenAPI specs and generated code
- `/pkg/` - Reusable packages (iso, log, metrics, migrations)
- `/deploy/` - Deployment configurations
- `/test/` - E2E and integration tests

### Database Models
Core entities managed by GORM:
- `Source` - VMware assessment sources
- `Inventory` - Collected infrastructure data
- Agent status and metadata

## Testing Configuration

- Test database: PostgreSQL on localhost:5432
- API endpoint: localhost:3443
- Agent endpoint: localhost:7443
- Test framework: Ginkgo/Gomega with gomock

## Authentication Modes

- `none` - No authentication (development)
- `local` - Local JWT tokens
- `RHSSO` - Red Hat SSO integration

## Code Generation

The project uses OpenAPI-driven code generation:
- API models and handlers generated from `/api/v1alpha1/openapi.yaml`
- Mock interfaces generated with gomock
- Run `make generate` after API spec changes

## Container Images

- `quay.io/kubev2v/migration-planner-api:latest`
- `quay.io/kubev2v/migration-planner-agent:latest`

## Data Collection

The agent collects anonymous VMware infrastructure data including datastores, hosts, networks, clusters, datacenters, VMs, and general vCenter information for migration assessment purposes.
