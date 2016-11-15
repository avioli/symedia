GO ?= go

SOURCEDIR=.
SOURCES := $(shell find $(SOURCEDIR) -name '*.go')

BINARY=symedia

VERSION := $(shell cat version | head -1)
BUILD := $(shell git rev-parse HEAD)

LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.Build=$(BUILD)"

.DEFAULT_GOAL: ${BINARY}

${BINARY}: ${SOURCES} version
	$(GO) build $(LDFLAGS) -o ${BINARY} ${SOURCEDIR}

${BINARY}_darwin_386: ${SOURCES} version
	env GOOS=darwin GOARCH=386 $(GO) build $(LDFLAGS) -o ${BINARY}_darwin_386 ${SOURCEDIR}

install:
	$(GO) install $(LDFLAGS) ${SOURCEDIR}

clean:
	test -f ${BINARY} && rm ${BINARY}

32bit: ${BINARY}_darwin_386

.PHONY: clean install 32bit
