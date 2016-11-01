SOURCEDIR=.
SOURCES := $(shell find $(SOURCEDIR) -name '*.go')

BINARY=symedia

VERSION=`cat version | head -1`
BUILD=`git rev-parse HEAD`

LDFLAGS=-ldflags "-X main.Version=${VERSION} -X main.Build=${BUILD}"

.DEFAULT_GOAL: ${BINARY}

${BINARY}: ${SOURCES}
	go build ${LDFLAGS} -o ${BINARY} ${SOURCEDIR}

${BINARY}_darwin_386: ${SOURCES}
	GOOS=darwin GOARCH=386 build go build ${LDFLAGS} -o ${BINARY}_darwin_386 ${SOURCEDIR}

install:
	go install ${LDFLAGS} ${SOURCEDIR}

clean:
	test -f ${BINARY} && rm ${BINARY}

.PHONY: clean install
