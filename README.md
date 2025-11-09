# ansible-host-prep

> **Status:** Beta — APIs, helpers, and bundles may change as the project evolves; expect occasional breaking changes until v1.

Provision a remote Linux host so it is ready for Ansible by running a guided, repeatable bootstrap pipeline. The tool ships with a terminal UI that walks you through connecting over SSH, gaining sudo privileges, ensuring Python is present, and finally creating a dedicated `ansible` user with passwordless sudo and an SSH key.

## Why This Exists

Homelab nodes or freshly provisioned servers often lack the prerequisites that an Ansible control node expects: Python might be missing, your admin account may not have sudo, or there may be no dedicated automation user. Running ad-hoc shell snippets to fix that is error-prone and hard to repeat. `ansible-host-prep` codifies the bootstrap as Go phases so you can:

- Re-run safely until every step succeeds.
- Follow progress in a clean Bubble Tea UI.
- Share the same workflow across your team.

## Features

- **Hermit-managed toolchain** – Go, Python, `just`, and lint tooling are pinned for reproducible builds.
- **Phase manager** – Each step (`sshconnect`, `sudoensure`, `pythonensure`, `ansibleuser`) exposes metadata, inputs, and shared context so the TUI can prompt for credentials or key paths automatically.
- **Responsive TUI workflow** – Bubble Tea interface resizes cleanly, surfaces keyboard shortcuts, and provides per-phase action menus (retry, copy errors, etc.) while remembering your last answers so restarts are painless.
- **Secure input handling** – Text defaults show up as placeholders until you press enter, secret prompts never prefill or echo actual values, and all logs/status messages are auto-redacted to avoid leaking credentials.
- **Dedicated ansible user** – Generates or reuses an SSH key pair, installs it in `authorized_keys`, and grants passwordless sudo with `/etc/sudoers.d` management.
- **Extensible architecture** – Additional phases can be registered with the manager to extend the bootstrap pipeline without touching the TUI.

## Quick Start

```bash
git clone https://github.com/BrianJOC/ansible-host-prep.git
cd ansible-host-prep
just init        # downloads/activates the Hermit toolchain
just tui         # launches the Bubble Tea workflow
```

During the run you will be prompted for:

1. SSH connection info (`host`, `port`, username, private key/password).
2. Sudo password if the SSH user is not already privileged.
3. Local path to store the ansible user's SSH private key (e.g., `~/.ssh/ansible_id`).

You can also drive individual phases or the CLI without the TUI:

```bash
just run                 # go run ./cmd/bootstrap-tui
go run ./cmd/bootstrap-tui  # identical to `just run`
just test                # go test ./...
```

## Embedding the Phased App

The Bubble Tea workflow now lives in `pkg/phasedapp`, making it easy for other binaries to consume. The API mirrors Cobra-style ergonomics: configure phases and options, then call `Start`/`Stop`.

```go
package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/BrianJOC/ansible-host-prep/phases"
	"github.com/BrianJOC/ansible-host-prep/pkg/phasedapp"
)

func main() {
	app, err := phasedapp.New(
		phasedapp.WithPhases(
			greetPhase{},
			summaryPhase{},
		),
	)
	if err != nil {
		log.Fatalf("setup failed: %v", err)
	}
	defer app.Stop()

	if err := app.Start(context.Background()); err != nil {
		log.Fatalf("bootstrap failed: %v", err)
	}
}

type greetPhase struct{}

func (g greetPhase) Metadata() phases.PhaseMetadata {
	return phases.PhaseMetadata{
		ID:          "greet",
		Title:       "Greet Host",
		Description: "Collect operator name and greet the target host.",
		Inputs: []phases.InputDefinition{
			{ID: "operator_name", Label: "Operator Name", Kind: phases.InputKindText},
		},
	}
}

func (g greetPhase) Run(ctx context.Context, phaseCtx *phases.Context) error {
	value, ok := phases.GetInput(phaseCtx, "greet", "operator_name")
	name := strings.TrimSpace(fmt.Sprint(value))
	if !ok || name == "" {
		return phases.InputRequestError{
			PhaseID: "greet",
			Input: phases.InputDefinition{
				ID:       "operator_name",
				Label:    "Operator Name",
				Kind:     phases.InputKindText,
				Required: true,
			},
		}
	}
	log.Printf("Hello %s, beginning bootstrap…", name)
	return nil
}

type summaryPhase struct{}

func (summaryPhase) Metadata() phases.PhaseMetadata {
	return phases.PhaseMetadata{
		ID:          "summary",
		Title:       "Summary",
		Description: "Display a final message once bootstrap completes.",
	}
}

func (summaryPhase) Run(ctx context.Context, phaseCtx *phases.Context) error {
	log.Println("Bootstrap pipeline complete!")
	return nil
}
```

