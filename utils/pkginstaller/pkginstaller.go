package pkginstaller

import (
	"fmt"
	"strings"
)

// Runner executes commands on the target system.
type Runner interface {
	Run(cmd string) (stdout string, stderr string, err error)
}

// Result reports actions taken by Installer.
type Result struct {
	PackageName string
	Installed   bool
	Skipped     bool
}

// Option configures Installer behavior.
type Option func(*options) error

type options struct {
	checkCmd string
	force    bool
}

// WithCustomCheck overrides the command used to detect existing packages.
func WithCustomCheck(cmd string) Option {
	return func(opts *options) error {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			return OptionError{Reason: "custom check command must not be empty"}
		}
		opts.checkCmd = cmd
		return nil
	}
}

// WithForce forces installation even if the check passes.
func WithForce() Option {
	return func(opts *options) error {
		opts.force = true
		return nil
	}
}

// Ensure installs the package when missing using the first available package manager.
func Ensure(r Runner, packageName string, opts ...Option) (*Result, error) {
	if r == nil {
		return nil, RunnerError{}
	}

	packageName = strings.TrimSpace(packageName)
	if packageName == "" {
		return nil, ValidationError{Reason: "package name is required"}
	}

	config := options{}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(&config); err != nil {
			return nil, err
		}
	}

	result := &Result{PackageName: packageName}
	if !config.force {
		checkCmd := config.checkCmd
		if checkCmd == "" {
			checkCmd = fmt.Sprintf("command -v %s >/dev/null 2>&1", shellQuote(packageName))
		}
		if err := runCheck(r, checkCmd); err == nil {
			result.Skipped = true
			return result, nil
		}
	}

	installCmd, err := buildInstallCommand(packageName)
	if err != nil {
		return nil, err
	}

	if err := runInstall(r, installCmd); err != nil {
		return nil, err
	}

	result.Installed = true
	return result, nil
}

func runCheck(r Runner, cmd string) error {
	_, _, err := r.Run(cmd)
	return err
}

func buildInstallCommand(packageName string) (string, error) {
	quoted := shellQuote(packageName)
	cmd := fmt.Sprintf(`
set -euo pipefail
if command -v apt-get >/dev/null 2>&1; then
	export DEBIAN_FRONTEND=noninteractive
	apt-get update -y >/dev/null 2>&1
	apt-get install -y %s
elif command -v yum >/dev/null 2>&1; then
	yum install -y %s
elif command -v dnf >/dev/null 2>&1; then
	dnf install -y %s
elif command -v zypper >/dev/null 2>&1; then
	zypper --non-interactive install -y %s
else
	echo "no supported package manager found" >&2
	exit 1
fi
`, quoted, quoted, quoted, quoted)
	return cmd, nil
}

func runInstall(r Runner, cmd string) error {
	_, stderr, err := r.Run(cmd)
	if err != nil {
		return CommandError{Step: "install", Err: err, Stderr: stderr}
	}
	return nil
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
