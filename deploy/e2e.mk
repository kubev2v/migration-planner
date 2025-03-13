.PHONY: deploy-e2e-environment
deploy-e2e-environment: install_qemu_img ignore_insecure_registry create_kind_e2e_cluster setup_libvirt deploy_registry deploy_vcsim build_assisted_migration_containers deploy_assisted_migration persistent_disk

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
	kind create cluster --name kind-e2e

.PHONY: setup_libvirt
setup_libvirt:
	@if [ "$(PKG_MANAGER)" = "apt" ]; then \
		sudo apt update && sudo apt install -y sshpass libvirt-dev libvirt-daemon libvirt-daemon-system; \
	elif [ "$(PKG_MANAGER)" = "dnf" ]; then \
		sudo dnf install -y sshpass libvirt-devel libvirt-daemon libvirt-daemon-config-network; \
	fi
	sudo systemctl restart libvirtd

.PHONY: deploy_registry
deploy_registry:
	$(KUBECTL) create deployment registry --image=docker.io/registry
	$(KUBECTL) rollout status deployment/registry --timeout=60s
	$(KUBECTL) wait --for=condition=Ready pods --all --timeout=240s
	$(KUBECTL) port-forward --address 0.0.0.0 deploy/registry 5000:5000 &

.PHONY: deploy_vcsim
deploy_vcsim:
	sed "s|@APP-NAME@|"vcsim1"|g; \
		  s|@PORT@|"8989"|g" \
			deploy/k8s/vcsim.yaml.template > deploy/k8s/vcsim.yaml
	$(KUBECTL) apply -f 'deploy/k8s/vcsim.yaml'

	sed "s|@APP-NAME@|"vcsim2"|g; \
		  s|@PORT@|"8990"|g" \
			deploy/k8s/vcsim.yaml.template > deploy/k8s/vcsim.yaml
	$(KUBECTL) apply -f 'deploy/k8s/vcsim.yaml'

	$(KUBECTL) wait --for=condition=Ready pods --all --timeout=240s
	$(KUBECTL) port-forward --address 0.0.0.0 deploy/vcsim1 8989:8989 &
	$(KUBECTL) port-forward --address 0.0.0.0 deploy/vcsim2 8990:8990 &

.PHONY: build_assisted_migration_containers
build_assisted_migration_containers:
	make migration-planner-agent-container
	make migration-planner-api-container
	$(PODMAN) push $(MIGRATION_PLANNER_AGENT_IMAGE)
	kind load docker-image $(MIGRATION_PLANNER_API_IMAGE) --name kind-e2e
	$(PODMAN) rmi $(MIGRATION_PLANNER_API_IMAGE)

.PHONY: deploy_assisted_migration
deploy_assisted_migration:
	make deploy-on-kind MIGRATION_PLANNER_NAMESPACE=default PERSISTENT_DISK_DEVICE=/dev/vda
	$(KUBECTL) wait --for=condition=Ready pods --all --timeout=240s
	sleep 30
	$(KUBECTL) port-forward --address 0.0.0.0 service/migration-planner-agent 7443:7443 &
	$(KUBECTL) port-forward --address 0.0.0.0 service/migration-planner 3443:3443 &

.PHONY: persistent_disk
persistent_disk:
	mkdir -p /tmp/untarova
	qemu-img convert -f vmdk -O qcow2 data/persistence-disk.vmdk /tmp/untarova/persistence-disk.qcow2
	cp /tmp/untarova/persistence-disk.qcow2 /tmp/untarova/persistence-disk-vm-2.qcow2

.PHONY: undeploy-e2e-environment
undeploy-e2e-environment:
	kind delete cluster --name kind-e2e
	rm ~/.config/planner/client.yaml
	$(PODMAN) rmi $(MIGRATION_PLANNER_AGENT_IMAGE)
