# Symedia

A tool to gather images and videos from a random source and output a directory
structure with hard-linked files.

## Build

```
# ensure you've got $GOPATH env variable
make clean
make
```

## Install

```
# ensure you've got $GOPATH env variable
make install
```

## Run

```
# if install, ensure $GOPATH/bin is in your path:
symedia <path>
```

```
# if build:
./symedia <path>
```

It will create a directory, called `output` and create the structure there.

