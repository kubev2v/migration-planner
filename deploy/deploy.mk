deploy-db:
	podman rm -f planner-db || true
	podman volume rm podman_planner-db || true
	podman volume create --opt device=tmpfs --opt type=tmpfs --opt o=nodev,noexec podman_planner-db
	cd deploy/podman && podman-compose up -d planner-db
	test/scripts/wait_for_postgres.sh podman
	podman exec -it planner-db psql -c 'ALTER ROLE admin WITH SUPERUSER'
	podman exec -it planner-db createdb admin || true

kill-db:
	cd deploy/podman && podman-compose down planner-db

.PHONY: deploy-db kill-db
