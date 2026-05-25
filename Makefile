CC ?= gcc
GO ?= go

BUILD_DIR := build
COMPILER := $(BUILD_DIR)/pyritec
SRC ?= examples/base.pyr
OUT ?= $(BUILD_DIR)/base
PYRITEC_FLAGS ?=
COMPILER_SOURCES := go.mod $(wildcard cmd/pyritec/*.go)

.PHONY: all clean compile run sample

all: $(COMPILER)

$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

$(COMPILER): $(COMPILER_SOURCES) | $(BUILD_DIR)
	$(GO) build -o $@ ./cmd/pyritec

compile sample: $(COMPILER) $(SRC) | $(BUILD_DIR)
	$(COMPILER) $(PYRITEC_FLAGS) $(SRC) -o $(OUT)

run: compile
	$(OUT); echo "exit=$$?"

clean:
	rm -rf $(BUILD_DIR) score.txt
