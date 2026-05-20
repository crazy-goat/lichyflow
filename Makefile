.PHONY: build test clean install lint examples

BINARY=lichyflow
GO=go

build:
	$(GO) build -o $(BINARY) ./cmd/lichyflow/

test:
	$(GO) test ./... -v

clean:
	rm -f $(BINARY)
	rm -rf .lichyflow/

install: build
	cp $(BINARY) /usr/local/bin/

lint:
	$(GO) vet ./...

golangci-lint:
	golangci-lint run ./... --timeout=5m

examples: build
	@echo "=== Example 1: simple-ci ==="
	@export PATH="$(CURDIR):$$PATH" && rm -rf /tmp/lichyflow-example1 && ./$(BINARY) -f examples/01-simple-ci.yaml -s /tmp/lichyflow-example1 ; echo "Exit code: $$?"

	@echo ""
	@echo "=== Example 2: retry-counter ==="
	@export PATH="$(CURDIR):$$PATH" && rm -rf /tmp/lichyflow-example2 && ./$(BINARY) -f examples/02-retry-counter.yaml -s /tmp/lichyflow-example2 ; echo "Exit code: $$?"

visualize: build
	@echo "=== Example 1: simple-ci ==="
	@./$(BINARY) -v -f examples/01-simple-ci.yaml

	@echo ""
	@echo "=== Example 2: retry-counter ==="
	@./$(BINARY) -v -f examples/02-retry-counter.yaml