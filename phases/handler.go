package phases

// InputHandler resolves InputRequestError instances by collecting values from the operator or another system.
type InputHandler interface {
	RequestInput(phase PhaseMetadata, input InputDefinition, reason string) (any, error)
}

// InputHandlerFunc adapts a function into an InputHandler.
type InputHandlerFunc func(phase PhaseMetadata, input InputDefinition, reason string) (any, error)

// RequestInput implements InputHandler.
func (f InputHandlerFunc) RequestInput(phase PhaseMetadata, input InputDefinition, reason string) (any, error) {
	return f(phase, input, reason)
}
