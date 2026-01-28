HOSTNAME=registry.terraform.io
NAMESPACE=zone-eu
NAME=zone
BINARY=terraform-provider-${NAME}
VERSION=1.0.0
OS_ARCH=$(shell go env GOOS)_$(shell go env GOARCH)

default: build

build:
	go build -o ${BINARY}

install: build
	mkdir -p ~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/${OS_ARCH}
	mv ${BINARY} ~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/${OS_ARCH}

test:
	go test -v ./...

testacc:
	TF_ACC=1 go test -v ./... -timeout 120m

fmt:
	go fmt ./...

lint:
	golangci-lint run ./...

docs:
	go generate ./...

clean:
	rm -f ${BINARY}

.PHONY: build install test testacc fmt lint docs clean
