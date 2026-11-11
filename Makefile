.PHONY: build test check
build:
	go build -o sapu .
test:
	go test ./...
check:
	test -z "$$(gofmt -l .)" && go vet ./... && go test ./...
