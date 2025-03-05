COMPOSE_FILE := $(realpath deploy/podman/compose.yaml)

deploy-db:
	podman rm -f planner-db || true
	podman volume rm podman_planner-db || true
	podman-compose -f $(COMPOSE_FILE) up -d planner-db
	test/scripts/wait_for_postgres.sh podman
	podman exec -it planner-db psql -c 'ALTER ROLE admin WITH SUPERUSER'
	podman exec -it planner-db createdb admin || true

kill-db:
	podman-compose -f $(COMPOSE_FILE) down planner-db

.PHONY: deploy-db kill-db
