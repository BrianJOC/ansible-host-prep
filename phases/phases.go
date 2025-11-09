package phases

import "context"

// Phase represents a single unit of work in the bootstrap pipeline.
type Phase interface {
	Metadata() PhaseMetadata
	Run(ctx context.Context, phaseCtx *Context) error
}

// PhaseMetadata contains descriptive information used by presentation layers (e.g., TUI).
type PhaseMetadata struct {
	ID          string
	Title       string
	Description string
	Inputs      []InputDefinition
	Tags        []string
}

// Observer receives lifecycle callbacks for each phase.
type Observer interface {
	PhaseStarted(meta PhaseMetadata)
	PhaseCompleted(meta PhaseMetadata, err error)
}

// InputDefinition describes data a phase requires from the operator/UI.
type InputDefinition struct {
	ID          string
	Label       string
	Description string
	Kind        InputKind
	Required    bool
	Secret      bool
	Options     []InputOption
	Default     any
}

// InputKind identifies how an input should be rendered.
type InputKind string

const (
	InputKindText   InputKind = "text"
	InputKindSecret InputKind = "secret"
	InputKindSelect InputKind = "select"
)

// InputOption represents a selectable value.
type InputOption struct {
	Value       string
	Label       string
	Description string
}
