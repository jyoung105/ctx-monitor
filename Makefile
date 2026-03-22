VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
BINARY := ctx-monitor

.PHONY: build test test-race vet lint bench install clean cross

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/ctx-monitor/

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

lint: vet
	@echo "Lint passed (go vet)"

bench:
	go test -run '^$$' -bench=. -benchmem ./...

install:
	go install $(LDFLAGS) ./cmd/ctx-monitor/

clean:
	rm -f $(BINARY)
	rm -f $(BINARY)-*

cross: \
	$(BINARY)-darwin-amd64 \
	$(BINARY)-darwin-arm64 \
	$(BINARY)-linux-amd64 \
	$(BINARY)-linux-arm64

$(BINARY)-darwin-amd64:
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $@ ./cmd/ctx-monitor/

$(BINARY)-darwin-arm64:
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $@ ./cmd/ctx-monitor/

$(BINARY)-linux-amd64:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $@ ./cmd/ctx-monitor/

$(BINARY)-linux-arm64:
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $@ ./cmd/ctx-monitor/
