GOBASE=$(shell pwd)
GOBIN=$(GOBASE)/bin
GO_BUILD_FLAGS := ${GO_BUILD_FLAGS}
ROOT_DIR := $(or ${ROOT_DIR},$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST)))))
GO_FILES := $(shell find ./ -name ".go" -not -path "./bin" -not -path "./packaging/*")
GO_CACHE := -v $${HOME}/go/migration-planner-go-cache:/opt/app-root/src/go:Z -v $${HOME}/go/migration-planner-go-cache/.cache:/opt/app-root/src/.cache:Z
TIMEOUT ?= 30m
VERBOSE ?= false
MIGRATION_PLANNER_AGENT_IMAGE ?= quay.io/kubev2v/migration-planner-agent
MIGRATION_PLANNER_API_IMAGE ?= quay.io/kubev2v/migration-planner-api
MIGRATION_PLANNER_IMAGE_TAG ?= latest
MIGRATION_PLANNER_IMAGE_TAG := $(MIGRATION_PLANNER_IMAGE_TAG)$(if $(DEBUG_MODE),-debug)
MIGRATION_PLANNER_API_IMAGE_PULL_POLICY ?= Always
MIGRATION_PLANNER_UI_IMAGE ?= quay.io/kubev2v/migration-planner-ui
MIGRATION_PLANNER_UI_IMAGE_TAG ?= latest
MIGRATION_PLANNER_NAMESPACE ?= assisted-migration
MIGRATION_PLANNER_REPLICAS ?= 1
MIGRATION_PLANNER_AUTH ?= local
PERSISTENT_DISK_DEVICE ?= /dev/sda
INSECURE_REGISTRY ?= "true"
DOWNLOAD_RHCOS ?= true
IFACE ?= eth0
GREP ?= grep
PODMAN ?= podman
DOCKER_CONF ?= $(CURDIR)/docker-config
DOCKER_AUTH_FILE ?= ${DOCKER_CONF}/auth.json
PKG_MANAGER ?= apt

SOURCE_GIT_TAG ?=$(shell git describe --always --long --tags --abbrev=7 --match 'v[0-9]*' || echo 'v0.0.0-unknown-$(SOURCE_GIT_COMMIT)')
SOURCE_GIT_TREE_STATE ?=$(shell ( ( [ ! -d ".git/" ] || git diff --quiet ) && echo 'clean' ) || echo 'dirty')
SOURCE_GIT_COMMIT ?=$(shell git rev-parse --short "HEAD^{commit}" 2>/dev/null)
BIN_TIMESTAMP ?=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
MAJOR := $(shell echo $(SOURCE_GIT_TAG) | awk -F'[._~-]' '{print $$1}')
MINOR := $(shell echo $(SOURCE_GIT_TAG) | awk -F'[._~-]' '{print $$2}')
PATCH := $(shell echo $(SOURCE_GIT_TAG) | awk -F'[._~-]' '{print $$3}')

GO_LD_FLAGS := -ldflags "\
	-X github.com/kubev2v/migration-planner/pkg/version.majorFromGit=$(MAJOR) \
	-X github.com/kubev2v/migration-planner/pkg/version.minorFromGit=$(MINOR) \
	-X github.com/kubev2v/migration-planner/pkg/version.patchFromGit=$(PATCH) \
	-X github.com/kubev2v/migration-planner/pkg/version.versionFromGit=$(SOURCE_GIT_TAG) \
	-X github.com/kubev2v/migration-planner/pkg/version.commitFromGit=$(SOURCE_GIT_COMMIT) \
	-X github.com/kubev2v/migration-planner/pkg/version.gitTreeState=$(SOURCE_GIT_TREE_STATE) \
	-X github.com/kubev2v/migration-planner/pkg/version.buildDate=$(BIN_TIMESTAMP) \
	-X github.com/kubev2v/migration-planner/internal/agent.version=$(SOURCE_GIT_TAG) \
	$(LD_FLAGS)"
GO_BUILD_FLAGS += $(GO_LD_FLAGS)

.EXPORT_ALL_VARIABLES:

all: build build-containers

help:
	@echo "Targets:"
	@echo "    generate:        regenerate all generated files"
	@echo "    tidy:            tidy go mod"
	@echo "    lint:            run golangci-lint"
	@echo "    build:           run all builds"
	@echo "    clean:           clean up all containers and volumes"

GOBIN = $(shell pwd)/bin
GINKGO ?= $(GOBIN)/ginkgo
ginkgo: ## Download ginkgo locally if necessary.
ifeq (, $(shell which ginkgo 2> /dev/null))
	go install -v github.com/onsi/ginkgo/v2/ginkgo@v2.15.0
endif

