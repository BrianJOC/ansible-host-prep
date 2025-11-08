# Phases Package Guide

## Purpose
The `phases` package coordinates all bootstrap stages for prepping a target host. Each phase implements `Phase` (metadata + `Run`), shares data via `phases.Context`, and is orchestrated by `phases.Manager` which handles ordering, observer notifications, and interactive input requests for the TUI.

## Structure
- `phases.go` defines the core interfaces (`Phase`, `Observer`, `PhaseMetadata`, `InputDefinition`).
- `context.go` provides a concurrency-safe key/value store (`Context`) that phases use to exchange artifacts such as SSH clients or user results.
- `manager.go` registers and executes phases sequentially, looping when a phase returns `InputRequestError` and delegating to the configured `InputHandler`.
- `handler.go` and `input.go` offer helpers for input resolution and context key composition.
- Subdirectories (`sshconnect`, `sudoensure`, `pythonensure`, `ansibleuser`) contain concrete phases; new phases should live in their own folder with a small interface and targeted tests.

## Phase Authoring Checklist
1. Create a new package under `phases/<name>` with a struct exposing `Metadata()` and `Run(ctx, phaseCtx)`.
2. Populate `PhaseMetadata`:
   - `ID`: kebab or snake case (`my_phase`); must be unique.
   - `Title`/`Description`: what the phase does.
   - `Inputs`: slice of `InputDefinition` (ID, label, `InputKindText`/`InputKindSecret`/`InputKindSelect`, `Required`, `Secret`, etc.).
3. Use `phases.GetInput` / `phases.SetInput` (or helper wrappers) to read operator input and persist values for later phases.
4. Return `phases.InputRequestError` if more input is required so the TUI can prompt the operator. Provide a clear reason string.
5. Place any intermediate artifacts in the shared context via descriptive keys (e.g., `myphase.ContextKeyWidget`). Document new keys in `AGENTS.md`.
6. Write focused unit tests that stub external dependencies (e.g., fake connectors, fake runners) to cover success, validation failures, and input-request scenarios.

## Manager & Input Handling
- Register phases in order using `phases.NewManager(WithObserver(...), WithInputHandler(...))`.
- The manager automatically re-runs a phase after the input handler supplies requested data; ensure phases are idempotent.
- Observers (`ObserverFunc` in tests or Bubble Tea’s wrapper) receive `PhaseStarted` and `PhaseCompleted` events; use them for logging or UI feedback.

## Common Context Keys
- `sshconnect.ContextKeySSHClient`, `ContextKeySSHPassword`, `ContextKeyAuthMethod` for raw SSH information.
- `sudoensure.ContextKeyElevatedClient` for the privileged SSH client (wrapped in `privilege.ElevatedClient`).
- `pythonensure.ContextKeyInstalled` indicates Python installation status.
- `ansibleuser.ContextKeyUserResult` and `ContextKeyKeyInfo` track the created user and keypair metadata.

When adding new phases, define context key constants in the phase package and reference them via imports rather than duplicating string literals.
