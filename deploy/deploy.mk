COMPOSE_FILE := $(realpath deploy/podman/compose.yaml)

deploy-db:
	@echo "🚀 Deploy DB on podman..."
	podman rm -f planner-db || true
	podman volume rm podman_planner-db || true
	podman-compose -f $(COMPOSE_FILE) up -d planner-db
	test/scripts/wait_for_postgres.sh podman
	podman exec -it planner-db psql -c 'ALTER ROLE admin WITH SUPERUSER'
	podman exec -it planner-db createdb admin || true
	podman exec -it planner-db createdb spicedb || true
	@echo "✅ DB was deployed successfully on podman."

deploy-spicedb:
	@echo "Deploy spicedb on podman..."
	@echo "Running SpiceDB migration..."
	podman run --rm \
		--network podman_planner-network \
		--name spicedb-migrate \
		-e SPICEDB_DATASTORE_ENGINE=postgres \
		-e SPICEDB_DATASTORE_CONN_URI="postgres://admin:adminpass@planner-db:5432/spicedb?sslmode=disable" \
		docker.io/authzed/spicedb \
		migrate head
	@echo "Starting SpiceDB service..."
	podman run -d \
		--network podman_planner-network \
		--name spicedb-service \
		-p 50051:50051 \
		-p 7070:9090 \
		--restart always \
		docker.io/authzed/spicedb \
		serve \
		--grpc-preshared-key foobar \
		--datastore-engine postgres \
		--datastore-conn-uri "postgres://admin:adminpass@planner-db:5432/spicedb?sslmode=disable" \
		--telemetry-endpoint=""
	@echo "Writing schema to SpiceDB..."
	@count=0; \
	until podman run --rm \
		--network podman_planner-network \
		-v $(CURDIR)/deploy/spicedb/schema.zed:/schema.zed:ro \
		docker.io/authzed/zed:latest \
		schema write --endpoint=spicedb-service:50051 --insecure=true --token=foobar /schema.zed 2>/dev/null || [ $$count -eq 30 ]; do \
		count=$$((count+1)); \
		if [ $$count -ge 30 ]; then \
			echo "spicedb schema write time out"; \
			exit 1; \
		fi; \
		sleep 1; \
	done
	@echo "SpiceDB deployed successfully on podman."

kill-spicedb:
	@echo "Remove spicedb..."
	podman rm -f spicedb-service || true
	podman rm -f spicedb-migrate || true
	@echo "SpiceDB removed successfully from podman."

kill-db:
	@echo "🗑️ Remove DB instance from podman..."
	podman-compose -f $(COMPOSE_FILE) down planner-db
	@echo "✅ DB instance was removed successfully from podman."

.PHONY: deploy-db kill-db deploy-spicedb kill-spicedb