OS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
OC_VERSION ?= 4.17.9
OC_BIN := $(shell command -v oc)
oc: # Verify oc installed, in linux install it if not already installed
ifeq ($(OC_BIN),)
	@if [ "$(OS)" = "darwin" ]; then \
		echo "Error: macOS detected. Please install oc manually from https://mirror.openshift.com/pub/openshift-v4/clients/ocp/$(OC_VERSION)/"; \
		exit 1; \
	fi
	@echo "oc not found. Installing for Linux..."
	@curl -sL "https://mirror.openshift.com/pub/openshift-v4/clients/ocp/$(OC_VERSION)/openshift-client-linux.tar.gz" | tar -xz
	@chmod +x oc kubectl
	@sudo mv oc kubectl /usr/local/bin/
	@echo "oc installed successfully."
else
	@echo "oc is already installed at $(OC_BIN)"
endif

TEST_PACKAGES := ./...
GINKGO_OPTIONS ?= --skip e2e
test: ginkgo
	$(GINKGO) --cover -output-dir=. -coverprofile=cover.out -v --show-node-events $(GINKGO_OPTIONS) $(TEST_PACKAGES)

generate:
	go generate -v $(shell go list ./...)
	hack/mockgen.sh

tidy:
	git ls-files go.mod '**/*go.mod' -z | xargs -0 -I{} bash -xc 'cd $$(dirname {}) && go mod tidy'

lint: tools
	$(GOBIN)/golangci-lint run -v --timeout 2m

migrate:
	MIGRATION_PLANNER_MIGRATIONS_FOLDER=$(CURDIR)/pkg/migrations/sql ./bin/planner-api migrate

run:
	MIGRATION_PLANNER_MIGRATIONS_FOLDER=$(CURDIR)/pkg/migrations/sql ./bin/planner-api run

image:
ifeq ($(DOWNLOAD_RHCOS), true)
	curl --silent -C - -O https://mirror.openshift.com/pub/openshift-v4/dependencies/rhcos/latest/rhcos-live.x86_64.iso
endif

integration-test: ginkgo
	$(GINKGO) -focus=$(FOCUS) run test/e2e

build: bin image
	go build -buildvcs=false $(GO_BUILD_FLAGS) -o $(GOBIN) ./cmd/...

build-api: bin
	go build -buildvcs=false $(GO_BUILD_FLAGS) -o $(GOBIN) ./cmd/planner-api ...

build-agent: bin
	go build -buildvcs=false $(GO_BUILD_FLAGS) -o $(GOBIN) ./cmd/planner-agent ...

build-cli: bin
	go build -buildvcs=false $(GO_BUILD_FLAGS) -o $(GOBIN) ./cmd/planner ...

# rebuild container only on source changes
bin/.migration-planner-agent-container: bin Containerfile.agent go.mod go.sum $(GO_FILES)
	$(PODMAN) build . --build-arg VERSION=$(SOURCE_GIT_TAG) -f $(if $(DEBUG_MODE),Containerfile.agent-debug,Containerfile.agent) $(if $(LABEL),--label "$(LABEL)") -t $(MIGRATION_PLANNER_AGENT_IMAGE):$(MIGRATION_PLANNER_IMAGE_TAG)

bin/.migration-planner-api-container: bin Containerfile.api go.mod go.sum $(GO_FILES)
	$(PODMAN) build . -f $(if $(DEBUG_MODE),Containerfile.api-debug,Containerfile.api) $(if $(LABEL),--label "$(LABEL)") -t $(MIGRATION_PLANNER_API_IMAGE):$(MIGRATION_PLANNER_IMAGE_TAG)

migration-planner-api-container: bin/.migration-planner-api-container
migration-planner-agent-container: bin/.migration-planner-agent-container

build-containers: migration-planner-api-container migration-planner-agent-container

.PHONY: build-containers

quay-login:
	@if [ ! -f $(DOCKER_AUTH_FILE) ] && [ $(QUAY_USER) ] && [ $(QUAY_TOKEN) ]; then \
		$(info Create Auth File: $(DOCKER_AUTH_FILE)) \
		mkdir -p "$(DOCKER_CONF)"; \
		$(PODMAN) login --authfile $(DOCKER_AUTH_FILE) -u=$(QUAY_USER) -p=$(QUAY_TOKEN) quay.io; \
	fi;

push-api-container: migration-planner-api-container quay-login
	if [ -f $(DOCKER_AUTH_FILE) ]; then \
		$(PODMAN) push --authfile $(DOCKER_AUTH_FILE) $(MIGRATION_PLANNER_API_IMAGE):$(MIGRATION_PLANNER_IMAGE_TAG); \
	else \
		$(PODMAN) push $(MIGRATION_PLANNER_API_IMAGE):$(MIGRATION_PLANNER_IMAGE_TAG); \
	fi;

