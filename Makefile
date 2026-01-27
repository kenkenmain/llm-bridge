.PHONY: build run clean test lint docker

BINARY=llm-bridge
VERSION?=dev

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY) ./cmd/llm-bridge

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
