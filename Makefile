BINARY  := p1mon-imitator
PKG     := ./cmd/p1mon-imitator
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

# CGO_ENABLED=0 gives a static binary with no libc dependency.
# -s -w strips symbols and DWARF, -trimpath removes local paths.
export CGO_ENABLED := 0
GOFLAGS := -trimpath
LDFLAGS := -s -w -X main.version=$(VERSION)

PREFIX ?= /usr/local

.PHONY: all build amd64 arm64 test vet fmt clean install

all: amd64 arm64

build:
	go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY) $(PKG)

amd64:
	GOOS=linux GOARCH=amd64 go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY)-linux-amd64 $(PKG)

arm64:
	GOOS=linux GOARCH=arm64 go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY)-linux-arm64 $(PKG)

test:
	go test ./...

vet:
	go vet ./...
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo 'gofmt: files need formatting'; exit 1; }

fmt:
	gofmt -w .

install: build
	install -m 0755 $(BINARY) $(PREFIX)/bin/$(BINARY)
	install -m 0644 systemd/$(BINARY).service /etc/systemd/system/$(BINARY).service
	systemctl daemon-reload

clean:
	rm -f $(BINARY) $(BINARY)-linux-amd64 $(BINARY)-linux-arm64
