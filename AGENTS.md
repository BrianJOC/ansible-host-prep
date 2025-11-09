# Repository Guidelines

## Project Structure & Module Organization
- `go.mod` defines the Go 1.25.4 module `github.com/BrianJOC/ansible-host-prep`; place reusable packages under `internal/` or `pkg/` as they are added.
- The primary CLI entrypoint is expected under `cmd/bootstrap-tui`, matching the build/run targets; keep each subcommand in its own file for clarity.
- `phases/` owns the bootstrap pipeline (e.g., `sshconnect`, `sudoensure`, `pythonensure`, `ansibleuser`) plus the shared `Manager`, input definitions, and observers; new phases should expose metadata (ID, inputs, description) and communicate via the shared `phases.Context`.
- `utils/` hosts supporting libraries (`sshconnection`, `privilege`, `sshkeypair`, `systemuser`, `pkginstaller`); keep these dependency-light so they can be imported from multiple phases.
- `pkg/phasedapp/` hosts the Bubble Tea-driven phase runner; keep this UI layer thin by delegating logic to phases/util packages, and have entrypoints (e.g., `cmd/bootstrap-tui`) wire it up.
- `bin/` is Hermit-managed tooling (Go toolchain, `golangci-lint`, `just`, Python shims); do not edit files there manually.

## Build, Test, and Development Commands
- `just fmt` – runs `gofmt -w` on every Go source; execute before committing.
- `just lint` – runs `golangci-lint run ./...`; fails on style or vet issues.
- `just test` – executes `go test ./...` across all packages.
- `just build` / `just run` – compile or run the `cmd/bootstrap-tui` binary.
- `just ci` – convenience target for `fmt`, `lint`, `test`, `build`; mirrors the expected CI pipeline.
- `just tui` – launches the Bubble Tea interface to run the full bootstrap interactively.

## Coding Style & Naming Conventions
- Rely on `gofmt` defaults (tabs for indentation, sorted imports); avoid manual alignment.
- Use lower_snake_case for filenames, mixedCaps for exported Go identifiers, and keep packages lowercase, short, and unique.
- Keep functions under 40 lines where possible; extract helpers into `internal/<module>` when the API should remain private.
- Prefer functional options for optional behavior (e.g., `sshconnection.WithTimeout`, `pkginstaller.WithCustomCheck`) instead of proliferating parameters so APIs remain stable.
- When adding phases, populate `PhaseMetadata.Inputs` with clear `InputDefinition` records (ID, label, kind, secret/select flags) and store values via `phases.SetInput` so the manager/TUI can fulfill prompts consistently.
- In the TUI, show default values as placeholder/background text for free-form inputs so the user can review or override them; only inject the default when the input is submitted empty. Select prompts may pre-select their default but should not overwrite prior user choices.

## Phase Workflow & Orchestration
- Register phases in execution order (`sshconnect` → `sudoensure` → `pythonensure` → `ansibleuser`) using `phases.Manager`; use `WithInputHandler` and observers so TUIs can react to lifecycle events.
- Surface missing or invalid operator input with `phases.InputRequestError`; the manager will pause execution, call the configured handler, and retry the phase.
- Share data between phases through `phases.Context` keys (e.g., `sshconnect.ContextKeySSHClient`, `sudoensure.ContextKeyElevatedClient`, `pythonensure.ContextKeyInstalled`); document any new keys when you add phases so downstream code knows how to consume them.
- Wrap privileged operations with the `utils/privilege` elevated client before calling runners such as `pkginstaller` or `systemuser`.

## Testing Guidelines
- Prefer table-driven tests in `_test.go` files beside the code under test; name tests `Test<Component><Scenario>`.
- Use `t.Helper()` in reusable assertions and `t.Parallel()` when tests do not mutate shared state.
- Write assertions with `github.com/stretchr/testify/require` for clarity and immediate failures; keep coverage for both success and error paths.
- For phases, add targeted tests that simulate manager interactions (e.g., fake connectors/ensurers, `InputRequestError` round-trips, context mutations) rather than relying on real SSH hosts.
- Aim to cover edge cases around SSH handling, privilege escalation, package installs, and CLI argument parsing before adding new features.

## Commit & Pull Request Guidelines
- Follow the imperative, scope-light style visible in history (e.g., `readme`); keep the first line ≤72 chars and explain “what/why” in the body if needed.
- Reference related issues in the PR description, note behavioral changes, and attach screenshots for any UI/UX tweaks in the TUI.
- Confirm `just ci` passes locally before opening a PR; CI failures for lint or formatting will block merges.

## Security & Configuration Tips
- Activate the Hermit environment (`source bin/activate-hermit` or `bin\\activate-hermit` on Windows) so the pinned toolchain is used consistently.
- Never commit secrets or SSH material; store sample configs under `utils/sshconnection/testdata` with redacted keys when fixtures are required, and generate ansible user keys with `sshkeypair` in temp directories during tests.
- Validate any shell commands executed by the TUI against least-privilege requirements before shipping new automation.
- Do not use `cat` (or similar shell heredocs) to edit files; rely on proper editors or tooling (`apply_patch`, `$EDITOR`, etc.) so accidental truncation is avoided.
