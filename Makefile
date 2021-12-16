SOURCES := $(shell find . -name '*.go')
BINARY := kubectl-yourname_diff

build: kubectl-yourname_diff

$(BINARY): $(SOURCES)
	GO111MODULE=on CGO_ENABLED=0 go build -o $(BINARY) ./cmd/kubectl-yourname-diff.go

.PHONY: clean
clean:
	rm $(BINARY)
