SOURCES := $(shell find . -name '*.go')
BINARY := kubectl-realname_diff

build: kubectl-realname_diff

$(BINARY): $(SOURCES)
	GO111MODULE=on CGO_ENABLED=0 go build -o $(BINARY) ./cmd/kubectl-realname-diff.go

.PHONY: clean
clean:
	rm $(BINARY)
