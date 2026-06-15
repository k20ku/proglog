TOOLS_DIR := $(CURDIR)/tools
BIN_DIR := $(TOOLS_DIR)/bin
TMP_DIR := $(CURDIR)/tmp
PROTOC_DIR := $(TOOLS_DIR)/protoc

PROTOC := $(PROTOC_DIR)/bin/protoc
PROTOC_VERSION = v35.1
PROTOC_GEN_GO := $(BIN_DIR)/protoc-gen-go
PROTOC_GEN_GO_VERSION = v1.36.11
PROTOC_GEN_GO_GRPC := $(BIN_DIR)/protoc-gen-go-grpc
PROTOC_GEN_GO_GRPC_VERSION = v1.6.2

export PATH := $(PROTOC_DIR)/bin:$(TOOLS_DIR)/bin:$(PATH)
export GOBIN := $(BIN_DIR)

#========= Commands =========#
.DEFAULT_GOAL := help

.PHONY: tools
tools: $(PROTOC) $(PROTOC_GEN_GO) $(PROTOC_GEN_GO_GRPC) ## Initialize tools (e.g. protoc)

.PHONY: compile
compile: $(PROTOC) $(PROTOC_GEN_GO) $(PROTOC_GEN_GO_GRPC) ## Compile proto files

	protoc api/v1/*.proto \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-grpc_out=. \
		--go-grpc_opt=paths=source_relative

.PHONY: test
test: ## Test all

	go test --race ./...

.PHONY: clean
clean: ## Clean tools and tmp

	@rm -rf tools/* tmp/*

.PHONY: help ## Show options
help:

	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

#========= RECIPIES =========#

$(PROTOC):
	@mkdir -p $(PROTOC_DIR)
	PROTOC_URL=$$(\
		curl -s https://api.github.com/repos/protocolbuffers/protobuf/releases/tags/$(PROTOC_VERSION) \
		| jq -r '.assets[] | select(.name | endswith("linux-x86_64.zip")) | .browser_download_url'\
	) \
	&& PROTOC_ZIP=$(TMP_DIR)/protoc.zip \
	&& curl -o $${PROTOC_ZIP} -L $${PROTOC_URL} \
	&& unzip -oq $${PROTOC_ZIP} -d $(PROTOC_DIR) \
	&& rm -f $${PROTOC_ZIP}

$(PROTOC_GEN_GO):
	@mkdir -p $(BIN_DIR)
	go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)

$(PROTOC_GEN_GO_GRPC):
	@mkdir -p $(BIN_DIR)
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION)