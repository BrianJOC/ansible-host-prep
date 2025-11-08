# Repository Guidelines

## Project Structure & Module Organization
- `go.mod` defines the Go 1.25.4 module `github.com/BrianJOC/prep-for-ansible`; place reusable packages under `internal/` or `pkg/` as they are added.
- The primary CLI entrypoint is expected under `cmd/bootstrap-tui`, matching the build/run targets; keep each subcommand in its own file for clarity.
- `utils/` hosts supporting libraries (for example `utils/sshconnection` for SSH helpers) and should stay dependency-light so it can be imported from multiple binaries.
- `bin/` is Hermit-managed tooling (Go toolchain, `golangci-lint`, `just`, Python shims); do not edit files there manually.

## Build, Test, and Development Commands
- `just fmt` – runs `gofmt -w` on every Go source; execute before committing.
- `just lint` – runs `golangci-lint run ./...`; fails on style or vet issues.
- `just test` – executes `go test ./...` across all packages.
- `just build` / `just run` – compile or run the `cmd/bootstrap-tui` binary.
- `just ci` – convenience target for `fmt`, `lint`, `test`, `build`; mirrors the expected CI pipeline.

## Coding Style & Naming Conventions
- Rely on `gofmt` defaults (tabs for indentation, sorted imports); avoid manual alignment.
- Use lower_snake_case for filenames, mixedCaps for exported Go identifiers, and keep packages lowercase, short, and unique.
- Keep functions under 40 lines where possible; extract helpers into `internal/<module>` when the API should remain private.
- Prefer functional options for optional behavior (e.g., `sshconnection.WithTimeout`) instead of proliferating parameters so APIs remain stable.

## Testing Guidelines
- Prefer table-driven tests in `_test.go` files beside the code under test; name tests `Test<Component><Scenario>`.
- Use `t.Helper()` in reusable assertions and `t.Parallel()` when tests do not mutate shared state.
- Write assertions with `github.com/stretchr/testify/require` for clarity and immediate failures; keep coverage for both success and error paths.
- Aim to cover edge cases around SSH handling and CLI argument parsing before adding new features.

## Commit & Pull Request Guidelines
- Follow the imperative, scope-light style visible in history (e.g., `readme`); keep the first line ≤72 chars and explain “what/why” in the body if needed.
- Reference related issues in the PR description, note behavioral changes, and attach screenshots for any UI/UX tweaks in the TUI.
- Confirm `just ci` passes locally before opening a PR; CI failures for lint or formatting will block merges.

## Security & Configuration Tips
- Activate the Hermit environment (`source bin/activate-hermit` or `bin\\activate-hermit` on Windows) so the pinned toolchain is used consistently.
- Never commit secrets or SSH material; store sample configs under `utils/sshconnection/testdata` with redacted keys when fixtures are required.
- Validate any shell commands executed by the TUI against least-privilege requirements before shipping new automation.
