# This Makefile assumed to be used for local development.
GO ?= go
STATICCHECK ?= staticcheck
DIST_DIR := dist
TESTBIN_DIR := testbin
ENVTEST = $(shell pwd)/bin/setup-envtest
SETUP_ENVTEST_VERSION ?= release-0.22
ENVTEST_K8S_VERSION = 1.34.0
ENVTEST_ASSETS_DIR = $(TESTBIN_DIR)/k8s/$(ENVTEST_K8S_VERSION)-$(shell go env GOOS)-$(shell go env GOARCH)
VERSION ?= dev

.PHONY: build
build:
	$(GO) build -ldflags "-X github.com/hhiroshell/kubectl-realname-diff/pkg/version.Version=$(VERSION)" -o $(DIST_DIR)/kubectl-realname_diff cmd/kubectl-realname_diff/main.go

.PHONY: test
test: vet fmt lint
	$(GO) test ./...

.PHONY: test-integration
test-integration: envtest
	KUBEBUILDER_ASSETS="$(shell pwd)/$(ENVTEST_ASSETS_DIR)" $(GO) test -tags=integration -v ./pkg/cmd/...

.PHONY: test-all
test-all: test test-integration

.PHONY: envtest
envtest: $(ENVTEST)
	@echo "Setting up envtest binaries..."
	@mkdir -p $(TESTBIN_DIR)
	$(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(TESTBIN_DIR) -p path

$(ENVTEST):
	@mkdir -p bin
	GOBIN=$(shell pwd)/bin $(GO) install sigs.k8s.io/controller-runtime/tools/setup-envtest@$(SETUP_ENVTEST_VERSION)

.PHONY: vet
vet:
	$(GO) vet ./...

.PHONY: fmt
fmt:
	$(GO) fmt ./...

.PHONY: lint
lint:
	$(STATICCHECK) ./...

.PHONY: clean
clean:
	rm -rf $(DIST_DIR) $(TESTBIN_DIR) bin/
