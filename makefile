MODULE := github.com/Asutorufa/tunnel

BUILD_COMMIT  := $(shell git rev-parse --short HEAD)
BUILD_VERSION := $(shell git describe --abbrev=0 --tags HEAD)
BUILD_ARCH	:= $(shell uname -a)
BUILD_TIME	:= $(shell date)
CGO_ENABLED := 0

GO=$(shell command -v go | head -n1)

GO_LDFLAGS= -s -w -buildid=
GO_LDFLAGS += -X "$(MODULE)/internal/version.Version=$(BUILD_VERSION)"
GO_LDFLAGS += -X "$(MODULE)/internal/version.GitCommit=$(BUILD_COMMIT)"
GO_LDFLAGS += -X "$(MODULE)/internal/version.BuildArch=$(BUILD_ARCH)"
GO_LDFLAGS += -X "$(MODULE)/internal/version.BuildTime=$(BUILD_TIME)"

GO_GCFLAGS=

GO_BUILD_CMD=CGO_ENABLED=$(CGO_ENABLED) $(GO) build -ldflags='$(GO_LDFLAGS)' -gcflags='$(GO_GCFLAGS)' -trimpath

# AMD64v3 https://github.com/golang/go/wiki/MinimumRequirements#amd64
LINUX_AMD64=GOOS=linux GOARCH=amd64
LINUX_AMD64v3=GOOS=linux GOARCH=amd64 GOAMD64=v3
DARWIN_AMD64=GOOS=darwin GOARCH=amd64
DARWIN_AMD64v3=GOOS=darwin GOARCH=amd64 GOAMD64=v3
WINDOWS_AMD64=GOOS=windows GOARCH=amd64
WINDOWS_AMD64v3=GOOS=windows GOARCH=amd64 GOAMD64=v3

SERVER=-v ./cmd/server/...
CLIENT=-v ./cmd/client/...

.PHONY: linux
linux:
	$(LINUX_AMD64) $(GO_BUILD_CMD) -o server_linux_amd64 $(SERVER)
	$(LINUX_AMD64v3) $(GO_BUILD_CMD) -o server_linux_amd64v3 $(SERVER)
	$(LINUX_AMD64) $(GO_BUILD_CMD) -o client_linux_amd64 $(CLIENT)
	$(LINUX_AMD64v3) $(GO_BUILD_CMD) -o client_linux_amd64v3 $(CLIENT)

.PHONY: windows
windows:
	$(WINDOWS_AMD64) $(GO_BUILD_CMD) -o server_windows_amd64.exe $(SERVER)
	$(WINDOWS_AMD64v3) $(GO_BUILD_CMD) -o server_windows_amd64v3.exe $(SERVER)
	$(WINDOWS_AMD64) $(GO_BUILD_CMD) -o client_windows_amd64.exe $(CLIENT)
	$(WINDOWS_AMD64v3) $(GO_BUILD_CMD) -o client_windows_amd64v3.exe $(CLIENT)

.PHONY: darwin
darwin:

	$(DARWIN_AMD64) $(GO_BUILD_CMD) -o server_darwin_amd64 $(SERVER)
	$(DARWIN_AMD64v3) $(GO_BUILD_CMD) -o server_darwin_amd64v3 $(SERVER)
	$(DARWIN_AMD64) $(GO_BUILD_CMD) -o client_darwin_amd64 $(CLIENT)
	$(DARWIN_AMD64v3) $(GO_BUILD_CMD) -o client_darwin_amd64v3 $(CLIENT)
