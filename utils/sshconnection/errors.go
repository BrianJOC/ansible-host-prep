package sshconnection

import (
	"fmt"
)

// InvalidTargetError indicates a required connection target parameter is missing.
type InvalidTargetError struct {
	Field string
}

func (e InvalidTargetError) Error() string {
	return fmt.Sprintf("invalid SSH target: %s is required", e.Field)
}

// CredentialError indicates the credential payload cannot be used to authenticate.
type CredentialError struct {
	Reason string
}

func (e CredentialError) Error() string {
	return fmt.Sprintf("invalid credential: %s", e.Reason)
}

// KeyLoadError wraps failures when reading a private key from disk.
type KeyLoadError struct {
	Path string
	Err  error
}

func (e KeyLoadError) Error() string {
	return fmt.Sprintf("failed to load private key from %s: %v", e.Path, e.Err)
}

func (e KeyLoadError) Unwrap() error {
	return e.Err
}

// KeyParseError wraps failures when parsing the loaded private key bytes.
type KeyParseError struct {
	Path string
	Err  error
}

func (e KeyParseError) Error() string {
	return fmt.Sprintf("failed to parse private key %s: %v", e.Path, e.Err)
}

func (e KeyParseError) Unwrap() error {
	return e.Err
}

// AuthenticationError represents SSH handshake failures due to invalid credentials.
type AuthenticationError struct {
	Username string
	Err      error
}

func (e AuthenticationError) Error() string {
	return fmt.Sprintf("authentication failed for %s: %v", e.Username, e.Err)
}

func (e AuthenticationError) Unwrap() error {
	return e.Err
}

// DialError encapsulates lower-level network failures when reaching the host.
type DialError struct {
	Addr string
	Err  error
}

func (e DialError) Error() string {
	return fmt.Sprintf("failed to dial %s: %v", e.Addr, e.Err)
}

func (e DialError) Unwrap() error {
	return e.Err
}

// TimeoutError is returned when the dial operation exceeds the configured timeout.
type TimeoutError struct {
	Addr string
	Err  error
}

func (e TimeoutError) Error() string {
	return fmt.Sprintf("timeout while connecting to %s: %v", e.Addr, e.Err)
}

func (e TimeoutError) Unwrap() error {
	return e.Err
}
