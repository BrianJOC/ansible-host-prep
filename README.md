# ansible-host-prep

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
go run ./tui             # identical to `just tui`
just test                # go test ./...
```

## Repository Layout

```
cmd/bootstrap-tui   # CLI entrypoint used by `just run`
phases/             # Phase manager plus sshconnect, sudoensure, pythonensure, ansibleuser
tui/                # Bubble Tea program that orchestrates phases + prompts for inputs
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
| Run full TUI  | `just tui`              |
| CI bundle     | `just ci`               |

### Adding a Phase

1. Create a package under `phases/<name>`.
2. Implement `phases.Phase` with metadata (ID, title, description, inputs) and a `Run` method that reads/writes `phases.Context`.
3. Register the new phase in `tui/main.go` (or wherever the manager is constructed) in the desired order.
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

Specify your preferred license here (e.g., MIT, Apache-2.0). Until then, contributions default to the repository owner’s terms.
