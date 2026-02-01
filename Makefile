.PHONY: build debug run clean test lint deps docker docker-run docker-stop docker-test docker-lint docker-build

BINARY=llm-bridge
VERSION?=dev
DOCKER_GO=docker run --rm -v $(PWD):/app -w /app golang:1.21

build:
	go build -o $(BINARY) ./cmd/llm-bridge

debug:
	go build -race -gcflags="all=-N -l" -o $(BINARY) ./cmd/llm-bridge

run: build
	./$(BINARY) serve

clean:
	rm -f $(BINARY)

test:
	go test -v ./...

lint:
	golangci-lint run

deps:
	go mod download
	go mod tidy

docker:
	docker build -t llm-bridge:$(VERSION) .

docker-run:
	docker-compose up -d

docker-stop:
	docker-compose down

docker-test:
	$(DOCKER_GO) go test -v -race -coverprofile=coverage.out ./...

docker-lint:
	docker run --rm -v $(PWD):/app -w /app golangci/golangci-lint:latest golangci-lint run

docker-build:
	$(DOCKER_GO) go build -o $(BINARY) ./cmd/llm-bridge
