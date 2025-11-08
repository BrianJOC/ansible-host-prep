package phases

import (
	"context"
	"errors"
)

// Manager coordinates the ordered execution of phases.
type Manager struct {
	phases       []Phase
	observers    []Observer
	inputHandler InputHandler
}

// ManagerOption mutates manager configuration.
type ManagerOption func(*Manager)

// WithObserver registers an observer to receive lifecycle events.
func WithObserver(obs Observer) ManagerOption {
	return func(m *Manager) {
		if obs == nil {
			return
		}
		m.observers = append(m.observers, obs)
	}
}

// WithInputHandler registers a handler to satisfy input requests.
func WithInputHandler(handler InputHandler) ManagerOption {
	return func(m *Manager) {
		if handler == nil {
			return
		}
		m.inputHandler = handler
	}
}

// NewManager constructs an empty Manager.
func NewManager(opts ...ManagerOption) *Manager {
	m := &Manager{}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(m)
	}
	return m
}

// Register appends phases, returning an error on duplicate IDs.
func (m *Manager) Register(phases ...Phase) error {
	for _, p := range phases {
		if p == nil {
			continue
		}
		meta := p.Metadata()
		if meta.ID == "" {
			return ValidationError{Reason: "phase id must not be empty"}
		}
		if m.hasPhase(meta.ID) {
			return DuplicatePhaseError{ID: meta.ID}
		}
		m.phases = append(m.phases, p)
	}
	return nil
}

// Run executes all registered phases sequentially.
func (m *Manager) Run(ctx context.Context, phaseCtx *Context) error {
	if phaseCtx == nil {
		phaseCtx = NewContext()
	}
	for _, phase := range m.phases {
		meta := phase.Metadata()
		m.notifyStart(meta)
		err := m.executePhase(ctx, phaseCtx, phase, meta)
		m.notifyComplete(meta, err)
		if err != nil {
			return PhaseExecutionError{Phase: meta, Err: err}
		}
	}
	return nil
}

func (m *Manager) executePhase(ctx context.Context, phaseCtx *Context, phase Phase, meta PhaseMetadata) error {
	for {
		err := phase.Run(ctx, phaseCtx)
		if err == nil {
			return nil
		}
		var inputErr InputRequestError
		if errors.As(err, &inputErr) {
			if m.inputHandler == nil {
				return err
			}
			value, handlerErr := m.inputHandler.RequestInput(meta, inputErr.Input, inputErr.Reason)
			if handlerErr != nil {
				return handlerErr
			}
			SetInput(phaseCtx, inputErr.PhaseID, inputErr.Input.ID, value)
			continue
		}
		return err
	}
}

func (m *Manager) hasPhase(id string) bool {
	for _, p := range m.phases {
		if p.Metadata().ID == id {
			return true
		}
	}
	return false
}

func (m *Manager) notifyStart(meta PhaseMetadata) {
	for _, obs := range m.observers {
		obs.PhaseStarted(meta)
	}
}

func (m *Manager) notifyComplete(meta PhaseMetadata, err error) {
	for _, obs := range m.observers {
		obs.PhaseCompleted(meta, err)
	}
}