Swap in any combination of built-in or custom phases using `phasedapp.WithPhases`, or extend behavior with `WithManagerOptions` and `WithProgramOptions`.

### Ergonomic Helpers

- **SimplePhase** – build phases inline without declaring new types:

```go
phase := phasedapp.NewPhase(
	phases.PhaseMetadata{
		ID:    "greet",
		Title: "Greet Host",
		Inputs: []phases.InputDefinition{
			phasedapp.TextInput("operator", "Operator Name", phasedapp.Required()),
		},
		Tags: []string{"demo"},
	},
	func(ctx context.Context, phaseCtx *phases.Context) error {
		value, ok := phases.GetInput(phaseCtx, "greet", "operator")
		if ok {
			phasedapp.SetContext(phaseCtx, phasedapp.Namespace("demo", "operator"), value)
		}
		name, _ := phasedapp.GetContext[string](phaseCtx, phasedapp.Namespace("demo", "operator"))
		log.Printf("Hello %s!", name)
		return nil
	},
)
```

- **Input helpers** – `TextInput`, `SecretInput`, `SelectInput`, plus options like `phasedapp.WithDescription` and `phasedapp.WithDefault`.
- **Context helpers** – `Namespace`, `SetContext`, `GetContext` provide typed storage for shared artifacts (SSH clients, elevated shells, etc.).
- **Builder & Bundles** – compose reusable bundles of phases and validate duplicates:

```go
phaseList, err := phasedapp.NewBuilder().
	AddPhases(ansibleprep.Bundle()...).
	AddPhase(customPhase).
	Build()
if err != nil { log.Fatal(err) }

app, _ := phasedapp.New(phasedapp.WithPhases(phaseList...))
```

Use `phasedapp.WithBundle(ansibleprep.Bundle)` when you just need the default Ansible prep pipeline, or `phasedapp.SelectPhases(phases, phasedapp.WithTag("ansible"))` to filter by metadata tags.

## Repository Layout

```
cmd/bootstrap-tui   # CLI entrypoint used by `just run`
pkg/phasedapp       # Reusable Bubble Tea runner library
phases/             # Phase manager plus sshconnect, sudoensure, pythonensure, ansibleuser
utils/              # Shared helpers (sshconnection, privilege, sshkeypair, systemuser, pkginstaller)
bin/                # Hermit-managed shims; never edit manually
.hermit/            # Toolchain caches (ignored except for Go binaries)
justfile            # Common developer tasks (fmt, lint, test, build, tui, init)
```

## Development Workflow

All commands run inside the Hermit environment installed by `just init`.

| Task          | Command                 |
|---------------|-------------------------|
| Format Go     | `just fmt`              |
| Lint          | `just lint`             |
| Tests         | `just test`             |
| Build binary  | `just build`            |
| Run CLI       | `just run`              |
| Run full TUI  | `just tui` (alias for `go run ./cmd/bootstrap-tui`) |
| CI bundle     | `just ci`               |

### Adding a Phase

1. Create a package under `phases/<name>`.
2. Implement `phases.Phase` with metadata (ID, title, description, inputs) and a `Run` method that reads/writes `phases.Context`.
3. Register the new phase in `cmd/bootstrap-tui/main.go` (or wherever the manager is constructed) in the desired order.
4. Add table-driven tests in `<name>/phase_test.go`, mocking any SSH/system interactions.

### Sharing Data Between Phases

Each phase uses typed context keys (e.g., `sshconnect.ContextKeySSHClient`, `sudoensure.ContextKeyElevatedClient`, `pythonensure.ContextKeyInstalled`, `ansibleuser.ContextKeyUserResult`). When you add new data to the context, define a package constant for the key and document how downstream consumers should use it.

## Troubleshooting

- **Sudo failures** – The `sudoensure` phase automatically tries to install sudo via `su` if it is missing. If both methods fail, ensure the provided password can `su - root` or grant the SSH user sudo privileges manually.
- **Python missing** – `pythonensure` uses `pkginstaller` to install Python via the system package manager. Check remote logs if the manager cannot detect a supported distro.
- **SSH key errors** – The ansible phase trims the public key before writing; verify the key path you provide is writable on your local machine. Keys are generated at the path you specify if they do not exist.

## Contributing

1. Fork or branch from `main`.
2. Run `just ci` before submitting a PR.
3. Document new context keys or inputs in `AGENTS.md` so automated agents stay aligned with expectations.

## License

Licensed under the [Apache License, Version 2.0](LICENSE).
