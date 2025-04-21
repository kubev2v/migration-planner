E2E_PRIVATE_KEY_FOLDER_PATH ?= /etc/planner/e2e
E2E_CLUSTER_NAME ?= kind-e2e

.PHONY: deploy-e2e-environment
deploy-e2e-environment: install_qemu_img ignore_insecure_registry create_kind_e2e_cluster setup_libvirt generate_private_key deploy_registry deploy_vcsim build_assisted_migration_containers deploy_assisted_migration

.PHONY: install_qemu_img
install_qemu_img:
	@if [ "$(PKG_MANAGER)" = "apt" ]; then \
		sudo apt update -y && sudo apt install -y qemu-utils; \
	elif [ "$(PKG_MANAGER)" = "dnf" ]; then \
		sudo dnf install -y qemu-img; \
	fi

.PHONY: ignore_insecure_registry
ignore_insecure_registry:
	echo '{' > daemon.json
	echo '  "insecure-registries": ["${INSECURE_REGISTRY}"]' >> daemon.json
	echo '}' >> daemon.json
	sudo mv daemon.json /etc/docker/daemon.json
	sudo systemctl daemon-reload
	sudo systemctl restart docker

.PHONY: create_kind_e2e_cluster
create_kind_e2e_cluster:
	kind create cluster --name $(E2E_CLUSTER_NAME)

.PHONY: setup_libvirt
setup_libvirt:
	@if [ "$(PKG_MANAGER)" = "apt" ]; then \
		sudo apt update && sudo apt install -y sshpass libvirt-dev libvirt-daemon libvirt-daemon-system; \
	elif [ "$(PKG_MANAGER)" = "dnf" ]; then \
		sudo dnf install -y sshpass libvirt-devel libvirt-daemon libvirt-daemon-config-network; \
	fi
	sudo systemctl restart libvirtd

.PHONY: generate_private_key
generate_private_key:
	sudo mkdir -p $(E2E_PRIVATE_KEY_FOLDER_PATH) && \
	sudo chown -R $(shell whoami):$(shell whoami) $(E2E_PRIVATE_KEY_FOLDER_PATH); \
	if [ ! -f $(E2E_PRIVATE_KEY_FOLDER_PATH)/private-key ]; then \
		openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out $(E2E_PRIVATE_KEY_FOLDER_PATH)/private-key; \
		openssl rsa -in $(E2E_PRIVATE_KEY_FOLDER_PATH)/private-key -out $(E2E_PRIVATE_KEY_FOLDER_PATH)/private-key -traditional; \
	fi

.PHONY: deploy_registry
deploy_registry: oc
	oc create deployment registry --image=docker.io/registry
	oc rollout status deployment/registry --timeout=60s
	oc wait --for=condition=Ready pods --all --timeout=240s
	oc port-forward --address 0.0.0.0 deploy/registry 5000:5000 > /dev/null 2>&1 &

.PHONY: deploy_vcsim
deploy_vcsim: oc
	oc process --local -f deploy/templates/vcsim-template.yml \
		-p APP_NAME=vcsim1 \
		-p PORT=8989 \
		-p USERNAME=core \
		-p PASSWORD=123456 | oc apply -f -

	oc process --local -f deploy/templates/vcsim-template.yml \
		-p APP_NAME=vcsim2 \
		-p PORT=8990 \
		-p USERNAME=core \
		-p PASSWORD=123456 | oc apply -f -

	oc wait --for=condition=Ready pods --all --timeout=240s
	oc port-forward --address 0.0.0.0 deploy/vcsim1 8989:8989 > /dev/null 2>&1 &
	oc port-forward --address 0.0.0.0 deploy/vcsim2 8990:8990 > /dev/null 2>&1 &

.PHONY: build_assisted_migration_containers
build_assisted_migration_containers:
	make migration-planner-agent-container
	make migration-planner-api-container
	$(PODMAN) push $(MIGRATION_PLANNER_AGENT_IMAGE)
	kind load docker-image $(MIGRATION_PLANNER_API_IMAGE) --name $(E2E_CLUSTER_NAME)
	$(PODMAN) rmi $(MIGRATION_PLANNER_API_IMAGE)

.PHONY: deploy_assisted_migration
deploy_assisted_migration: oc
	make deploy-on-kind MIGRATION_PLANNER_NAMESPACE=default PERSISTENT_DISK_DEVICE=/dev/vda
	oc wait --for=condition=Ready pods --all --timeout=240s
	sleep 30
	oc port-forward --address 0.0.0.0 service/migration-planner-agent 7443:7443 > /dev/null 2>&1 &
	oc port-forward --address 0.0.0.0 service/migration-planner 3443:3443 > /dev/null 2>&1 &
	oc port-forward --address 0.0.0.0 service/migration-planner-image 11443:11443 > /dev/null 2>&1 &

.PHONY: undeploy-e2e-environment
undeploy-e2e-environment:
	kind delete cluster --name $(E2E_CLUSTER_NAME)
	$(PODMAN) rmi $(MIGRATION_PLANNER_AGENT_IMAGE)
