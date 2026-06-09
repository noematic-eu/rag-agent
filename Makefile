F4KVS_ROOT ?= $(HOME)/dev/rust/f4kvs-v2
F4KVS_TARGET ?= $(F4KVS_ROOT)/target/ffi-release
TANTIVY_SRC ?= $(CURDIR)/.deps/tantivy-go
TANTIVY_MODULE := github.com/anyproto/tantivy-go@v1.0.6
TANTIVY_VERSION := v1.0.6
LIB_DIR := lib
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)
ifeq ($(GOOS),linux)
  ifeq ($(GOARCH),amd64)
    TANTIVY_PLATFORM := linux-amd64-musl
  else ifeq ($(GOARCH),arm64)
    TANTIVY_PLATFORM := linux-arm64-musl
  else
    $(error unsupported linux GOARCH for tantivy: $(GOARCH))
  endif
else
  TANTIVY_PLATFORM := $(GOOS)-$(GOARCH)
endif
TANTIVY_INSTALL_TARGET := install-$(TANTIVY_PLATFORM)
TANTIVY_LIB_DIR := $(TANTIVY_SRC)/libs/$(TANTIVY_PLATFORM)
TANTIVY_LIB := $(TANTIVY_LIB_DIR)/libtantivy_go.a
BUILD_TAGS := tantivy
CGO_LDFLAGS := -L$(CURDIR)/$(LIB_DIR)
export CGO_LDFLAGS

.PHONY: f4kvs tantivy build test agent client fmt fmt-check install-hooks vet lint test-lite check eval-public eval-domain compare-lexical

fmt:
	gofmt -w ./agent ./client ./lexical ./model ./internal

fmt-check:
	@test -z "$$(gofmt -l ./agent ./client ./lexical ./model ./internal)"

install-hooks:
	git config core.hooksPath .githooks

vet:
	go vet ./client/... ./lexical/... ./agent/p9fs/... ./model/...

lint:
	golangci-lint run ./...

test-lite:
	go test ./client/... ./lexical/... ./agent/p9fs/... ./model/...

check: fmt-check vet lint test-lite

f4kvs:
	cargo build -p f4kvs-ffi --release --manifest-path $(F4KVS_ROOT)/Cargo.toml --target-dir $(F4KVS_TARGET)
	mkdir -p $(LIB_DIR)
	cp $(F4KVS_TARGET)/release/libf4kvs_ffi.a $(LIB_DIR)/
	cp $(F4KVS_TARGET)/release/libf4kvs_ffi.dylib $(LIB_DIR)/ 2>/dev/null || \
		cp $(F4KVS_TARGET)/release/libf4kvs_ffi.so $(LIB_DIR)/ 2>/dev/null || true

$(TANTIVY_SRC)/rust/Cargo.toml:
	go mod download $(TANTIVY_MODULE)
	@TANTIVY_MOD_DIR=$$(go list -m -f '{{.Dir}}' $(TANTIVY_MODULE)); \
	if [ -z "$$TANTIVY_MOD_DIR" ]; then \
		echo "failed to resolve $(TANTIVY_MODULE)"; exit 1; \
	fi; \
	mkdir -p .deps; \
	rm -rf "$(TANTIVY_SRC)"; \
	cp -R "$$TANTIVY_MOD_DIR" "$(TANTIVY_SRC)"; \
	chmod -R u+w "$(TANTIVY_SRC)"

ifeq ($(GOOS),linux)
tantivy:
	mkdir -p $(TANTIVY_LIB_DIR) $(LIB_DIR)
	@if [ ! -f "$(TANTIVY_LIB)" ]; then \
		curl -fsSL -o /tmp/tantivy-$(TANTIVY_PLATFORM).tar.gz \
			https://github.com/anyproto/tantivy-go/releases/download/$(TANTIVY_VERSION)/$(TANTIVY_PLATFORM).tar.gz; \
		tar -xzf /tmp/tantivy-$(TANTIVY_PLATFORM).tar.gz -C $(TANTIVY_LIB_DIR); \
		rm -f /tmp/tantivy-$(TANTIVY_PLATFORM).tar.gz; \
	fi
	cp $(TANTIVY_LIB) $(LIB_DIR)/
else
tantivy: $(TANTIVY_SRC)/rust/Cargo.toml
	CARGO_TARGET_DIR=$(TANTIVY_SRC)/rust/target $(MAKE) -C $(TANTIVY_SRC)/rust $(TANTIVY_INSTALL_TARGET)
	mkdir -p $(LIB_DIR)
	cp $(TANTIVY_LIB) $(LIB_DIR)/
endif

build: f4kvs
	CGO_ENABLED=1 go build -tags $(BUILD_TAGS) ./...

test: f4kvs
	CGO_ENABLED=1 go test -tags $(BUILD_TAGS) ./...

agent: f4kvs tantivy
	CGO_ENABLED=1 go build -tags $(BUILD_TAGS) -o bin/agent ./agent

client:
	go build -o bin/client ./client

EVAL_SERVER ?= http://127.0.0.1:8080

eval-public:
	./scripts/eval_setup_public.sh $(EVAL_SERVER)
	./scripts/eval.sh $(EVAL_SERVER) eval/gold/public.jsonl

eval-domain:
	./scripts/eval_setup_domain.sh $(EVAL_SERVER)
	EVAL_MIN_RECALL=0 ./scripts/eval.sh $(EVAL_SERVER) eval/gold/domain.jsonl

compare-lexical:
	./scripts/compare_lexical_engines.sh
