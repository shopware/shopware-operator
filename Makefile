# Image URL to use all building/pushing image targets
IMG ?= ghcr.io/shopware/shopware-operator:main
IMG_REPO ?= ghcr.io/shopware/shopware-operator
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.28.0
TAG ?= v0.0.1
TAG_REGEX := ^v([0-9]{1,}\.){2}[0-9]{1,}$$

# In which namespace should the operator run
NAMESPACE ?= default

## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ZAP_PRETTY ?= $(LOCALBIN)/zap-pretty
ENVTEST ?= $(LOCALBIN)/setup-envtest
HELMIFY ?= $(LOCALBIN)/helmify
MYSQLSH ?= $(LOCALBIN)/mysqlsh/bin/mysqlsh
YQ ?= $(LOCALBIN)/yq
GOLICENSES ?= $(LOCALBIN)/go-licenses

## Tool Versions
KUSTOMIZE_VERSION ?= v5.2.1
CONTROLLER_TOOLS_VERSION ?= v0.17.0
ZAP_PRETTY_VERSION ?= v0.3.0
HELMIFY_VERSION ?= v0.4.11
MYSQLSH_VERSION ?= 8.4.6
YQ_VERSION ?= 4.44.2

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif
branch := $(shell git rev-parse --abbrev-ref HEAD)

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: licenses
licenses: path go-licenses
	mkdir -p $(path)
	@cd cmd; \
	$(GOLICENSES) report . --template ../build/licenses.tpl > ../tpl.md
	mv tpl.md $(path)/third-party-licenses.md

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=shopware-operator crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: test
test: manifests generate envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test ./... -coverprofile cover.out

GOLANGCI_LINT = $(shell pwd)/bin/golangci-lint
GOLANGCI_LINT_VERSION ?= v1.54.2
golangci-lint:
	@[ -f $(GOLANGCI_LINT) ] || { \
	set -e ;\
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell dirname $(GOLANGCI_LINT)) $(GOLANGCI_LINT_VERSION) ;\
	}

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter & yamllint
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

.PHONY: test-chart
test-chart: install
	kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/baremetal/deploy.yaml
	kubectl apply -k "github.com/minio/operator?ref=v5.0.15"
	helm install test shopware/shopware
##@ Build

.PHONY: run
run: manifests generate zap-pretty ## Run a controller from your host.
	LEADER_ELECT=false DISABLE_CHECKS=true LOG_LEVEL=debug LOG_FORMAT=zap-pretty go run ./cmd/main.go \
		2>&1 | $(ZAP_PRETTY) --all


.PHONY: snapshot-create
snapshot-create: mysqlsh
	LOG_LEVEL=debug LOG_FORMAT=zap-pretty \
	  DB_HOST=<host> \
		DB_PASSWORD=<password> \
	  DB_USER=<user> \
		AWS_ENDPOINT=s3.eu-central-1.amazonaws.com \
		AWS_PRIVATE_BUCKET=<private-bucket-name> \
		AWS_PUBLIC_BUCKET=<public-bucket-name> \
	  DB_DATABASE=shopware \
		DB_MYSQL_SHELL_BINARY_PATH=$(MYSQLSH) \
		go run cmd/snapshot/snapshot.go create \
		2>&1 | $(ZAP_PRETTY) --all

.PHONY: snapshot-restore
snapshot-restore: mysqlsh path
	LOG_LEVEL=debug LOG_FORMAT=zap-pretty \
	  DB_HOST=<host> \
		DB_PASSWORD=<password> \
	  DB_USER=<user> \
		AWS_ENDPOINT=s3.eu-central-1.amazonaws.com \
		AWS_PRIVATE_BUCKET=<private-bucket-name> \
		AWS_PUBLIC_BUCKET=<public-bucket-name> \
	  DB_DATABASE=shopware \
		DB_MYSQL_SHELL_BINARY_PATH=$(MYSQLSH) \
		go run cmd/snapshot/snapshot.go restore --backup-file $(path) \
		2>&1 | $(ZAP_PRETTY) --all

