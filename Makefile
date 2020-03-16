.PHONY: build
build:
		go build -v ./cmd/ariserver

.DEFAULT_GOAL := build
