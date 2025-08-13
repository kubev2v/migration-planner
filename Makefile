GOBASE=$(shell pwd)
GOBIN=$(GOBASE)/bin
GO_BUILD_FLAGS := ${GO_BUILD_FLAGS}
ROOT_DIR := $(or ${ROOT_DIR},$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST)))))
GO_FILES := $(shell find ./ -name ".go" -not -path "./bin" -not -path "./packaging/*")
GO_CACHE := -v $${HOME}/go/migration-planner-go-cache:/opt/app-root/src/go:Z -v $${HOME}/go/migration-planner-go-cache/.cache:/opt/app-root/src/.cache:Z
TIMEOUT ?= 30m
VERBOSE ?= false
REGISTRY ?= quay.io
REGISTRY_ORG ?= kubev2v
MIGRATION_PLANNER_IMAGE_TAG ?= latest
MIGRATION_PLANNER_IMAGE_TAG := $(MIGRATION_PLANNER_IMAGE_TAG)$(if $(DEBUG_MODE),-debug)
MIGRATION_PLANNER_AGENT_IMAGE ?= $(REGISTRY)/$(REGISTRY_ORG)/migration-planner-agent
MIGRATION_PLANNER_API_IMAGE ?= $(REGISTRY)/$(REGISTRY_ORG)/migration-planner-api
MIGRATION_PLANNER_API_IMAGE_PULL_POLICY ?= Always
MIGRATION_PLANNER_NAMESPACE ?= assisted-migration
MIGRATION_PLANNER_REPLICAS ?= 1
MIGRATION_PLANNER_AUTH ?= local
MIGRATION_PLANNER_ISO_URL ?= https://mirror.openshift.com/pub/openshift-v4/dependencies/rhcos/latest/rhcos-4.19.0-x86_64-live-iso.x86_64.iso
MIGRATION_PLANNER_ISO_SHA256 ?= 6a9cf9df708e014a2b44f372ab870f873cf2db5685f9ef4518f52caa36160c36
PERSISTENT_DISK_DEVICE ?= /dev/sda
INSECURE_REGISTRY ?= "true"
DOWNLOAD_RHCOS ?= true
RHCOS_PASSWORD ?= '$$$$y$$$$j9T$$$$hUUbW8zoB.Qcmpwm4/RuK1$$$$FMtuDAxNLp3sEa2PnGiJdXr8uYbvUNPlVDXpcJim529'
IFACE ?= eth0
GREP ?= grep
PODMAN ?= podman
DOCKER_CONF ?= $(CURDIR)/docker-config
DOCKER_AUTH_FILE ?= ${DOCKER_CONF}/auth.json
PKG_MANAGER ?= apt

# OPA Configuration for eval mode
MIGRATION_PLANNER_OPA_POLICIES_FOLDER ?= $(CURDIR)/policies
FORKLIFT_POLICIES_TMP_DIR ?= /tmp/forklift-policies

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
	@echo "    generate:               regenerate all generated files"
	@echo "    tidy:                   tidy go mod"
	@echo "    lint:                   run golangci-lint"
	@echo "    build:                  run all builds"
	@echo "    clean:                  clean up all containers and volumes"
	@echo "    test:                   run unit tests"
	@echo "    run:                    run the service for development"
	@echo "    setup-opa-policies:     download OPA policies from Forklift project"
	@echo "    clean-opa-policies:     clean OPA policies directory"
	@echo "    generate:        regenerate all generated files"
	@echo "    tidy:            tidy go mod"
	@echo "    lint:            run golangci-lint"
	@echo "    build:           run all builds"
	@echo "    clean:           clean up all containers and volumes"
	@echo "    migrate:         run database migrations"
	@echo "    init:            initialize RHCOS ISO for migration planner"
	@echo "    run:             run the migration planner API service"
	@echo "    integration-test: run e2e integration tests"

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

