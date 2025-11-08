package privilege

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

const ensureSudoScript = `
set -euo pipefail
if command -v sudo >/dev/null 2>&1; then
	exit 0
fi
if command -v apt-get >/dev/null 2>&1; then
	apt-get update -y >/dev/null 2>&1 && apt-get install -y sudo >/dev/null 2>&1
elif command -v yum >/dev/null 2>&1; then
	yum install -y sudo >/dev/null 2>&1
elif command -v dnf >/dev/null 2>&1; then
	dnf install -y sudo >/dev/null 2>&1
elif command -v zypper >/dev/null 2>&1; then
	zypper --non-interactive install -y sudo >/dev/null 2>&1
else
	echo "unable to install sudo: no supported package manager found" >&2
	exit 1
fi
`

type elevationMethod string

const (
	methodSudo elevationMethod = "sudo"
	methodSu   elevationMethod = "su"
)

// Password wraps the credential used for privilege escalation.
type Password struct {
	Value string
}

// ElevatedClient ensures privileged commands are executed with the chosen method.
type ElevatedClient struct {
	client   *ssh.Client
	method   elevationMethod
	password string
}

// Client exposes the underlying SSH client.
func (c *ElevatedClient) Client() *ssh.Client {
	return c.client
}

// Method returns how elevation is performed ("sudo" or "su").
func (c *ElevatedClient) Method() string {
	return string(c.method)
}

// Run executes the given command with elevated privileges and returns stdout/stderr.
func (c *ElevatedClient) Run(cmd string) (string, string, error) {
	runner := &sshRunner{client: c.client}
	return runPrivileged(runner, c.method, c.password, cmd)
}

// EnsureElevatedClient verifies privileged access and installs sudo when necessary.
func EnsureElevatedClient(client *ssh.Client, password Password) (*ElevatedClient, error) {
	if client == nil {
		return nil, NilClientError{}
	}

	pass, err := password.validate()
	if err != nil {
		return nil, err
	}

	runner := &sshRunner{client: client}
	method, err := ensureElevation(runner, pass)
	if err != nil {
		return nil, err
	}

	return &ElevatedClient{
		client:   client,
		method:   method,
		password: pass,
	}, nil
}

func ensureElevation(r runner, password string) (elevationMethod, error) {
	if err := validateSudo(r, password); err == nil {
		if err := ensureSudoInstalled(r, methodSudo, password); err != nil {
			return "", err
		}
		return methodSudo, nil
	} else {
		var permErr SudoPermissionError
		var missingErr SudoNotInstalledError
		var authErr SudoAuthenticationError
		switch {
		case errors.As(err, &authErr):
			return "", err
		case errors.As(err, &permErr):
			if err := ensureRootViaSu(r, password); err != nil {
				return "", err
			}
			if err := ensureSudoInstalled(r, methodSu, password); err != nil {
				return "", err
			}
			return methodSu, nil
		case errors.As(err, &missingErr):
			if err := ensureRootViaSu(r, password); err != nil {
				return "", err
			}
			if err := ensureSudoInstalled(r, methodSu, password); err != nil {
				return "", err
			}
			if err := validateSudo(r, password); err == nil {
				if err := ensureSudoInstalled(r, methodSudo, password); err != nil {
					return "", err
				}
				return methodSudo, nil
			}
			return methodSu, nil
		default:
			return "", err
		}
	}
}

type runner interface {
	Run(cmd string, stdin string) (string, string, error)
}

type sshRunner struct {
	client *ssh.Client
}

func (r *sshRunner) Run(cmd string, stdin string) (string, string, error) {
	session, err := r.client.NewSession()
	if err != nil {
		return "", "", err
	}
	defer func() {
		_ = session.Close()
	}()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	if stdin != "" {
		session.Stdin = strings.NewReader(stdin)
	}

	err = session.Run(cmd)
	return stdout.String(), stderr.String(), err
}

func runPrivileged(r runner, method elevationMethod, password, cmd string) (string, string, error) {
	quotedCmd := shellQuote(cmd)
	switch method {
	case methodSudo:
		command := fmt.Sprintf("sudo -S -p '' -k bash -c %s", quotedCmd)
		return r.Run(command, password+"\n")
	case methodSu:
		command := fmt.Sprintf("su - root -c %s", quotedCmd)
		return r.Run(command, password+"\n")
	default:
		return "", "", fmt.Errorf("unsupported elevation method %q", method)
	}
}

func ensureRootViaSu(r runner, password string) error {
	_, stderr, err := runPrivileged(r, methodSu, password, "true")
	if err != nil {
		if isAuthenticationFailure(stderr) {
			return SuAuthenticationError{Err: err}
		}
		return SuUnavailableError{Err: err, Stderr: stderr}
	}
	return nil
}

func ensureSudoInstalled(r runner, method elevationMethod, password string) error {
	encoded := base64.StdEncoding.EncodeToString([]byte(ensureSudoScript))
	command := fmt.Sprintf("printf %%s %s | base64 -d | bash", shellQuote(encoded))
	_, stderr, err := runPrivileged(r, method, password, command)
	if err != nil {
		return EnsureSudoError{Err: err, Stderr: stderr}
	}
	return nil
}

func validateSudo(r runner, password string) error {
	_, stderr, err := runPrivileged(r, methodSudo, password, "true")
	if err == nil {
		return nil
	}

	if strings.Contains(stderr, "sudo: command not found") {
		return SudoNotInstalledError{Stderr: stderr}
	}

	if strings.Contains(stderr, "is not in the sudoers file") || strings.Contains(stderr, "may not run sudo") {
		return SudoPermissionError{Stderr: stderr}
	}

	if isAuthenticationFailure(stderr) || strings.Contains(stderr, "Sorry, try again.") {
		return SudoAuthenticationError{Err: err}
	}

	return SudoUnknownError{Err: err, Stderr: stderr}
}

func isAuthenticationFailure(stderr string) bool {
	return strings.Contains(stderr, "Authentication failure") ||
		strings.Contains(stderr, "authentication failure") ||
		strings.Contains(stderr, "incorrect password")
}

func (p Password) validate() (string, error) {
	if p.Value == "" {
		return "", PasswordError{Reason: "password must not be empty"}
	}
	return p.Value, nil
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
