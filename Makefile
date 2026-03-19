BINARY    = neukeiho
VERSION   = v0.1.0
LDFLAGS   = -ldflags "-X main.version=$(VERSION) -s -w"
BUILD_DIR = dist

.PHONY: all build build-all clean install

all: build

## build: build for current OS/arch
build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/neukeiho

## build-all: cross-compile for Linux amd64 + arm64
build-all: clean
	mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) \
		-o $(BUILD_DIR)/$(BINARY)-linux-amd64 ./cmd/neukeiho
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) \
		-o $(BUILD_DIR)/$(BINARY)-linux-arm64 ./cmd/neukeiho
	@echo "✅ binaries in $(BUILD_DIR)/"
	@ls -lh $(BUILD_DIR)/

## install: install binary to /usr/bin
install: build
	sudo install -m 0755 $(BINARY) /usr/bin/$(BINARY)
	sudo mkdir -p /etc/neukeiho /var/log/neukeiho /var/lib/neukeiho
	sudo mkdir -p /usr/share/neukeiho
	sudo cp -r deploy /usr/share/neukeiho/
	@echo "✅ installed /usr/bin/$(BINARY)"
	@echo "   run: neukeiho init"

## clean: remove build artifacts
clean:
	rm -rf $(BUILD_DIR) $(BINARY)

## tidy: tidy go modules
tidy:
	go mod tidy

## test: run tests
test:
	go test ./...

## vet: run go vet
vet:
	go vet ./...