tidy:
	git ls-files go.mod '**/*go.mod' -z | xargs -0 -I{} bash -xc 'cd $$(dirname {}) && go mod tidy'

migrate:
	MIGRATION_PLANNER_MIGRATIONS_FOLDER=$(CURDIR)/pkg/migrations/sql ./bin/planner-api migrate

init:
	MIGRATION_PLANNER_ISO_URL=$(MIGRATION_PLANNER_ISO_URL) \
	MIGRATION_PLANNER_ISO_SHA256=$(MIGRATION_PLANNER_ISO_SHA256) \
	./bin/planner-api init

run: image
	MIGRATION_PLANNER_MIGRATIONS_FOLDER=$(CURDIR)/pkg/migrations/sql \
	MIGRATION_PLANNER_OPA_POLICIES_FOLDER=$(MIGRATION_PLANNER_OPA_POLICIES_FOLDER) \
	./bin/planner-api run

run-agent:
	MIGRATION_PLANNER_OPA_POLICIES_FOLDER=$(MIGRATION_PLANNER_OPA_POLICIES_FOLDER) \
	./bin/planner-agent

image:
ifeq ($(DOWNLOAD_RHCOS), true)
	@if [ ! -f rhcos-live-iso.x86_64.iso ]; then \
		curl --silent -C - -O https://mirror.openshift.com/pub/openshift-v4/dependencies/rhcos/latest/rhcos-live-iso.x86_64.iso; \
	fi
endif

build: bin
	go build -buildvcs=false $(GO_BUILD_FLAGS) -o $(GOBIN) ./cmd/...

build-api: bin
	go build -buildvcs=false $(GO_BUILD_FLAGS) -o $(GOBIN) ./cmd/planner-api ...

build-agent: bin
	go build -buildvcs=false $(GO_BUILD_FLAGS) -o $(GOBIN) ./cmd/planner-agent ...

build-cli: bin
	go build -buildvcs=false $(GO_BUILD_FLAGS) -o $(GOBIN) ./cmd/planner ...

# rebuild container only on source changes
bin/.migration-planner-agent-container: bin Containerfile.agent go.mod go.sum $(GO_FILES)
	$(PODMAN) build . --build-arg VERSION=$(SOURCE_GIT_TAG) $(if $(DEBUG_MODE),--build-arg GCFLAGS="all=-N -l") -f Containerfile.agent $(if $(LABEL),--label "$(LABEL)") -t $(MIGRATION_PLANNER_AGENT_IMAGE):$(MIGRATION_PLANNER_IMAGE_TAG)
	if [ "$(DEBUG_MODE)" = "true" ]; then $(PODMAN) build . --build-arg BASE_IMAGE=$(MIGRATION_PLANNER_AGENT_IMAGE):$(MIGRATION_PLANNER_IMAGE_TAG) -f Containerfile.agent-debug -t $(MIGRATION_PLANNER_AGENT_IMAGE):$(MIGRATION_PLANNER_IMAGE_TAG); fi

