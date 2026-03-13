.PHONY: build test lint vet clean

build:
	go build -o teploy ./cmd/teploy

test:
	go test ./... -v

lint:
	golangci-lint run ./...

vet:
	go vet ./...

clean:
	rm -f teploy
