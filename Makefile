.PHONY: build check fmt-check test vet

build:
	mkdir -p .build
	go build -o .build/ack ./cmd/ack

fmt-check:
	test -z "$$(gofmt -l .)"

test:
	go test -race ./...

vet:
	go vet ./...

check: fmt-check vet test build