bin/.migration-planner-api-container: bin Containerfile.api go.mod go.sum $(GO_FILES)
	$(PODMAN) build . $(if $(DEBUG_MODE),--build-arg GCFLAGS="all=-N -l") -f Containerfile.api $(if $(LABEL),--label "$(LABEL)") -t $(MIGRATION_PLANNER_API_IMAGE):$(MIGRATION_PLANNER_IMAGE_TAG)
	if [ "$(DEBUG_MODE)" = "true" ]; then $(PODMAN) build . --build-arg BASE_IMAGE=$(MIGRATION_PLANNER_API_IMAGE):$(MIGRATION_PLANNER_IMAGE_TAG) -f Containerfile.api-debug -t $(MIGRATION_PLANNER_API_IMAGE):$(MIGRATION_PLANNER_IMAGE_TAG); fi

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
	oc process -f deploy/templates/s3-secret-template.yml | oc apply -f -; \
	oc process -f deploy/templates/service-template.yml \
       -p DEBUG_MODE=$(DEBUG_MODE) \
       -p MIGRATION_PLANNER_IMAGE=$(MIGRATION_PLANNER_API_IMAGE) \
       -p MIGRATION_PLANNER_AGENT_IMAGE=$(MIGRATION_PLANNER_AGENT_IMAGE) \
       -p MIGRATION_PLANNER_REPLICAS=${MIGRATION_PLANNER_REPLICAS} \
       -p IMAGE_TAG=$(MIGRATION_PLANNER_IMAGE_TAG) \
       -p MIGRATION_PLANNER_URL=http://planner-agent-$${openshift_project}.apps.$${openshift_base_url}/api/migration-assessment \
       -p MIGRATION_PLANNER_UI_URL=http://planner-ui-$${openshift_project}.apps.$${openshift_base_url} \
       -p MIGRATION_PLANNER_IMAGE_URL=http://planner-image-$${openshift_project}.apps.$${openshift_base_url} \
	   | oc apply -f -; \
	oc expose service migration-planner-agent --name planner-agent; \
	oc expose service migration-planner-image --name planner-image; \
	echo "*** Migration Planner has been deployed successfully on Openshift ***"; \

delete-from-openshift: oc
	@openshift_base_url=$$(oc whoami --show-server | sed -E 's~https?://api\.~~; s~:[0-9]+/?$$~~'); \
	openshift_project=$$(oc project -q); \
	echo "*** Delete Migration Planner from Openshift. Project: $${openshift_project}, Base URL: $${openshift_base_url} ***"; \
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
	oc process -f deploy/templates/s3-secret-template.yml | oc delete -f -; \
	oc delete route planner-agent planner-image; \
	echo "*** Migration Planner has been deleted successfully from Openshift ***"

deploy-on-kind: oc
	@inet_ip=$$(ip addr show ${IFACE} | $(GREP) -oP '(?<=inet\s)\d+\.\d+\.\d+\.\d+'); \
		echo "*** Deploy Migration Planner on Kind. Namespace: $${MIGRATION_PLANNER_NAMESPACE}, inet_ip: $${inet_ip}, PERSISTENT_DISK_DEVICE: $${PERSISTENT_DISK_DEVICE} ***"; \
	oc process --local -f  deploy/templates/pk-secret-template.yml \
		-p E2E_PRIVATE_KEY_BASE64=$(shell base64 -w 0 $(E2E_PRIVATE_KEY_FOLDER_PATH)/private-key) \
		| oc apply -n "${MIGRATION_PLANNER_NAMESPACE}" -f -; \
	oc process --local -f  deploy/templates/postgres-template.yml | oc apply -n "${MIGRATION_PLANNER_NAMESPACE}" -f -; \
	oc process --local -f deploy/templates/s3-secret-template.yml | oc apply -n "${MIGRATION_PLANNER_NAMESPACE}" -f -; \
	oc process --local -f deploy/templates/service-template.yml \
	   -p MIGRATION_PLANNER_URL=http://$${inet_ip}:7443/api/migration-assessment \
	   -p MIGRATION_PLANNER_UI_URL=http://$${inet_ip}:3333 \
	   -p MIGRATION_PLANNER_IMAGE_URL=http://$${inet_ip}:7443/api/migration-assessment \
	   -p MIGRATION_PLANNER_API_IMAGE_PULL_POLICY=Never \
	   -p MIGRATION_PLANNER_IMAGE=$(MIGRATION_PLANNER_API_IMAGE) \
	   -p MIGRATION_PLANNER_AGENT_IMAGE=$(MIGRATION_PLANNER_AGENT_IMAGE) \
	   -p MIGRATION_PLANNER_REPLICAS=$(MIGRATION_PLANNER_REPLICAS) \
	   -p PERSISTENT_DISK_DEVICE=$(PERSISTENT_DISK_DEVICE) \
	   -p INSECURE_REGISTRY=$(INSECURE_REGISTRY) \
	   -p MIGRATION_PLANNER_AUTH=$(MIGRATION_PLANNER_AUTH) \
	   -p RHCOS_PASSWORD=${RHCOS_PASSWORD} \
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
	oc process --local -f deploy/templates/s3-secret-template.yml | oc delete -n "${MIGRATION_PLANNER_NAMESPACE}" -f -; \

