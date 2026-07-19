HOSTNAME=registry.terraform.io
NAMESPACE=bitnixdev
NAME=skipjack
BINARY=terraform-provider-${NAME}

# Auto-version as SemVer-compatible CalVer: YYYY.MMDD.<revid>
# (year / month*100+day / commit count). Override with VERSION=... locally.
REVID ?= $(shell git rev-list --count HEAD 2>/dev/null || echo 0)
VERSION ?= $(shell \
	y=$$(date -u +%Y); \
	m=$$(date -u +%m); m=$$((10\#$$m)); \
	d=$$(date -u +%d); d=$$((10\#$$d)); \
	echo "$$y.$$((m * 100 + d)).$(REVID)")

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
