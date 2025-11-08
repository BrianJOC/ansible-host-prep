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

// InputRequestError indicates a phase requires additional input from the operator before continuing.
type InputRequestError struct {
	PhaseID string
	Input   InputDefinition
	Reason  string
}

func (e InputRequestError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("phase %s requires input %s: %s", e.PhaseID, e.Input.ID, e.Reason)
	}
	return fmt.Sprintf("phase %s requires input %s", e.PhaseID, e.Input.ID)
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