deploy-local-obs:
	@podman play kube --network host deploy/observability.yml

undeploy-local-obs:
	@podman kube down deploy/observability.yml

bin:
	mkdir -p bin

clean:
	- rm -f -r bin

##################### "make lint" support start ##########################
GOLANGCI_LINT_VERSION := v1.64.8
GOLANGCI_LINT := $(GOBIN)/golangci-lint

# Download golangci-lint locally if not already present
$(GOLANGCI_LINT):
	@echo "🔍 Installing golangci-lint $(GOLANGCI_LINT_VERSION)..."
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | \
		sh -s -- -b $(CURDIR)/bin $(GOLANGCI_LINT_VERSION)
	@echo "✅ 'golangci-lint' installed successfully."

# Run linter
lint: $(GOLANGCI_LINT)
	@echo "🔍 Running golangci-lint..."
	@$(GOLANGCI_LINT) run --timeout=5m
	@echo "✅ Lint passed successfully!"
##################### "make lint" support end   ##########################

##################### "make generate" support start ##########################
MOQ := $(GOBIN)/moq

# Install moq if not already present
$(MOQ):
	@echo "📦 Installing moq..."
	@go install github.com/matryer/moq@latest
	@echo "✅ 'moq' installed successfully."

# Code generation
generate: $(MOQ)
	@echo "⚙️ Running go generate..."
	@PATH="$(GOBIN):$$PATH" go generate -v $(shell go list ./...)
	@echo "⚙️ Running mockgen script..."
	@hack/mockgen.sh
	@$(MAKE) format
	@echo "✅ Generate complete."

# Check if generate changes the repo
check-generate: generate
	@echo "🔍 Checking if generated files are up to date..."
	@git diff --quiet || (echo "❌ Detected uncommitted changes after generate. Run 'make generate' and commit the result." && git status && exit 1)
	@echo "✅ All generated files are up to date."
##################### "make generate" support end   ##########################

##################### "make format" support start ##########################
GOIMPORTS := $(GOBIN)/goimports

# Install goimports if not already available
$(GOIMPORTS):
	@echo "📦 Installing goimports..."
	@go install golang.org/x/tools/cmd/goimports@latest
	@echo "✅ 'goimports' installed successfully."

# Format Go code using gofmt and goimports
format: $(GOIMPORTS)
	@echo "🧹 Formatting Go code..."
	@gofmt -s -w .
	@$(GOIMPORTS) -w .
	@echo "✅ Format complete."

# Check that formatting does not introduce changes
check-format: format
	@echo "🔍 Checking if formatting is up to date..."
	@git diff --quiet || (echo "❌ Detected uncommitted changes after format. Run 'make format' and commit the result." && git status && exit 1)
	@echo "✅ All formatted files are up to date."
##################### "make format" support end   ##########################

##################### tests support start ##########################
GINKGO := $(GOBIN)/ginkgo
UNIT_TEST_PACKAGES := ./...
UNIT_TEST_GINKGO_OPTIONS ?= --skip e2e

# Install ginkgo if not already available
$(GINKGO):
	@echo "📦 Installing ginkgo..."
	@go install -v github.com/onsi/ginkgo/v2/ginkgo@v2.22.0
	@echo "✅ 'ginkgo' installed successfully."

# Run unit tests using ginkgo
test: $(GINKGO)
	@echo "🧪 Running Unit tests..."
	@$(GINKGO) --cover -output-dir=. -coverprofile=cover.out -v --show-node-events $(UNIT_TEST_GINKGO_OPTIONS) $(UNIT_TEST_PACKAGES)
	@echo "✅ All Unit tests passed successfully."

