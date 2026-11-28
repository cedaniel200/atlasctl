SHELL := /bin/sh

VERSION ?= 0.6.0
DIST ?= dist
COMMANDS_PKG ?= github.com/cedaniel200/atlasctl/internal/commands
PLATFORMS ?= darwin/arm64 linux/amd64
LDFLAGS := -s -w -X $(COMMANDS_PKG).EngineVersion=$(VERSION)
HOST_OS ?= $(shell uname -s | tr '[:upper:]' '[:lower:]')
HOST_ARCH ?= $(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
SHASUM := $(shell if command -v sha256sum >/dev/null 2>&1; then echo sha256sum; else echo "shasum -a 256"; fi)

.PHONY: dist verify clean

dist:
	@rm -rf "$(DIST)"
	@mkdir -p "$(DIST)"
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		stage="$(DIST)/atlasctl_$(VERSION)_$${os}_$${arch}"; \
		mkdir -p "$$stage"; \
		echo "building $$os/$$arch"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -trimpath \
			-ldflags "$(LDFLAGS)" -o "$$stage/atlasctl" ./cmd/atlasctl; \
		tar -czf "$(DIST)/atlasctl_$(VERSION)_$${os}_$${arch}.tar.gz" -C "$$stage" atlasctl; \
	done
	@cd "$(DIST)" && $(SHASUM) atlasctl_*.tar.gz > SHA256SUMS

verify: dist
	@set -eu; \
	tmp=$$(mktemp -d "$${TMPDIR:-/tmp}/atlasctl-verify.XXXXXX"); \
	trap 'rm -rf "$$tmp"' EXIT; \
	echo "verifying checksums"; \
	(cd "$(DIST)" && $(SHASUM) -c SHA256SUMS); \
	archive="$(DIST)/atlasctl_$(VERSION)_$(HOST_OS)_$(HOST_ARCH).tar.gz"; \
	if [ ! -f "$$archive" ]; then echo "no archive for $(HOST_OS)/$(HOST_ARCH)"; exit 1; fi; \
	tar -xzf "$$archive" -C "$$tmp"; \
	echo "running extracted binary"; \
	"$$tmp/atlasctl" --help >/dev/null; \
	mkdir -p "$$tmp/project"; \
	(cd "$$tmp/project" && "$$tmp/atlasctl" create --host opencode >/dev/null); \
	(cd "$$tmp/project" && "$$tmp/atlasctl" --lang en doctor | grep -F "Binary: $(VERSION)" >/dev/null); \
	echo "checking tampered checksum"; \
	awk 'NR == 1 { sub(/^[0-9a-f]+/, "0000000000000000000000000000000000000000000000000000000000000000") } { print }' "$(DIST)/SHA256SUMS" > "$$tmp/SHA256SUMS.bad"; \
	if (cd "$(DIST)" && $(SHASUM) -c "$$tmp/SHA256SUMS.bad" >/dev/null 2>&1); then \
		echo "tampered checksum was not detected"; \
		exit 1; \
	fi; \
	echo "release packages verified"

clean:
	@rm -rf "$(DIST)"
