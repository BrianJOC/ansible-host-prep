package pkginstaller

import (
	"fmt"
)

// RunnerError indicates the installer was invoked without a runner.
type RunnerError struct{}

func (RunnerError) Error() string {
	return "runner is required"
}

// ValidationError captures invalid package inputs.
type ValidationError struct {
	Reason string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("package validation failed: %s", e.Reason)
}

// OptionError surfaces invalid installer options.
type OptionError struct {
	Reason string
}

func (e OptionError) Error() string {
	return fmt.Sprintf("installer option error: %s", e.Reason)
}

// CommandError wraps execution failures from the remote host.
type CommandError struct {
	Step   string
	Err    error
	Stderr string
}

func (e CommandError) Error() string {
	return fmt.Sprintf("%s failed: %v (%s)", e.Step, e.Err, e.Stderr)
}

func (e CommandError) Unwrap() error {
	return e.Err
}
