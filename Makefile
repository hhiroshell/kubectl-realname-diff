SOURCES := $(shell find . -name '*.go')
BINARY := kubectl-natty_diff

build: kubectl-natty_diff

$(BINARY): $(SOURCES)
	GO111MODULE=on CGO_ENABLED=0 go build -o $(BINARY) ./cmd/kubectl-natty-diff.go

.PHONY: clean
clean:
	rm $(BINARY)
