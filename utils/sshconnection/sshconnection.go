package sshconnection

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	defaultPort        = 22
	defaultDialTimeout = 10 * time.Second
)

// Credential represents either a password or private key path for SSH authentication.
type Credential struct {
	Password string
	KeyPath  string
}

// Option configures optional behavior for Connect.
type Option func(*connectOptions) error

type connectOptions struct {
	timeout time.Duration
}

// WithTimeout overrides the default dial timeout.
func WithTimeout(d time.Duration) Option {
	return func(opts *connectOptions) error {
		if d <= 0 {
			return OptionError{Reason: "timeout must be greater than zero"}
		}
		opts.timeout = d
		return nil
	}
}

// OptionError captures invalid option state passed to Connect.
type OptionError struct {
	Reason string
}

func (e OptionError) Error() string {
	return fmt.Sprintf("invalid option: %s", e.Reason)
}

// Connect establishes an SSH client to the provided host using the supplied credentials.
func Connect(host string, port int, username string, cred Credential, opts ...Option) (*ssh.Client, error) {
	host = strings.TrimSpace(host)
	username = strings.TrimSpace(username)

	if host == "" {
		return nil, InvalidTargetError{Field: "host"}
	}

	if username == "" {
		return nil, InvalidTargetError{Field: "username"}
	}

	authMethod, err := cred.authMethod()
	if err != nil {
		return nil, err
	}

	if port <= 0 {
		port = defaultPort
	}

	cfg := connectOptions{
		timeout: defaultDialTimeout,
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}

	config := &ssh.ClientConfig{
		User:            username,
		Auth:            []ssh.AuthMethod{authMethod},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // callers should replace when host key management is available
		Timeout:         cfg.timeout,
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
			return nil, TimeoutError{Addr: addr, Err: err}
		}

		if strings.Contains(err.Error(), "unable to authenticate") {
			return nil, AuthenticationError{Username: username, Err: err}
		}

		return nil, DialError{Addr: addr, Err: err}
	}

	return client, nil
}

func (c Credential) authMethod() (ssh.AuthMethod, error) {
	hasPassword := strings.TrimSpace(c.Password) != ""
	hasKey := strings.TrimSpace(c.KeyPath) != ""

	switch {
	case hasPassword && hasKey:
		return nil, CredentialError{Reason: "provide either password or key path, not both"}
	case !hasPassword && !hasKey:
		return nil, CredentialError{Reason: "password or key path required"}
	}

	if hasPassword {
		return ssh.Password(c.Password), nil
	}

	keyBytes, err := os.ReadFile(c.KeyPath)
	if err != nil {
		return nil, KeyLoadError{Path: c.KeyPath, Err: err}
	}

	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil {
		return nil, KeyParseError{Path: c.KeyPath, Err: err}
	}

	return ssh.PublicKeys(signer), nil
}
