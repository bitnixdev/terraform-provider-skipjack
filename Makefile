HOSTNAME=registry.terraform.io
NAMESPACE=bitnixdev
NAME=skipjack
BINARY=terraform-provider-${NAME}

# Auto-version: YYYY.MM.DD.<revid> (UTC date + commit count).
# Override with VERSION=... for one-off local installs.
REVID ?= $(shell git rev-list --count HEAD 2>/dev/null || echo 0)
VERSION ?= $(shell date -u +%Y.%m.%d).$(REVID)

OS_ARCH?=$(shell go env GOOS)_$(shell go env GOARCH)

default: build

.PHONY: version
version:
	@echo $(VERSION)

.PHONY: build
build:
	go build -o ${BINARY} -ldflags "-X main.version=${VERSION}"

.PHONY: install
install: build
	mkdir -p ~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/${OS_ARCH}
	cp ${BINARY} ~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/${OS_ARCH}

.PHONY: test
test:
	go test ./... -count=1

.PHONY: testacc
testacc:
	TF_ACC=1 go test ./... -v -count=1 -timeout 120m

.PHONY: fmt
fmt:
	gofmt -w .

.PHONY: vet
vet:
	go vet ./...

.PHONY: tidy
tidy:
	go mod tidy

# Local multi-arch package dry-run (does not publish or sign for release).
.PHONY: release-snapshot
release-snapshot:
	goreleaser release --snapshot --clean --skip=sign
