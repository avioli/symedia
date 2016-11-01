GO ?= go

SOURCEDIR=.
SOURCES := $(shell find $(SOURCEDIR) -name '*.go')

BINARY=symedia

VERSION := $(shell cat version | head -1)
BUILD := $(shell git rev-parse HEAD)

LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.Build=$(BUILD)"

.DEFAULT_GOAL: ${BINARY}

${BINARY}: ${SOURCES}
	$(GO) build $(LDFLAGS) -o $(BINARY) $(SOURCEDIR)

${BINARY}_darwin_386: ${SOURCES}
	GOOS=darwin GOARCH=386 build $(GO) build $(LDFLAGS) -o $(BINARY)_darwin_386 ${SOURCEDIR}

install:
	$(GO) install $(LDFLAGS) ${SOURCEDIR}

clean:
	test -f ${BINARY} && rm ${BINARY}

.PHONY: clean install