.PHONY: debug
debug: manifests generate zap-pretty ## Run a controller from your host.
	go build -gcflags="all=-N -l" ./cmd/main.go
	LEADER_ELECT=false DISABLE_CHECKS=true LOG_LEVEL=debug dlv --log --listen=:40000 --headless=true --api-version=2 --accept-multiclient exec ./main

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build . -t ${IMG} -f ./build/Dockerfile

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' ./build/Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name project-v3-builder
	$(CONTAINER_TOOL) buildx use project-v3-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm project-v3-builder
	rm Dockerfile.cross

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	@echo "Building CRD..."
	$(KUSTOMIZE) build config/crd > /tmp/temp-crd.yaml
	@echo "Applying CRD..."
	@if $(KUBECTL) get -f /tmp/temp-crd.yaml >/dev/null 2>&1; then \
		echo "CRD exists, replacing..."; \
		$(KUBECTL) replace --namespace ${NAMESPACE} -f /tmp/temp-crd.yaml; \
	else \
		echo "CRD does not exist, creating..."; \
		$(KUBECTL) create --namespace ${NAMESPACE} -f /tmp/temp-crd.yaml; \
	fi
	@echo "Cleaning up..."
	rm -f /tmp/temp-crd.yaml
	@echo "Done."

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: build
build: ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	goreleaser release --snapshot --clean
	# Doesn't overwrite the image
	kind load docker-image --name helm-chart $(IMG_REPO):$(branch)-amd64

