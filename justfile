set shell := ["bash", "-eu", "-o", "pipefail", "-c"]

default:
    @just --list

list:
    @just --list

tidy:
    go mod tidy

init:
    . bin/activate-hermit

fmt:
    go fmt ./...

format:
    @just fmt

lint:
    golangci-lint run ./...

test:
    go test ./...

build:
    go build ./cmd/bootstrap-tui

run:
    go run ./cmd/bootstrap-tui

tui:
    go run ./cmd/bootstrap-tui

ci: fmt lint test build
