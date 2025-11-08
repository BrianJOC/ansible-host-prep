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
}

// Observer receives lifecycle callbacks for each phase.
type Observer interface {
	PhaseStarted(meta PhaseMetadata)
	PhaseCompleted(meta PhaseMetadata, err error)
}
