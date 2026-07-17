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