.PHONY: deploy
deploy: install manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	kubectl config use-context kind-helm-chart
	make helm path=temp version=0.0.0
	@helm template shopware-operator temp \
		--namespace shopware \
		--set crds.install=false \
		--set image.tag=$(branch)-amd64 \
		--set image.pullPolicy=Always > temp/helm.yaml
	kubectl apply -f temp/helm.yaml

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: helm
helm: path version manifests kustomize yq ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	echo Create version $(version)
	rm -r $(path) 2> /dev/null || true
	cp -r helm $(path)
	$(KUSTOMIZE) build config/crd > $(path)/templates/crds/all.yaml
	$(KUSTOMIZE) build config/helm > $(path)/templates/operator.yaml
	$(YQ) e -i '.appVersion = "$(version)"' $(path)/Chart.yaml
	$(YQ) e -i '.version = "$(version)"' $(path)/Chart.yaml
	sed -i 's|image: ghcr.io/shopware/shopware-operator-snapshot:main|image: ghcr.io/shopware/shopware-operator-snapshot:'${version}'|g' $(path)/templates/crds/all.yaml
	$(YQ) $(path)/templates/crds/all.yaml -s '"$(path)/templates/crds/" + .spec.names.kind' --no-doc
	rm $(path)/templates/crds/all.yaml
	rm $(path)/templates/.gitkeep
	rm $(path)/templates/crds/.gitkeep
	@for file in $(path)/templates/crds/*.yml; do \
		if [ -f "$$file" ]; then \
			echo "{{- if .Values.crds.install }}" > "$$file.tmp"; \
			cat "$$file" >> "$$file.tmp"; \
			echo "{{- end }}" >> "$$file.tmp"; \
			mv "$$file.tmp" "$$file"; \
			echo "Modified $$file"; \
		fi \
	done
	sed -i '1i {{- if not .Values.crds.installOnly }}' $(path)/templates/operator.yaml
	echo "{{- end }}" >> $(path)/templates/operator.yaml

.PHONY: resources
resources: path manifests kustomize yq ## Create crd's and manager for a direct kubectl apply
	mkdir -p $(path)
	$(KUSTOMIZE) build config/crd > $(path)/crd.yaml
	$(KUSTOMIZE) build config/default > $(path)/manager.yaml
	$(YQ) eval -i 'select(.kind == "Deployment") | .spec.template.spec.containers[0].image = "ghcr.io/shopware/shopware-operator:$(shell git describe --tags --abbrev=0)"' release/manager.yaml
	echo "---" >> $(path)/manager.yaml
	$(KUSTOMIZE) build config/rbac >> $(path)/manager.yaml
	sed -i '/namespace: default/d' $(path)/manager.yaml

##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary. If wrong version is installed, it will be removed before downloading.
$(KUSTOMIZE): $(LOCALBIN)
	@if test -x $(LOCALBIN)/kustomize && ! $(LOCALBIN)/kustomize version | grep -q $(KUSTOMIZE_VERSION); then \
		echo "$(LOCALBIN)/kustomize version is not expected $(KUSTOMIZE_VERSION). Removing it before installing."; \
		rm -rf $(LOCALBIN)/kustomize; \
	fi
	test -s $(LOCALBIN)/kustomize || GOBIN=$(LOCALBIN) GO111MODULE=on go install sigs.k8s.io/kustomize/kustomize/v5@$(KUSTOMIZE_VERSION)

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary. If wrong version is installed, it will be overwritten.
$(CONTROLLER_GEN): $(LOCALBIN)
	test -s $(LOCALBIN)/controller-gen && $(LOCALBIN)/controller-gen --version | grep -q $(CONTROLLER_TOOLS_VERSION) || \
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: zap-pretty
zap-pretty: $(ZAP_PRETTY) ## Download controller-gen locally if necessary. If wrong version is installed, it will be overwritten.
$(ZAP_PRETTY): $(LOCALBIN)
	test -s $(LOCALBIN)/zap-pretty || \
	GOBIN=$(LOCALBIN) go install github.com/maoueh/zap-pretty@$(ZAP_PRETTY_VERSION)

.PHONY: yq
yq: $(YQ) ## Download locally if necessary. If wrong version is installed, it will be overwritten.
$(YQ): $(LOCALBIN)
	test -s $(LOCALBIN)/yq && $(LOCALBIN)/yq --version | grep -q $(YQ_VERSION) || \
	GOBIN=$(LOCALBIN) go install github.com/mikefarah/yq/v4@v$(YQ_VERSION)

.PHONY: go-licenses
go-licenses: $(GOLICENSES) ## Download locally if necessary. If wrong version is installed, it will be overwritten.
$(GOLICENSES): $(LOCALBIN)
	test -s $(LOCALBIN)/go-licenses || \
	GOBIN=$(LOCALBIN) go install github.com/google/go-licenses@v1.6.0

.PHONY: helmify
helmify: $(HELMIFY) ## Download locally if necessary. If wrong version is installed, it will be overwritten.
$(HELMIFY): $(LOCALBIN)
	test -s $(LOCALBIN)/helmify && $(LOCALBIN)/helmify --version | grep -q $(HELMIFY_VERSION) || \
	GOBIN=$(LOCALBIN) go install github.com/arttor/helmify/cmd/helmify@$(HELMIFY_VERSION)

.PHONY: mysqlsh
mysqlsh: $(MYSQLSH) ## Download locally if necessary. If wrong version is installed, it will be overwritten.
$(MYSQLSH): $(LOCALBIN)
	test -s $(LOCALBIN)/mysqlsh/bin/mysqlsh && $(LOCALBIN)/mysqlsh/bin/mysqlsh --version | grep -q $(MYSQLSH_VERSION) || \
	(echo "Binary mysqlsh not found or wrong version. Downloading..." && \
	wget https://dev.mysql.com/get/Downloads/MySQL-Shell/mysql-shell-8.4.6-linux-glibc2.28-x86-64bit.tar.gz -O /home/patrick/dev/go/src/github.com/shopware/shopware-operator/bin/mysql-shell.tar.gz && \
	mkdir -p $(LOCALBIN)/mysqlsh && \
	tar -xzf $(LOCALBIN)/mysql-shell.tar.gz -C $(LOCALBIN)/mysqlsh --strip-components=1)

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	test -s $(LOCALBIN)/setup-envtest || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

.PHONY: path
path:
ifeq (, $(path))
	$(error path option is required for this command: path=<path>)
endif

.PHONY: version
version:
ifeq (, $(version))
	$(error version option is required for this command: version=<version>)
endif
