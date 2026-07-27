.DEFAULT_GOAL := help
GOBIN := $(PWD)/tools/bin

.PHONY: gen
gen: buf ## Generate proto files
	buf generate

buf:
	@mkdir -p $(GOBIN)
	@GOBIN=$(GOBIN) go install github.com/bufbuild/buf/cmd/buf@v1.72.0

.PHONY: tests
tests: ## Test all
	go test --race ./...

.PNONY: tools
tools: buf

.PHONY: clean
clean: ## Clean 
	@rm -rf gen/* tools/*

.PHONY: help ## Show options
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'