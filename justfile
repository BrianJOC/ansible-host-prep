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
    rg --files -g '*.go' | xargs gofmt -w

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
    go run ./tui

ci: fmt lint test build
