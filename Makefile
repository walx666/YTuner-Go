# YTuner (Go port) - GNU Makefile
#
#   make            build for the host        -> bin/ytuner[.exe]
#   make test       go vet + unit tests
#   make cross      build all release targets -> bin/<os>-<arch>/ytuner[.exe]
#   make dist       cross + copy sample config (../../cfg) next to each binary
#   make sql        re-sync embedded SQL scripts from ../../src/sql
#   make fmt        gofmt -w
#   make clean
#
# Pure Go (CGO_ENABLED=0, SQLite via modernc.org/sqlite): cross compiling
# needs no C toolchain.

GO        ?= go
BIN       := bin
PKG       := ./cmd/ytuner
LDFLAGS   := -s -w
GOFLAGS_B := -trimpath -ldflags "$(LDFLAGS)"

export CGO_ENABLED := 0

# MSYS2 / Git-Bash: make sure GOROOT/GOPATH/GOCACHE are usable
ifneq ($(findstring MINGW,$(shell uname -s 2>/dev/null))$(findstring MSYS,$(shell uname -s 2>/dev/null)),)
  HOST_EXE := .exe
  ifeq ($(GOROOT),)
    ifneq ($(wildcard /mingw64/lib/go),)
      export GOROOT := /mingw64/lib/go
    endif
  endif
  ifeq ($(GOPATH),)
    export GOPATH := $(shell cygpath -m "$${USERPROFILE:-$$HOME}")/go
  endif
  ifeq ($(GOCACHE),)
    export GOCACHE := $(shell cygpath -m "$${LOCALAPPDATA:-$$HOME/.cache}")/go-build
  endif
endif
ifeq ($(OS),Windows_NT)
  HOST_EXE := .exe
endif

TARGETS := linux/amd64 linux/arm64 linux/arm linux/386 windows/amd64 windows/386 windows/arm64 darwin/amd64 darwin/arm64 freebsd/amd64

.PHONY: all build test vet fmt cross dist sql clean

all: build

build:
	$(GO) build $(GOFLAGS_B) -o $(BIN)/ytuner$(HOST_EXE) $(PKG)

vet:
	$(GO) vet ./...

test: vet
	$(GO) test -count=1 ./...

fmt:
	gofmt -w .

cross:
	@for t in $(TARGETS); do \
	  os=$${t%/*}; arch=$${t#*/}; ext=""; [ "$$os" = windows ] && ext=".exe"; \
	  echo "building $$os/$$arch"; \
	  GOOS=$$os GOARCH=$$arch GOARM=7 $(GO) build $(GOFLAGS_B) -o $(BIN)/$$os-$$arch/ytuner$$ext $(PKG) || exit 1; \
	done

dist: cross
	@for t in $(TARGETS); do \
	  d=$(BIN)/$${t%/*}-$${t#*/}; mkdir -p $$d/config; \
	  cp ../../cfg/ytuner.ini $$d/; cp ../../cfg/avr.ini ../../cfg/stations.ini ../../cfg/stations.yaml $$d/config/; \
	done
	@echo "dist ready in $(BIN)/<os>-<arch>/"

sql:
	cp ../../src/sql/*.sql internal/ytuner/sql/

clean:
	rm -rf $(BIN)
