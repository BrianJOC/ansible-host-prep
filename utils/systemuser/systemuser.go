package systemuser

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Runner executes commands on the target system with elevated privileges.
type Runner interface {
	Run(cmd string) (stdout string, stderr string, err error)
}

// Result reports what EnsureUser performed.
type Result struct {
	Username               string
	HomeDir                string
	UserCreated            bool
	AuthorizedKeyUpdated   bool
	AddedToSudo            bool
	PasswordlessConfigured bool
}

// Option configures EnsureUser behavior.
type Option func(*ensureUserOptions) error

type ensureUserOptions struct {
	shell            string
	homeDir          string
	addToSudo        bool
	passwordlessSudo bool
	sudoGroup        string
	sudoersDir       string
}

// WithShell overrides the login shell assigned to the user.
func WithShell(shell string) Option {
	return func(opts *ensureUserOptions) error {
		shell = strings.TrimSpace(shell)
		if shell == "" {
			return OptionError{Reason: "shell must not be empty"}
		}
		opts.shell = shell
		return nil
	}
}

// WithHomeDir overrides the home directory assigned to the user.
func WithHomeDir(dir string) Option {
	return func(opts *ensureUserOptions) error {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			return OptionError{Reason: "home directory must not be empty"}
		}
		opts.homeDir = dir
		return nil
	}
}

// WithSudoAccess ensures the user is added to the sudo group.
func WithSudoAccess() Option {
	return func(opts *ensureUserOptions) error {
		opts.addToSudo = true
		return nil
	}
}

// WithPasswordlessSudo configures /etc/sudoers.d for NOPASSWD access.
func WithPasswordlessSudo() Option {
	return func(opts *ensureUserOptions) error {
		opts.addToSudo = true
		opts.passwordlessSudo = true
		return nil
	}
}

// WithSudoGroup overrides the primary sudo-capable group (default "sudo").
func WithSudoGroup(group string) Option {
	return func(opts *ensureUserOptions) error {
		group = strings.TrimSpace(group)
		if group == "" {
			return OptionError{Reason: "sudo group must not be empty"}
		}
		opts.sudoGroup = group
		return nil
	}
}

// WithSudoersDir overrides the location used for sudoers drop-ins.
func WithSudoersDir(dir string) Option {
	return func(opts *ensureUserOptions) error {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			return OptionError{Reason: "sudoers dir must not be empty"}
		}
		opts.sudoersDir = dir
		return nil
	}
}

// EnsureUser provisions a local user with SSH access and optional sudo privileges.
func EnsureUser(r Runner, username, publicKey string, opts ...Option) (*Result, error) {
	if r == nil {
		return nil, RunnerError{}
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return nil, ValidationError{Reason: "username is required"}
	}
	if strings.Contains(username, " ") {
		return nil, ValidationError{Reason: "username must not contain spaces"}
	}

	publicKey = strings.TrimSpace(publicKey)
	if publicKey == "" {
		return nil, ValidationError{Reason: "public key is required"}
	}

	config := ensureUserOptions{
		shell:      "/bin/bash",
		sudoGroup:  "sudo",
		sudoersDir: "/etc/sudoers.d",
	}

	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(&config); err != nil {
			return nil, err
		}
	}

	if config.homeDir == "" {
		config.homeDir = filepath.Join("/home", username)
	}

	result := &Result{
		Username: username,
		HomeDir:  config.homeDir,
	}

	exists := userExists(r, username)
	if !exists {
		if err := createUser(r, username, config.homeDir, config.shell); err != nil {
			return nil, err
		}
		result.UserCreated = true
	}

	if err := ensureAuthorizedKey(r, username, config.homeDir, publicKey); err != nil {
		return nil, err
	}
	result.AuthorizedKeyUpdated = true

	if config.addToSudo {
		if err := addUserToSudo(r, username, config.sudoGroup); err != nil {
			return nil, err
		}
		result.AddedToSudo = true
	}

	if config.passwordlessSudo {
		if err := configurePasswordlessSudo(r, username, config.sudoersDir); err != nil {
			return nil, err
		}
		result.PasswordlessConfigured = true
	}

	return result, nil
}

func userExists(r Runner, username string) bool {
	cmd := fmt.Sprintf("id -u %s >/dev/null 2>&1", shellQuote(username))
	_, _, err := r.Run(cmd)
	return err == nil
}

func createUser(r Runner, username, homeDir, shell string) error {
	cmd := fmt.Sprintf("useradd -m -d %s -s %s %s", shellQuote(homeDir), shellQuote(shell), shellQuote(username))
	return runStep(r, "useradd", cmd)
}

func ensureAuthorizedKey(r Runner, username, homeDir, publicKey string) error {
	sshDir := filepath.Join(homeDir, ".ssh")
	authPath := filepath.Join(sshDir, "authorized_keys")
	script := fmt.Sprintf(`
set -euo pipefail
install -o %s -g %s -m 700 -d %s
cat <<'EOF' > %s
%s
EOF
chown %s:%s %s
chmod 600 %s
`, shellQuote(username), shellQuote(username), shellQuote(sshDir),
		shellQuote(authPath), publicKey, shellQuote(username), shellQuote(username),
		shellQuote(authPath), shellQuote(authPath))

	return runStep(r, "authorized_keys", script)
}

func addUserToSudo(r Runner, username, group string) error {
	cmd := fmt.Sprintf("usermod -aG %s %s", shellQuote(group), shellQuote(username))
	return runStep(r, "add-to-sudo", cmd)
}

func configurePasswordlessSudo(r Runner, username, sudoersDir string) error {
	file := filepath.Join(sudoersDir, username)
	script := fmt.Sprintf(`
set -euo pipefail
install -o root -g root -m 755 -d %s
cat <<'EOF' > %s
%s ALL=(ALL) NOPASSWD:ALL
EOF
chmod 440 %s
`, shellQuote(sudoersDir), shellQuote(file), username, shellQuote(file))
	return runStep(r, "passwordless-sudo", script)
}

func runStep(r Runner, step, cmd string) error {
	_, stderr, err := r.Run(cmd)
	if err != nil {
		return CommandError{Step: step, Err: err, Stderr: stderr}
	}
	return nil
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
