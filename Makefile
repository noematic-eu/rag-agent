F4KVS_ROOT ?= $(HOME)/dev/rust/f4kvs-v2
F4KVS_TARGET ?= $(F4KVS_ROOT)/target/ffi-release
TANTIVY_SRC ?= $(CURDIR)/.deps/tantivy-go
TANTIVY_MOD_DIR := $(shell go list -m -f '{{.Dir}}' github.com/anyproto/tantivy-go@v1.0.6 2>/dev/null)
LIB_DIR := lib
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)
TANTIVY_INSTALL_TARGET := install-$(GOOS)-$(GOARCH)
BUILD_TAGS := tantivy
CGO_LDFLAGS := -L$(CURDIR)/$(LIB_DIR)
export CGO_LDFLAGS

.PHONY: f4kvs tantivy build test agent client eval-public eval-domain compare-lexical

f4kvs:
	cargo build -p f4kvs-ffi --release --manifest-path $(F4KVS_ROOT)/Cargo.toml --target-dir $(F4KVS_TARGET)
	mkdir -p $(LIB_DIR)
	cp $(F4KVS_TARGET)/release/libf4kvs_ffi.a $(LIB_DIR)/
	cp $(F4KVS_TARGET)/release/libf4kvs_ffi.dylib $(LIB_DIR)/ 2>/dev/null || \
		cp $(F4KVS_TARGET)/release/libf4kvs_ffi.so $(LIB_DIR)/ 2>/dev/null || true

$(TANTIVY_SRC)/rust/Cargo.toml:
	@if [ -z "$(TANTIVY_MOD_DIR)" ]; then echo "run: go mod download github.com/anyproto/tantivy-go@v1.0.6"; exit 1; fi
	mkdir -p .deps
	rm -rf "$(TANTIVY_SRC)"
	cp -R "$(TANTIVY_MOD_DIR)" "$(TANTIVY_SRC)"
	chmod -R u+w "$(TANTIVY_SRC)"

tantivy: $(TANTIVY_SRC)/rust/Cargo.toml
	CARGO_TARGET_DIR=$(TANTIVY_SRC)/rust/target $(MAKE) -C $(TANTIVY_SRC)/rust $(TANTIVY_INSTALL_TARGET)
	mkdir -p $(LIB_DIR)
	cp $(TANTIVY_SRC)/libs/$(GOOS)-$(GOARCH)/libtantivy_go.a $(LIB_DIR)/

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
