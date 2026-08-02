.DEFAULT_GOAL := help
GOBIN := $(PWD)/tools/bin
CERT = $(HOME)/.proglog/cert

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

.PHONY: gencert
gencert: ## Generate certificate
	@mkdir -p $(CERT)
	step certificate create "Root CA" $(CERT)/root-ca.crt $(CERT)/root-ca.key \
		--profile root-ca \
		--kty=OKP --curve=Ed25519 \
		--no-password --insecure \
		--force \

	step certificate create \
		"Intermediate CA 1" \
		$(CERT)/intermediate-ca.crt \
		$(CERT)/intermediate-ca.key \
		--profile intermediate-ca \
		--ca $(CERT)/root-ca.crt \
		--ca-key $(CERT)/root-ca.key \
		--no-password --insecure \
		--force \

	step certificate create \
		--profile leaf \
		"server" \
		$(CERT)/server.crt \
		$(CERT)/server.key \
		--not-after=8760h \
		--ca $(CERT)/intermediate-ca.crt \
		--ca-key $(CERT)/intermediate-ca.key \
		--no-password --insecure \
		--bundle -f \

.PHONY: help ## Show options
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'