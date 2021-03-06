GO ?= go

SOURCEDIR=.
SOURCES := $(shell find $(SOURCEDIR) -name '*.go')

BINARY=symedia

VERSION := $(shell cat version | head -1)
BUILD := $(shell git rev-parse HEAD)

LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.Build=$(BUILD)"

.DEFAULT_GOAL: ${BINARY}

${BINARY}: ${SOURCES} version error-template.go
	$(GO) build $(LDFLAGS) -o ${BINARY} ${SOURCEDIR}

${BINARY}_darwin_386: ${SOURCES} version error-template.go
	env GOOS=darwin GOARCH=386 $(GO) build $(LDFLAGS) -o ${BINARY}_darwin_386 ${SOURCEDIR}

${BINARY}_linux_386: ${SOURCES} version error-template.go
	env GOOS=linux GOARCH=386 $(GO) build $(LDFLAGS) -o ${BINARY}_linux_386 ${SOURCEDIR}

install:
	$(GO) install $(LDFLAGS) ${SOURCEDIR}

clean:
	test -f ${BINARY} && rm ${BINARY}

error-template.go: error-template.html
	$(GO) generate

32bit: ${BINARY}_darwin_386 ${BINARY}_linux_386

all: ${BINARY} ${BINARY}_darwin_386 ${BINARY}_linux_386

.PHONY: clean install 32bit
