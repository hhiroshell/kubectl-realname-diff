# This Makefile assumed to be used for local development.
GO ?= go
STATICCHECK ?= staticcheck
DIST_DIR := dist

.PHONY: build
build:
	$(GO) build -o $(DIST_DIR)/kubectl-realname_diff cmd/kubectl-realname_diff/main.go

.PHONY: test
test: vet fmt lint
	$(GO) test ./...

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
	rm -rf $(DIST_DIR)
