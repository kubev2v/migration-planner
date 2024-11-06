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
MIGRATION_PLANNER_API_IMAGE_PULL_POLICY ?= Always
MIGRATION_PLANNER_UI_IMAGE ?= quay.io/kubev2v/migration-planner-ui
INSECURE_REGISTRY ?= true
DOWNLOAD_RHCOS ?= true
KUBECTL ?= kubectl
IFACE ?= eth0
PODMAN ?= podman

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
GINKGO = $(GOBIN)/ginkgo
ginkgo: ## Download ginkgo locally if necessary.
ifeq (, $(shell which ginkgo 2> /dev/null))
	go install -v github.com/onsi/ginkgo/v2/ginkgo@v2.15.0
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
	./bin/planner-api migrate --config $(PWD)/test/config.yaml

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


# rebuild container only on source changes
bin/.migration-planner-agent-container: bin Containerfile.agent go.mod go.sum $(GO_FILES)
	$(PODMAN) build . -f Containerfile.agent -t $(MIGRATION_PLANNER_AGENT_IMAGE):latest

bin/.migration-planner-api-container: bin Containerfile.api go.mod go.sum $(GO_FILES)
	$(PODMAN) build . -f Containerfile.api -t $(MIGRATION_PLANNER_API_IMAGE):latest

migration-planner-api-container: bin/.migration-planner-api-container
migration-planner-agent-container: bin/.migration-planner-agent-container

build-containers: migration-planner-api-container migration-planner-agent-container

.PHONY: build-containers

push-containers: build-containers
	$(PODMAN) push $(MIGRATION_PLANNER_API_IMAGE):latest
	$(PODMAN) push $(MIGRATION_PLANNER_AGENT_IMAGE):latest

deploy-on-kind:
	sed 's|@MIGRATION_PLANNER_AGENT_IMAGE@|$(MIGRATION_PLANNER_AGENT_IMAGE)|g; s|@INSECURE_REGISTRY@|$(INSECURE_REGISTRY)|g; s|@MIGRATION_PLANNER_API_IMAGE_PULL_POLICY@|$(MIGRATION_PLANNER_API_IMAGE_PULL_POLICY)|g; s|@MIGRATION_PLANNER_API_IMAGE@|$(MIGRATION_PLANNER_API_IMAGE)|g' deploy/k8s/migration-planner.yaml.template > deploy/k8s/migration-planner.yaml
	$(KUBECTL) apply -f 'deploy/k8s/*-service.yaml'
	$(KUBECTL) apply -f 'deploy/k8s/*-secret.yaml'
	@config_server=$$(ip addr show ${IFACE}| grep -oP '(?<=inet\s)\d+\.\d+\.\d+\.\d+'); \
	$(KUBECTL) create secret generic migration-planner-secret --from-literal=config_server=http://$$config_server:7443 || true
	$(KUBECTL) apply -f deploy/k8s/

deploy-on-openshift:
	sed 's|@MIGRATION_PLANNER_AGENT_IMAGE@|$(MIGRATION_PLANNER_AGENT_IMAGE)|g; s|@MIGRATION_PLANNER_API_IMAGE_PULL_POLICY@|$(MIGRATION_PLANNER_API_IMAGE_PULL_POLICY)|g; s|@MIGRATION_PLANNER_API_IMAGE@|$(MIGRATION_PLANNER_API_IMAGE)|g; s|@INSECURE_REGISTRY@|$(INSECURE_REGISTRY)|g' deploy/k8s/migration-planner.yaml.template > deploy/k8s/migration-planner.yaml
	sed 's|@MIGRATION_PLANNER_UI_IMAGE@|$(MIGRATION_PLANNER_UI_IMAGE)|g' deploy/k8s/migration-planner-ui.yaml.template > deploy/k8s/migration-planner-ui.yaml
	ls deploy/k8s | awk '/secret|service/' | xargs -I {} oc apply -f deploy/k8s/{}
	oc create route edge planner --service=migration-planner-ui || true
	oc expose service migration-planner-agent --name planner-agent || true
	@config_server=$$(oc get route planner-agent -o jsonpath='{.spec.host}'); \
	oc create secret generic migration-planner-secret --from-literal=config_server=http://$$config_server || true
	ls deploy/k8s | awk '! /secret|service|template/' | xargs -I {} oc apply -f deploy/k8s/{}

undeploy-on-openshift:
	oc delete route planner || true
	oc delete route planner-agent || true
	oc delete secret migration-planner-secret || true
	oc delete -f deploy/k8s || true

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
