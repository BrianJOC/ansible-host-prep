package systemuser

import (
	"fmt"
	"strings"
)

// RunnerError indicates EnsureUser was invoked without a valid runner.
type RunnerError struct{}

func (RunnerError) Error() string {
	return "runner is required"
}

// ValidationError captures bad input values passed to EnsureUser.
type ValidationError struct {
	Reason string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("user validation failed: %s", e.Reason)
}

// OptionError represents invalid option combinations.
type OptionError struct {
	Reason string
}

func (e OptionError) Error() string {
	return fmt.Sprintf("option error: %s", e.Reason)
}

// CommandError wraps failures when running remote commands.
type CommandError struct {
	Step   string
	Err    error
	Stderr string
}

func (e CommandError) Error() string {
	return fmt.Sprintf("%s failed: %v (%s)", e.Step, e.Err, strings.TrimSpace(e.Stderr))
}

func (e CommandError) Unwrap() error {
	return e.Err
}