# Build the migration-planner without downloading the RHCOS image
build-no-rhcos:
	@DOWNLOAD_RHCOS=false make build

# Full unit test cycle: build, prepare DB, run tests, and clean up
unit-test: build-no-rhcos kill-db deploy-db migrate test kill-db

# Run integration tests using ginkgo
integration-test: $(GINKGO) build
	@echo "🧪 Running integration tests..."
	MIGRATION_PLANNER_ISO_URL=$(MIGRATION_PLANNER_ISO_URL) \
	MIGRATION_PLANNER_ISO_SHA256=$(MIGRATION_PLANNER_ISO_SHA256) \
	$(GINKGO) -focus=$(FOCUS) run test/e2e
	@echo "✅ All Integration tests passed successfully."
##################### tests support end   ##########################

validate-all: lint check-generate check-format unit-test

# OPA Policies Setup for Local Development
.PHONY: setup-opa-policies
setup-opa-policies:
	@echo "Setting up OPA policies for local development..."
	@mkdir -p $(MIGRATION_PLANNER_OPA_POLICIES_FOLDER)
	@if [ -z "$$(find $(MIGRATION_PLANNER_OPA_POLICIES_FOLDER) -name '*.rego' 2>/dev/null)" ]; then \
		echo "Downloading policies from Forklift GitHub repository..."; \
		mkdir -p $(FORKLIFT_POLICIES_TMP_DIR); \
		curl -L https://github.com/kubev2v/forklift/archive/main.tar.gz \
			| tar -xz -C $(FORKLIFT_POLICIES_TMP_DIR) \
				--wildcards '*/validation/policies/io/konveyor/forklift/vmware/*' \
				--strip-components=1 \
				--exclude='*/..*' --exclude='*/.*'; \
		if [ -d "$(FORKLIFT_POLICIES_TMP_DIR)/validation/policies/io/konveyor/forklift/vmware" ]; then \
			find $(FORKLIFT_POLICIES_TMP_DIR)/validation/policies/io/konveyor/forklift/vmware \
				-name "*.rego" ! -name "*_test.rego" -exec cp {} $(MIGRATION_PLANNER_OPA_POLICIES_FOLDER)/ \; ; \
			echo "Successfully downloaded VMware policies"; \
		else \
			echo "Failed to download policies from GitHub"; \
			exit 1; \
		fi; \
		rm -rf $(FORKLIFT_POLICIES_TMP_DIR); \
	fi
	@echo "OPA policies ready in $(MIGRATION_PLANNER_OPA_POLICIES_FOLDER)"
	@echo "Found $$(find $(MIGRATION_PLANNER_OPA_POLICIES_FOLDER) -name '*.rego' | wc -l) .rego files"

.PHONY: clean-opa-policies  
clean-opa-policies:
	@echo "Cleaning OPA policies..."
	@rm -rf $(MIGRATION_PLANNER_OPA_POLICIES_FOLDER)

# include the deployment targets
include deploy/deploy.mk
include deploy/e2e.mk

################################################################################
# Emoji Legend for Makefile Targets
#
# Action Type        | Emoji | Description
# -------------------|--------|------------------------------------------------
# Install tool        📦     Installing a dependency or binary
# Running task        ⚙️     Executing tasks like generate, build, etc.
# Linting/validation  🔍     Checking format, lint, static analysis, etc.
# Formatting          🧹     Formatting source code
# Tests               🧪     Running unit or integration tests
# Warnings/info       ⚠️     Temporary notice, caution, or pre-check
# Success/complete    ✅     Task completed successfully
# Failure/alert       ❌     An error or failure occurred
# Deploy operations   🚀     Launching or bringing up environments/services
# Teardown/cleanup    🗑️     Stopping, removing, or cleaning up resources
################################################################################
