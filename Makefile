.PHONY: api-gen build clean test

BIN_DIR := bin
DOCS_GEN_DIR := docs/generated

# Generate OpenAPI spec from Go annotations, then generate TypeScript types
api-gen:
	mkdir -p $(DOCS_GEN_DIR)
	cd web && mkdir -p ../$(DOCS_GEN_DIR)
	swag init -g cmd/api/main.go -o $(DOCS_GEN_DIR) --parseDependency --parseInternal
	cd web && npx swagger2openapi ../$(DOCS_GEN_DIR)/swagger.yaml -o ../$(DOCS_GEN_DIR)/openapi3.yaml
	cd web && npx openapi-typescript ../$(DOCS_GEN_DIR)/openapi3.yaml -o lib/api-types.ts
	@echo "API spec and TypeScript types generated."

# Build all Go binaries
build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/api ./cmd/api/
	go build -o $(BIN_DIR)/bidder ./cmd/bidder/
	go build -o $(BIN_DIR)/consumer ./cmd/consumer/
	go build -o $(BIN_DIR)/autopilot ./cmd/autopilot/
	go build -o $(BIN_DIR)/exchange-sim ./cmd/exchange-sim/
	go build -o $(BIN_DIR)/resetbudget ./cmd/resetbudget/
	go build -o $(BIN_DIR)/simulate ./cmd/simulate/

# Remove generated binaries
clean:
	rm -f $(BIN_DIR)/api $(BIN_DIR)/api.exe $(BIN_DIR)/bidder $(BIN_DIR)/bidder.exe $(BIN_DIR)/consumer $(BIN_DIR)/consumer.exe $(BIN_DIR)/autopilot $(BIN_DIR)/autopilot.exe $(BIN_DIR)/exchange-sim $(BIN_DIR)/exchange-sim.exe $(BIN_DIR)/resetbudget $(BIN_DIR)/resetbudget.exe $(BIN_DIR)/simulate $(BIN_DIR)/simulate.exe

# Run all tests (short mode)
test:
	go test ./... -short -count=1
