.DEFAULT_GOAL := help

.PHONY: gen
gen: ## Generate proto files

	buf generate

.PHONY: test
test: ## Test all

	go test --race ./...

.PHONY: clean
clean: ## Clean 

	@rm -rf gen/* tools/*

.PHONY: help ## Show options
help:

	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'