push-agent-container: migration-planner-agent-container quay-login
	if [ -f $(DOCKER_AUTH_FILE) ]; then \
		$(PODMAN) push --authfile=$(DOCKER_AUTH_FILE) $(MIGRATION_PLANNER_AGENT_IMAGE):$(MIGRATION_PLANNER_IMAGE_TAG); \
	else \
		$(PODMAN) push $(MIGRATION_PLANNER_AGENT_IMAGE):$(MIGRATION_PLANNER_IMAGE_TAG); \
	fi;

push-containers: push-api-container push-agent-container

deploy-on-openshift: oc
	@openshift_base_url=$$(oc whoami --show-server | sed -E 's~https?://api\.~~; s~:[0-9]+/?$$~~'); \
	openshift_project=$$(oc project -q); \
	echo "*** Deploy Migration Planner on Openshift. Project: $${openshift_project}, Base URL: $${openshift_base_url} ***";\
	oc process -f deploy/templates/postgres-template.yml | oc apply -f -; \
	oc process -f deploy/templates/service-template.yml \
       -p DEBUG_MODE=$(DEBUG_MODE) \
       -p MIGRATION_PLANNER_IMAGE=$(MIGRATION_PLANNER_API_IMAGE) \
       -p MIGRATION_PLANNER_AGENT_IMAGE=$(MIGRATION_PLANNER_AGENT_IMAGE) \
       -p MIGRATION_PLANNER_REPLICAS=${MIGRATION_PLANNER_REPLICAS} \
       -p IMAGE_TAG=$(MIGRATION_PLANNER_IMAGE_TAG) \
       -p MIGRATION_PLANNER_URL=http://planner-agent-$${openshift_project}.apps.$${openshift_base_url} \
       -p MIGRATION_PLANNER_UI_URL=http://planner-ui-$${openshift_project}.apps.$${openshift_base_url} \
       -p MIGRATION_PLANNER_IMAGE_URL=http://planner-image-$${openshift_project}.apps.$${openshift_base_url} \
	   | oc apply -f -; \
	oc process -f https://raw.githubusercontent.com/kubev2v/migration-planner-ui/refs/heads/main/deploy/templates/ui-template.yml \
     -p MIGRATION_PLANNER_UI_IMAGE=$(MIGRATION_PLANNER_UI_IMAGE) \
     -p MIGRATION_PLANNER_REPLICAS=$(MIGRATION_PLANNER_REPLICAS) \
     -p IMAGE_TAG=$(MIGRATION_PLANNER_UI_IMAGE_TAG) \
	   | oc apply -f -; \
	oc expose service migration-planner-agent --name planner-agent; \
	oc expose service migration-planner-ui --name planner-ui; \
	oc expose service migration-planner-image --name planner-image; \
	echo "*** Migration Planner has been deployed successfully on Openshift ***"; \
	echo "*** Open UI: http://planner-ui-$${openshift_project}.apps.$${openshift_base_url}"

delete-from-openshift: oc
	@openshift_base_url=$$(oc whoami --show-server | sed -E 's~https?://api\.~~; s~:[0-9]+/?$$~~'); \
	openshift_project=$$(oc project -q); \
	echo "*** Delete Migration Planner from Openshift. Project: $${openshift_project}, Base URL: $${openshift_base_url} ***"; \
	oc process -f https://raw.githubusercontent.com/kubev2v/migration-planner-ui/refs/heads/main/deploy/templates/ui-template.yml \
     -p MIGRATION_PLANNER_UI_IMAGE=$(MIGRATION_PLANNER_UI_IMAGE) \
     -p MIGRATION_PLANNER_REPLICAS=$(MIGRATION_PLANNER_REPLICAS) \
     -p IMAGE_TAG=$(MIGRATION_PLANNER_UI_IMAGE_TAG) \
	   | oc delete -f -; \
	openshift_base_url=$$(oc whoami --show-server | sed -E 's~https?://api\.~~; s~:[0-9]+/?$$~~'); \
	openshift_project=$$(oc project -q); \
	oc process -f deploy/templates/service-template.yml \
       -p MIGRATION_PLANNER_IMAGE=$(MIGRATION_PLANNER_API_IMAGE) \
       -p MIGRATION_PLANNER_AGENT_IMAGE=$(MIGRATION_PLANNER_AGENT_IMAGE) \
       -p MIGRATION_PLANNER_REPLICAS=$(MIGRATION_PLANNER_REPLICAS) \
       -p IMAGE_TAG=$(MIGRATION_PLANNER_IMAGE_TAG) \
       -p MIGRATION_PLANNER_URL=http://planner-agent-$${openshift_project}.apps.$${openshift_base_url} \
       -p MIGRATION_PLANNER_UI_URL=http://planner-ui-$${openshift_project}.apps.$${openshift_base_url} \
       -p MIGRATION_PLANNER_IMAGE_URL=http://planner-image-$${openshift_project}.apps.$${openshift_base_url} \
	   | oc delete -f -; \
	oc process -f deploy/templates/postgres-template.yml | oc delete -f -; \
	oc delete route planner-agent planner-ui planner-image; \
	echo "*** Migration Planner has been deleted successfully from Openshift ***"

