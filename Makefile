.PHONY: help test lint tidy
.DEFAULT_GOAL := help

help:
	@echo "Usage: make <target> [ROOT=<dir>]"
	@echo ""
	@echo "Targets:"
	@echo "  test         Run all tests"
	@echo "  bench        Run benchmarks"
	@echo "  lint         Run golangci-lint"
	@echo "  deps         Update dependencies"
	@echo "  tidy         Tidy go.mod"

test:
	go test ./...

bench:
	go test -bench=. ./...

lint:
	golangci-lint run

deps:
	go get -u ./...

tidy:
	go mod tidy
