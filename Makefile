HOSTNAME=registry.terraform.io
NAMESPACE=bitnixdev
NAME=skipjack
BINARY=terraform-provider-${NAME}
VERSION?=0.1.0
OS_ARCH?=$(shell go env GOOS)_$(shell go env GOARCH)

default: build

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

# Publish: push a v* tag after GPG secrets are configured (see docs/publishing.md).
.PHONY: release-tag
release-tag:
	@test -n "$(VERSION)" || (echo "VERSION=0.1.0 make release-tag"; exit 1)
	@echo "Create and push tag: v$(VERSION:v%=%)"
	@echo "  git tag v$(VERSION:v%=%) && git push origin v$(VERSION:v%=%)"
