# Makefile for goany (Go to Any Language Transpiler)

# Variables
BINARY_NAME = goany
GO_FILES = $(wildcard *.go)
ASTYLE_DIR = ../compiler/astyle
ASTYLE_LIB = $(ASTYLE_DIR)/libastyle.a

# Go build flags
GO_BUILD_FLAGS = -v

# Default target
.PHONY: all
all: build

# Generate code (runs go generate for emitter.go)
.PHONY: generate
generate:
	@echo "Generating code..."
	cd .. && go generate ./...

# Build astyle static library
.PHONY: astyle
astyle:
	@echo "Building astyle static library..."
	$(MAKE) -C $(ASTYLE_DIR) -f Makefile.cgo

# Clean astyle
.PHONY: clean-astyle
clean-astyle:
	@echo "Cleaning astyle..."
	$(MAKE) -C $(ASTYLE_DIR) -f Makefile.cgo clean

# Build the main binary
.PHONY: build
build: generate astyle
	@echo "Building goany..."
	go build $(GO_BUILD_FLAGS) -o $(BINARY_NAME) .
	@echo "Build completed successfully!"

# Clean all build artifacts
.PHONY: clean
clean: clean-astyle
	@echo "Cleaning build artifacts..."
	rm -f $(BINARY_NAME)
	go clean

# Run tests
.PHONY: test
test:
	go test -v ./...

# Run go fmt on all Go files
.PHONY: fmt
fmt:
	go fmt ./...

# Run go vet
.PHONY: vet
vet:
	go vet ./...

# Run go mod tidy
.PHONY: tidy
tidy:
	cd .. && go mod tidy

# Install dependencies
.PHONY: deps
deps:
	cd .. && go mod download

# Development build (no optimizations, with debug info)
.PHONY: dev
dev: generate astyle
	@echo "Building goany (development)..."
	go build $(GO_BUILD_FLAGS) -gcflags="all=-N -l" -o $(BINARY_NAME) .

# Production build (optimizations enabled)
.PHONY: prod
prod: generate astyle
	@echo "Building goany (production)..."
	go build $(GO_BUILD_FLAGS) -ldflags="-w -s" -o $(BINARY_NAME) .

# Rebuild everything from scratch
.PHONY: rebuild
rebuild: clean build

# Run the binary with arguments
.PHONY: run
run: build
	./$(BINARY_NAME) $(ARGS)

# Show help
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  all        - Build the project (default)"
	@echo "  build      - Generate code, build astyle, and build goany binary"
	@echo "  generate   - Run go generate to generate code"
	@echo "  astyle     - Build astyle static library"
	@echo "  clean      - Clean all build artifacts"
	@echo "  clean-astyle - Clean only astyle build artifacts"
	@echo "  test       - Run tests"
	@echo "  fmt        - Run go fmt"
	@echo "  vet        - Run go vet"
	@echo "  tidy       - Run go mod tidy"
	@echo "  deps       - Download dependencies"
	@echo "  dev        - Development build (with debug info)"
	@echo "  prod       - Production build (optimized)"
	@echo "  rebuild    - Clean and rebuild everything"
	@echo "  run        - Build and run (use ARGS= to pass arguments)"
	@echo "  help       - Show this help message"
	@echo ""
	@echo "Examples:"
	@echo "  make                    # Build the project"
	@echo "  make clean build        # Clean and build"
	@echo "  make run ARGS=\"--help\"   # Build and run with arguments"
	@echo "  make dev                # Development build"
	@echo "  make prod               # Production build"

# Dependencies
build: $(GO_FILES) $(ASTYLE_LIB)
$(ASTYLE_LIB):
	$(MAKE) astyle
