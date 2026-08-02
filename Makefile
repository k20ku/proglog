.DEFAULT_GOAL := help
GOBIN := $(PWD)/tools/bin
CERT_DIR = $(HOME)/.proglog/cert

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
gencert: ## Generate certificate at CERT_DIR. Default CERT_DIR is ${HOME}/.proglog/cert
	@mkdir -p $(CERT_DIR)
	step certificate create "Root CA" $(CERT_DIR)/root-ca.pem $(CERT_DIR)/root-ca.key \
		--profile root-ca \
		--kty=OKP --curve=Ed25519 \
		--no-password --insecure \
		--force \

	step certificate create \
		"Intermediate CA 1" \
		$(CERT_DIR)/ca.pem \
		$(CERT_DIR)/ca.key \
		--profile intermediate-ca \
		--ca $(CERT_DIR)/root-ca.pem \
		--ca-key $(CERT_DIR)/root-ca.key \
		--no-password --insecure \
		--force \

	step certificate create \
		--profile leaf \
		"server" \
		$(CERT_DIR)/server.crt \
		$(CERT_DIR)/server.key \
		--not-after=8760h \
		--san localhost \
		--san 127.0.0.1 \
		--ca $(CERT_DIR)/ca.pem \
		--ca-key $(CERT_DIR)/ca.key \
		--no-password --insecure \
		--bundle -f \

	@echo CERT_DIR=$(CERT_DIR)

.PHONY: help ## Show options
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'