deploy-on-kind: oc
	@inet_ip=$$(ip addr show ${IFACE} | $(GREP) -oP '(?<=inet\s)\d+\.\d+\.\d+\.\d+'); \
		echo "*** Deploy Migration Planner on Kind. Namespace: $${MIGRATION_PLANNER_NAMESPACE}, inet_ip: $${inet_ip}, PERSISTENT_DISK_DEVICE: $${PERSISTENT_DISK_DEVICE} ***"; \
	oc process --local -f  deploy/templates/pk-secret-template.yml \
		-p E2E_PRIVATE_KEY_BASE64=$(shell base64 -w 0 $(E2E_PRIVATE_KEY_FOLDER_PATH)/private-key) \
		| oc apply -n "${MIGRATION_PLANNER_NAMESPACE}" -f -; \
	oc process --local -f  deploy/templates/postgres-template.yml | oc apply -n "${MIGRATION_PLANNER_NAMESPACE}" -f -; \
	oc process --local -f deploy/templates/service-template.yml \
	   -p MIGRATION_PLANNER_URL=http://$${inet_ip}:7443 \
	   -p MIGRATION_PLANNER_UI_URL=http://$${inet_ip}:3333 \
	   -p MIGRATION_PLANNER_IMAGE_URL=http://$${inet_ip}:11443 \
	   -p MIGRATION_PLANNER_API_IMAGE_PULL_POLICY=Never \
	   -p MIGRATION_PLANNER_IMAGE=$(MIGRATION_PLANNER_API_IMAGE) \
	   -p MIGRATION_PLANNER_AGENT_IMAGE=$(MIGRATION_PLANNER_AGENT_IMAGE) \
	   -p MIGRATION_PLANNER_REPLICAS=$(MIGRATION_PLANNER_REPLICAS) \
	   -p PERSISTENT_DISK_DEVICE=$(PERSISTENT_DISK_DEVICE) \
	   -p INSECURE_REGISTRY=$(INSECURE_REGISTRY) \
	   -p MIGRATION_PLANNER_AUTH=$(MIGRATION_PLANNER_AUTH) \
	   | oc apply -n "${MIGRATION_PLANNER_NAMESPACE}" -f -; \
	echo "*** Migration Planner has been deployed successfully on Kind ***"

delete-from-kind: oc
	inet_ip=$$(ip addr show ${IFACE} | $(GREP) -oP '(?<=inet\s)\d+\.\d+\.\d+\.\d+'); \
	oc process --local -f deploy/templates/service-template.yml \
	   -p MIGRATION_PLANNER_URL=http://$${inet_ip}:7443 \
	   -p MIGRATION_PLANNER_UI_URL=http://$${inet_ip}:3333 \
	   -p MIGRATION_PLANNER_IMAGE_URL=http://$${inet_ip}:11443 \
	   -p MIGRATION_PLANNER_API_IMAGE_PULL_POLICY=Never \
	   -p MIGRATION_PLANNER_IMAGE=$(MIGRATION_PLANNER_API_IMAGE) \
	   -p MIGRATION_PLANNER_AGENT_IMAGE=$(MIGRATION_PLANNER_AGENT_IMAGE) \
	   -p MIGRATION_PLANNER_REPLICAS=$(MIGRATION_PLANNER_REPLICAS) \
	   -p PERSISTENT_DISK_DEVICE=$(PERSISTENT_DISK_DEVICE) \
	   -p INSECURE_REGISTRY=$(INSECURE_REGISTRY) \
	   | oc delete -n "${MIGRATION_PLANNER_NAMESPACE}" -f -; \
	oc process --local -f  deploy/templates/postgres-template.yml | oc delete -n "${MIGRATION_PLANNER_NAMESPACE}" -f -; \
	oc process --local -f deploy/templates/pk-secret-template.yml \
		-p E2E_PRIVATE_KEY_BASE64=$(shell base64 -w 0 $(E2E_PRIVATE_KEY_FOLDER_PATH)/private-key) \
		| oc delete -n "${MIGRATION_PLANNER_NAMESPACE}" -f -; \

deploy-local-obs:
	@podman play kube --network host deploy/observability.yml

undeploy-local-obs:
	@podman kube down deploy/observability.yml

bin:
	mkdir -p bin

clean:
	- rm -f -r bin

.PHONY: tools
tools: $(GOBIN)/golangci-lint

$(GOBIN)/golangci-lint:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOBIN) v1.54.0

# include the deployment targets
include deploy/deploy.mk
include deploy/e2e.mk
