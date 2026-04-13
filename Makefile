.PHONY: api-gen build test

# Generate OpenAPI spec from Go annotations, then generate TypeScript types
api-gen:
	swag init -g cmd/api/main.go -o docs/ --parseDependency --parseInternal
	cd web && npx swagger2openapi ../docs/swagger.yaml -o ../docs/openapi3.yaml
	cd web && npx openapi-typescript ../docs/openapi3.yaml -o lib/api-types.ts
	@echo "API spec and TypeScript types generated."

# Build all Go binaries
build:
	go build ./cmd/api/
	go build ./cmd/bidder/
	go build ./cmd/autopilot/

# Run all tests (short mode)
test:
	go test ./... -short -count=1
