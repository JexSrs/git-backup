# Git Backup — build & run

BINARY := git-backup
BIN_DIR := bin
PKG     := ./src

.DEFAULT_GOAL := build

.PHONY: build run dev fmt vet tidy clean docker docker-run help

## build: compile the binary into ./bin/git-backup
build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(BINARY) $(PKG)

## run: build, then run from the repo root (so config.jsonc is found)
run: build
	./$(BIN_DIR)/$(BINARY)

## dev: run straight from source without producing a binary
dev:
	go run $(PKG)

## fmt: format all Go sources
fmt:
	gofmt -w src/

## vet: run go vet
vet:
	go vet $(PKG)/...

## tidy: sync go.mod / go.sum
tidy:
	go mod tidy

## clean: remove build artifacts
clean:
	rm -rf $(BIN_DIR)

## docker: build the docker image via compose
docker:
	docker compose build

## docker-run: build and start the container
docker-run:
	docker compose up --build -d

## help: list available targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## //'
