package phases

import "fmt"

// DuplicatePhaseError occurs when a phase with an existing ID is registered.
type DuplicatePhaseError struct {
	ID string
}

func (e DuplicatePhaseError) Error() string {
	return fmt.Sprintf("phase with id %q already registered", e.ID)
}

// ValidationError represents invalid manager/phase configuration.
type ValidationError struct {
	Reason string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("phase validation failed: %s", e.Reason)
}

// PhaseExecutionError wraps failures emitted by a specific phase.
type PhaseExecutionError struct {
	Phase PhaseMetadata
	Err   error
}

func (e PhaseExecutionError) Error() string {
	return fmt.Sprintf("phase %s failed: %v", e.Phase.ID, e.Err)
}

func (e PhaseExecutionError) Unwrap() error {
	return e.Err
}
