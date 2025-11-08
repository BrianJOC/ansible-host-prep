package phases

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManagerRunsPhasesSequentially(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	phaseCtx := NewContext()

	var order []string
	phaseA := &fakePhase{
		meta: PhaseMetadata{ID: "ssh", Title: "SSH", Description: "connect"},
		run: func(context.Context, *Context) error {
			order = append(order, "ssh")
			return nil
		},
	}
	phaseB := &fakePhase{
		meta: PhaseMetadata{ID: "sudo", Title: "Sudo", Description: "elevate"},
		run: func(context.Context, *Context) error {
			order = append(order, "sudo")
			return nil
		},
	}

	manager := NewManager()
	require.NoError(t, manager.Register(phaseA, phaseB))
	require.NoError(t, manager.Run(ctx, phaseCtx))
	require.Equal(t, []string{"ssh", "sudo"}, order)
}

func TestManagerStopsOnError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	failErr := errors.New("boom")
	phase := &fakePhase{
		meta: PhaseMetadata{ID: "ssh"},
		run: func(context.Context, *Context) error {
			return failErr
		},
	}

	manager := NewManager()
	require.NoError(t, manager.Register(phase))
	err := manager.Run(ctx, nil)
	require.Error(t, err)
	var execErr PhaseExecutionError
	require.ErrorAs(t, err, &execErr)
	require.Equal(t, "ssh", execErr.Phase.ID)
	require.ErrorIs(t, err, failErr)
}

func TestManagerObserverNotifications(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	var mu sync.Mutex
	var started []string
	var completed []string

	observer := ObserverFunc{
		OnStart: func(meta PhaseMetadata) {
			mu.Lock()
			defer mu.Unlock()
			started = append(started, meta.ID)
		},
		OnComplete: func(meta PhaseMetadata, err error) {
			mu.Lock()
			defer mu.Unlock()
			completed = append(completed, meta.ID)
		},
	}

	manager := NewManager(WithObserver(observer))
	require.NoError(t, manager.Register(&fakePhase{
		meta: PhaseMetadata{ID: "ssh"},
		run:  func(context.Context, *Context) error { return nil },
	}))
	require.NoError(t, manager.Run(ctx, nil))

	require.Equal(t, []string{"ssh"}, started)
	require.Equal(t, []string{"ssh"}, completed)
}

func TestManagerDetectsDuplicates(t *testing.T) {
	t.Parallel()

	manager := NewManager()
	err := manager.Register(&fakePhase{meta: PhaseMetadata{ID: "ssh"}}, &fakePhase{meta: PhaseMetadata{ID: "ssh"}})
	require.Error(t, err)
	require.IsType(t, DuplicatePhaseError{}, err)
}

func TestManagerHandlesInputRequest(t *testing.T) {
	t.Parallel()

	var attempts int
	phase := &fakePhase{
		meta: PhaseMetadata{ID: "sudo"},
		run: func(ctx context.Context, c *Context) error {
			attempts++
			if val, ok := GetInput(c, "sudo", "password"); ok && val != "" {
				return nil
			}
			return InputRequestError{
				PhaseID: "sudo",
				Input: InputDefinition{
					ID:       "password",
					Label:    "Password",
					Kind:     InputKindSecret,
					Required: true,
				},
				Reason: "required",
			}
		},
	}

	handlerCalls := 0
	handler := InputHandlerFunc(func(meta PhaseMetadata, input InputDefinition, reason string) (any, error) {
		handlerCalls++
		require.Equal(t, "sudo", meta.ID)
		require.Equal(t, "password", input.ID)
		return "secret", nil
	})

	manager := NewManager(WithInputHandler(handler))
	require.NoError(t, manager.Register(phase))
	err := manager.Run(context.Background(), NewContext())
	require.NoError(t, err)
	require.Equal(t, 2, attempts)
	require.Equal(t, 1, handlerCalls)
}

func TestManagerInputHandlerError(t *testing.T) {
	t.Parallel()

	phase := &fakePhase{
		meta: PhaseMetadata{ID: "sudo"},
		run: func(context.Context, *Context) error {
			return InputRequestError{
				PhaseID: "sudo",
				Input: InputDefinition{
					ID: "password",
				},
			}
		},
	}

	manager := NewManager(WithInputHandler(InputHandlerFunc(func(PhaseMetadata, InputDefinition, string) (any, error) {
		return nil, fmt.Errorf("user cancelled")
	})))

	require.NoError(t, manager.Register(phase))
	err := manager.Run(context.Background(), NewContext())
	require.Error(t, err)
	var execErr PhaseExecutionError
	require.ErrorAs(t, err, &execErr)
	require.ErrorContains(t, execErr, "user cancelled")
}

func TestManagerPropagatesInputRequestWithoutHandler(t *testing.T) {
	t.Parallel()

	phase := &fakePhase{
		meta: PhaseMetadata{ID: "sudo"},
		run: func(context.Context, *Context) error {
			return InputRequestError{
				PhaseID: "sudo",
				Input: InputDefinition{
					ID: "password",
				},
			}
		},
	}

	manager := NewManager()
	require.NoError(t, manager.Register(phase))
	err := manager.Run(context.Background(), NewContext())
	require.Error(t, err)
	var execErr PhaseExecutionError
	require.ErrorAs(t, err, &execErr)
	var inputErr InputRequestError
	require.ErrorAs(t, execErr.Err, &inputErr)
}

type fakePhase struct {
	meta PhaseMetadata
	run  func(context.Context, *Context) error
}

func (p *fakePhase) Metadata() PhaseMetadata {
	return p.meta
}

func (p *fakePhase) Run(ctx context.Context, c *Context) error {
	return p.run(ctx, c)
}

// ObserverFunc allows using functions for Observer callbacks.
type ObserverFunc struct {
	OnStart    func(meta PhaseMetadata)
	OnComplete func(meta PhaseMetadata, err error)
}

func (o ObserverFunc) PhaseStarted(meta PhaseMetadata) {
	if o.OnStart != nil {
		o.OnStart(meta)
	}
}

func (o ObserverFunc) PhaseCompleted(meta PhaseMetadata, err error) {
	if o.OnComplete != nil {
		o.OnComplete(meta, err)
	}
}
