# How add migration files and migrate the database

Assisted-Migration use [goose](https://github.com/pressly/goose) to migrate the database.

## Add a new migration file
The migration sql files are located in `pkg/migrations/sql`. 
To add a new file, either use `goose` cli or just add a new sql file with the current timestamp.

```bash
 cd pkg/migrations/sql
goose create add-test-column sql
2025/04/03 15:25:01 Created new file: 20250403132501_add_test_column.sql
```

A new file `20250403132501_add_test_column.sql` has been created in the migration folder.

## Add the migrations
Edit the new and add your migration.
For example, let's add a new column to the `keys` tables.

```sql
-- +goose Up
-- +goose StatementBegin
ALTER TABLE keys ADD COLUMN test VARCHAR(255);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE keys DROP COLUMN test;
-- +goose StatementEnd
```

> You must add a sql statement for rollback action. In this case is the statement inside gooseDown block.

## Migrate the db

### Using the cli
To use the cli, you must define 3 env vars:
```
export GOOSE_DBSTRING=postgres://user:password@localhost:5432/planner
export GOOSE_DRIVER=postgres
export GOOSE_MIGRATION_DIR=./pkg/migrations/sql
```

```bash
goose up
2025/04/03 15:30:26 OK   20250403132501_add_test_column.sql (1.02ms)
2025/04/03 15:30:26 goose: successfully migrated database to version: 20250403132501
```

To rollback use:
```
goose down
2025/04/03 15:30:57 OK   20250403132501_add_test_column.sql (1.06ms)
```

### Using the planner-api

`planner-api` automatically migrate the db at each start up.
```bash
make run
MIGRATION_PLANNER_MIGRATIONS_FOLDER=/home/cosmin/projects/migration-planner/pkg/migrations/sql ./bin/planner-api run
2025-04-03T15:31:59+02:00       info    planner-api/run.go:49   Starting API service...
2025-04-03T15:31:59+02:00       info    planner-api/run.go:50   Build from git commit: beb3d03
2025-04-03T15:31:59+02:00       info    planner-api/run.go:51   Initializing data store
2025-04-03T15:31:59+02:00       info    gorm    store/gorm.go:69        PostgreSQL information: 'PostgreSQL 12.15 on x86_64-redhat-linux-gnu, compiled by gcc (GCC) 8.5.0 20210514 (Red Hat 8.5.0-20), 64-bit'
2025-04-03T15:31:59+02:00       info    migrations/migrations.go:52     OK   20250403132501_add_test_column.sql (804.86µs)

2025-04-03T15:31:59+02:00       info    migrations/migrations.go:52     goose: successfully migrated database to version: 20250403132501

2025-04-03T15:31:59+02:00       info    image_server    imageserver/server.go:57        Initializing Image-side API server
2025-04-03T15:31:59+02:00       info    agent_server    agentserver/server.go:58        Initializing Agent-side API server
2025-04-03T15:31:59+02:00       info    api_server      api_server/server.go:68 Initializing API server
2025-04-03T15:31:59+02:00       info    metrics_server  api_server/metrics_server.go:49 serving metrics: 0.0.0.0:8080
2025-04-03T15:31:59+02:00       info    image_server    imageserver/server.go:97        Listening on [::]:11443...
2025-04-03T15:31:59+02:00       info    agent_server    agentserver/server.go:99        Listening on [::]:7443...
2025-04-03T15:31:59+02:00       info    auth    auth/auth.go:26 authentication: 'local'
2025-04-03T15:31:59+02:00       info    api_server      api_server/server.go:114        Listening on [::]:3443...
```

As we can see, the db has been migrated at the last version `20250403132501`.
