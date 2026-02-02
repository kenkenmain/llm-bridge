.PHONY: build test lint coverage gazelle image docker clean run stop

BAZEL=bazel

build:
	$(BAZEL) build //cmd/llm-bridge

test:
	$(BAZEL) test //...

lint:
	$(BAZEL) test //:lint

coverage:
	$(BAZEL) coverage //internal/...
	./scripts/check-coverage.sh

gazelle:
	$(BAZEL) run //:gazelle

image:
	$(BAZEL) build //:image
	$(BAZEL) run //:image_load

docker: build
	mkdir -p .build
	cp -L bazel-bin/cmd/llm-bridge/llm-bridge_/llm-bridge .build/llm-bridge
	docker build -f Dockerfile.base -t llm-bridge-base:latest .
	docker build -t llm-bridge:latest .
	rm -rf .build

clean:
	$(BAZEL) clean
	rm -rf .build

run:
	docker-compose up -d

stop:
	docker-compose down
