package phasedapp

import (
	"context"
	"fmt"
	"strings"

	"github.com/BrianJOC/ansible-host-prep/phases"
)

// PhaseFunc represents the work performed by a SimplePhase.
type PhaseFunc func(ctx context.Context, phaseCtx *phases.Context) error

// SimplePhase lets callers define phases with a metadata struct and a function
// instead of declaring a custom type for every phase.
type SimplePhase struct {
	meta phases.PhaseMetadata
	run  PhaseFunc
}

// NewPhase constructs a SimplePhase, panicking if metadata is missing an ID or
// the provided function is nil.
func NewPhase(meta phases.PhaseMetadata, run PhaseFunc) phases.Phase {
	if meta.ID == "" {
		panic("phasedapp: simple phase metadata must include an ID")
	}
	if run == nil {
		panic("phasedapp: simple phase requires a run function")
	}
	return SimplePhase{meta: meta, run: run}
}

// Metadata returns the phase definition supplied at construction time.
func (p SimplePhase) Metadata() phases.PhaseMetadata {
	return p.meta
}

// Run calls the PhaseFunc provided to NewPhase.
func (p SimplePhase) Run(ctx context.Context, phaseCtx *phases.Context) error {
	return p.run(ctx, phaseCtx)
}

// Builder helps compose ordered phase lists with duplicate detection.
type Builder struct {
	phases []phases.Phase
	seen   map[string]struct{}
	err    error
}

// NewBuilder constructs an empty Builder.
func NewBuilder() *Builder {
	return &Builder{
		seen: make(map[string]struct{}),
	}
}

// AddPhase appends a phase, capturing duplicate/validation errors.
func (b *Builder) AddPhase(phase phases.Phase) *Builder {
	if b == nil || phase == nil || b.err != nil {
		return b
	}
	meta := phase.Metadata()
	if meta.ID == "" {
		b.err = phases.ValidationError{Reason: "phase id must not be empty"}
		return b
	}
	if _, exists := b.seen[meta.ID]; exists {
		b.err = phases.DuplicatePhaseError{ID: meta.ID}
		return b
	}
	b.seen[meta.ID] = struct{}{}
	b.phases = append(b.phases, phase)
	return b
}

// AddPhases appends multiple phases, stopping early on error.
func (b *Builder) AddPhases(phases ...phases.Phase) *Builder {
	for _, ph := range phases {
		b.AddPhase(ph)
	}
	return b
}

// Build returns the accumulated phase slice or any captured error.
func (b *Builder) Build() ([]phases.Phase, error) {
	if b == nil {
		return nil, nil
	}
	if b.err != nil {
		return nil, b.err
	}
	out := make([]phases.Phase, len(b.phases))
	copy(out, b.phases)
	return out, nil
}

// PhaseFilter matches phases based on metadata properties.
type PhaseFilter func(phases.PhaseMetadata) bool

// WithTag matches phases containing the provided tag (case-insensitive).
func WithTag(tag string) PhaseFilter {
	tag = strings.ToLower(tag)
	return func(meta phases.PhaseMetadata) bool {
		for _, t := range meta.Tags {
			if strings.ToLower(t) == tag {
				return true
			}
		}
		return false
	}
}

// SelectPhases returns phases that satisfy every provided filter. When no
// filters are supplied, all phases are returned.
func SelectPhases(list []phases.Phase, filters ...PhaseFilter) []phases.Phase {
	if len(list) == 0 {
		return nil
	}
	if len(filters) == 0 {
		out := make([]phases.Phase, len(list))
		copy(out, list)
		return out
	}
	var res []phases.Phase
outer:
	for _, ph := range list {
		if ph == nil {
			continue
		}
		meta := ph.Metadata()
		for _, filter := range filters {
			if filter == nil {
				continue
			}
			if !filter(meta) {
				continue outer
			}
		}
		res = append(res, ph)
	}
	return res
}

// WithBundle appends all phases from the provided bundle function.
func WithBundle(bundle func() []phases.Phase) Option {
	return func(cfg *Config) {
		if cfg == nil || bundle == nil {
			return
		}
		cfg.Phases = append(cfg.Phases, bundle()...)
	}
}

// MustBundle builds phases from a bundle constructor, panicking on errors.
func MustBundle(builder func() ([]phases.Phase, error)) []phases.Phase {
	phases, err := builder()
	if err != nil {
		panic(fmt.Sprintf("phasedapp: bundle failed: %v", err))
	}
	return phases
}
