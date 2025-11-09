package phasedapp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	phasespkg "github.com/BrianJOC/ansible-host-prep/phases"
)

func TestNewRequiresPhases(t *testing.T) {
	t.Parallel()
	if _, err := New(); !errors.Is(err, ErrNoPhases) {
		t.Fatalf("expected ErrNoPhases, got %v", err)
	}
}

func TestAppStartRunsPhases(t *testing.T) {
	t.Parallel()

	observer := newRecordingObserver(2)
	app := newTestApp(t,
		WithPhases(newStubPhase("one"), newStubPhase("two")),
		WithManagerOptions(phasespkg.WithObserver(observer)),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := runAppAsync(app, ctx)
	observer.wait(t, time.Second)

	if err := app.Stop(); err != nil {
		t.Fatalf("stop error: %v", err)
	}
	assertNoError(t, errCh)

	want := []string{"start:one", "complete:one", "start:two", "complete:two"}
	if got := observer.events(); !equalStrings(got, want) {
		t.Fatalf("unexpected events: got %v want %v", got, want)
	}
}

func TestAppStartFromSkipsLeadingPhases(t *testing.T) {
	t.Parallel()

	observer := newRecordingObserver(2)
	app := newTestApp(t,
		WithPhases(newStubPhase("zero"), newStubPhase("one"), newStubPhase("two")),
		WithManagerOptions(phasespkg.WithObserver(observer)),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := runAppAsyncStartFrom(app, ctx, 1)
	observer.wait(t, time.Second)

	if err := app.Stop(); err != nil {
		t.Fatalf("stop error: %v", err)
	}
	assertNoError(t, errCh)

	want := []string{"start:one", "complete:one", "start:two", "complete:two"}
	if got := observer.events(); !equalStrings(got, want) {
		t.Fatalf("unexpected events: got %v want %v", got, want)
	}
}

func TestAppStartFromBeyondEndIsNoop(t *testing.T) {
	t.Parallel()

	observer := newRecordingObserver(0)
	app := newTestApp(t,
		WithPhases(newStubPhase("only")),
		WithManagerOptions(phasespkg.WithObserver(observer)),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := runAppAsyncStartFrom(app, ctx, 5)

	time.Sleep(50 * time.Millisecond)

	if len(observer.events()) != 0 {
		t.Fatalf("expected no events, got %v", observer.events())
	}

	if err := app.Stop(); err != nil {
		t.Fatalf("stop error: %v", err)
	}
	assertNoError(t, errCh)
}

func TestAppRejectsConcurrentStart(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	blocking := newStubPhaseFunc("block", func(ctx context.Context, _ *phasespkg.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
			return nil
		}
	})

	app := newTestApp(t, WithPhases(blocking))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := runAppAsync(app, ctx)

	// Give the first Start call a moment to initialize.
	time.Sleep(50 * time.Millisecond)

	if err := app.Start(ctx); !errors.Is(err, ErrProgramRunning) {
		t.Fatalf("expected ErrProgramRunning, got %v", err)
	}

	close(release)

	if err := app.Stop(); err != nil {
		t.Fatalf("stop error: %v", err)
	}
	assertNoError(t, errCh)
}

func TestAppStartReturnsWhenContextCancelled(t *testing.T) {
	t.Parallel()

	blocking := newStubPhaseFunc("block", func(ctx context.Context, _ *phasespkg.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	app := newTestApp(t, WithPhases(blocking))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := runAppAsync(app, ctx)

	cancel()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("expected nil or context cancellation error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("start did not return after cancellation")
	}
}

// --- helpers ---

func newTestApp(t *testing.T, opts ...Option) *App {
	t.Helper()
	headlessInput := bytes.NewBuffer(nil)
	opts = append(opts, WithProgramOptions(
		tea.WithoutRenderer(),
		tea.WithInput(headlessInput),
		tea.WithOutput(io.Discard),
	))
	app, err := New(opts...)
	if err != nil {
		t.Fatalf("app init error: %v", err)
	}
	return app
}

func runAppAsync(app *App, ctx context.Context) chan error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Start(ctx)
	}()
	return errCh
}

func runAppAsyncStartFrom(app *App, ctx context.Context, start int) chan error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- app.StartFrom(ctx, start)
	}()
	return errCh
}

func assertNoError(t *testing.T, errCh <-chan error) {
	t.Helper()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("app exited with error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("app did not exit")
	}
}

type stubPhase struct {
	meta phasespkg.PhaseMetadata
	run  func(ctx context.Context, phaseCtx *phasespkg.Context) error
}

func newStubPhase(id string) phasespkg.Phase {
	return stubPhase{
		meta: phasespkg.PhaseMetadata{
			ID:    id,
			Title: id,
		},
	}
}

func newStubPhaseFunc(id string, fn func(context.Context, *phasespkg.Context) error) phasespkg.Phase {
	return stubPhase{
		meta: phasespkg.PhaseMetadata{
			ID:    id,
			Title: id,
		},
		run: fn,
	}
}

func (s stubPhase) Metadata() phasespkg.PhaseMetadata {
	return s.meta
}

func (s stubPhase) Run(ctx context.Context, phaseCtx *phasespkg.Context) error {
	if s.run != nil {
		return s.run(ctx, phaseCtx)
	}
	return nil
}

type recordingObserver struct {
	target int

	mu        sync.Mutex
	eventLog  []string
	completed int
	done      chan struct{}
}

func newRecordingObserver(target int) *recordingObserver {
	return &recordingObserver{
		target: target,
		done:   make(chan struct{}),
	}
}

func (o *recordingObserver) PhaseStarted(meta phasespkg.PhaseMetadata) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.eventLog = append(o.eventLog, "start:"+meta.ID)
}

func (o *recordingObserver) PhaseCompleted(meta phasespkg.PhaseMetadata, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err != nil {
		o.eventLog = append(o.eventLog, "error:"+meta.ID+":"+err.Error())
	} else {
		o.eventLog = append(o.eventLog, "complete:"+meta.ID)
	}
	o.completed++
	if o.completed >= o.target && o.done != nil {
		close(o.done)
		o.done = nil
	}
}

func (o *recordingObserver) wait(t *testing.T, timeout time.Duration) {
	t.Helper()
	if o.target == 0 {
		return
	}
	select {
	case <-o.done:
	case <-time.After(timeout):
		t.Fatalf("timeout waiting for events: %v", o.events())
	}
}

func (o *recordingObserver) events() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]string, len(o.eventLog))
	copy(out, o.eventLog)
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
