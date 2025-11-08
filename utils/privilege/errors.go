package privilege

import (
	"fmt"
	"strings"
)

// NilClientError indicates EnsureElevatedClient received a nil SSH client.
type NilClientError struct{}

func (NilClientError) Error() string {
	return "ssh client is required"
}

// PasswordError captures validation issues with the password object.
type PasswordError struct {
	Reason string
}

func (e PasswordError) Error() string {
	return fmt.Sprintf("password error: %s", e.Reason)
}

// SudoPermissionError indicates the current user is not allowed to use sudo.
type SudoPermissionError struct {
	Stderr string
}

func (e SudoPermissionError) Error() string {
	return fmt.Sprintf("sudo permission denied: %s", strings.TrimSpace(e.Stderr))
}

// SudoNotInstalledError indicates the sudo binary is missing on the target.
type SudoNotInstalledError struct {
	Stderr string
}

func (e SudoNotInstalledError) Error() string {
	return fmt.Sprintf("sudo not installed: %s", strings.TrimSpace(e.Stderr))
}

// SudoAuthenticationError wraps incorrect sudo password attempts.
type SudoAuthenticationError struct {
	Err error
}

func (e SudoAuthenticationError) Error() string {
	return fmt.Sprintf("sudo authentication failed: %v", e.Err)
}

func (e SudoAuthenticationError) Unwrap() error {
	return e.Err
}

// SudoUnknownError surfaces unclassified sudo failures.
type SudoUnknownError struct {
	Err    error
	Stderr string
}

func (e SudoUnknownError) Error() string {
	return fmt.Sprintf("sudo failed: %v (%s)", e.Err, strings.TrimSpace(e.Stderr))
}

func (e SudoUnknownError) Unwrap() error {
	return e.Err
}

// SuAuthenticationError reports bad passwords when invoking su.
type SuAuthenticationError struct {
	Err error
}

func (e SuAuthenticationError) Error() string {
	return fmt.Sprintf("su authentication failed: %v", e.Err)
}

func (e SuAuthenticationError) Unwrap() error {
	return e.Err
}

// SuUnavailableError represents generic su failures.
type SuUnavailableError struct {
	Err    error
	Stderr string
}

func (e SuUnavailableError) Error() string {
	return fmt.Sprintf("su unavailable: %v (%s)", e.Err, strings.TrimSpace(e.Stderr))
}

func (e SuUnavailableError) Unwrap() error {
	return e.Err
}

// EnsureSudoError wraps failures when attempting to install sudo.
type EnsureSudoError struct {
	Err    error
	Stderr string
}

func (e EnsureSudoError) Error() string {
	return fmt.Sprintf("failed to ensure sudo: %v (%s)", e.Err, strings.TrimSpace(e.Stderr))
}

func (e EnsureSudoError) Unwrap() error {
	return e.Err
}
