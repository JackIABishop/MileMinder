.PHONY: build build-web build-go dev dev-api dev-web clean install

# Build everything
build: build-web build-go

# Build the web UI
build-web:
	cd web && npm install && npm run build
	rm -rf internal/web/dist/*
	cp -r web/dist/* internal/web/dist/

# Build the Go binary
build-go:
	go build -o mileminder .

# Development mode - run both API and web dev server
dev:
	@echo "Starting development servers..."
	@echo "Run 'make dev-api' in one terminal"
	@echo "Run 'make dev-web' in another terminal"

# Run API server only (for development)
dev-api:
	go run . serve --dev --port 8080

# Run web dev server with hot reload
dev-web:
	cd web && npm run dev

# Clean build artifacts
clean:
	rm -f mileminder
	rm -rf web/dist
	rm -rf web/node_modules
	rm -rf web/.svelte-kit
	rm -rf internal/web/dist/*
	touch internal/web/dist/.gitkeep

# Install dependencies
install:
	go mod download
	cd web && npm install

# Run tests
test:
	go test ./...
