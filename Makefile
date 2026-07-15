BINARY  := lazydbx
MODULE  := github.com/jongracecox/lazydbx
VERSION ?= dev

.PHONY: build run test cover lint fmt tidy tools clean

build: ## Build the binary into ./bin
	go build -o bin/$(BINARY) ./cmd/$(BINARY)

run: ## Run the TUI
	go run ./cmd/$(BINARY)

test: ## Run all tests with race detector
	go test -race ./...

cover: ## Run tests with coverage and open the HTML report
	go test -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -func=coverage.out | tail -1
	go tool cover -html=coverage.out

lint: ## Run golangci-lint
	golangci-lint run

fmt: ## Format code (gofumpt + goimports via golangci-lint)
	golangci-lint fmt

tidy: ## Verify go.mod/go.sum are tidy
	go mod tidy -diff

tools: ## Install dev tools
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	go install github.com/evilmartians/lefthook@latest
	lefthook install

clean:
	rm -rf bin dist coverage.out coverage.xml
