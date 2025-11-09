package phasedapp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	phasespkg "github.com/BrianJOC/ansible-host-prep/phases"
)

func TestInputHelpers(t *testing.T) {
	t.Parallel()

	text := TextInput("host", "Host", Required(), WithDescription("fqdn"), WithDefault("srv"))
	require.Equal(t, phasespkg.InputKindText, text.Kind)
	require.True(t, text.Required)
	require.Equal(t, "fqdn", text.Description)
	require.Equal(t, "srv", text.Default)

	secret := SecretInput("pwd", "Password", Required())
	require.Equal(t, phasespkg.InputKindSecret, secret.Kind)
	require.True(t, secret.Secret)

	options := []phasespkg.InputOption{{Value: "a", Label: "Option A"}}
	sel := SelectInput("opt", "Option", options)
	require.Equal(t, phasespkg.InputKindSelect, sel.Kind)
	require.Len(t, sel.Options, 1)
}

func TestSimplePhase(t *testing.T) {
	t.Parallel()

	meta := phasespkg.PhaseMetadata{ID: "demo", Title: "Demo"}
	phase := NewPhase(meta, func(ctx context.Context, pc *phasespkg.Context) error {
		SetContext(pc, Namespace("demo", "hit"), true)
		return nil
	})

	ctx := phasespkg.NewContext()
	require.NoError(t, phase.Run(context.Background(), ctx))

	val, ok := GetContext[bool](ctx, Namespace("demo", "hit"))
	require.True(t, ok)
	require.True(t, val)
}

func TestBuilderDetectsDuplicates(t *testing.T) {
	t.Parallel()

	builder := NewBuilder().
		AddPhase(SimplePhase{meta: phasespkg.PhaseMetadata{ID: "one"}}).
		AddPhase(SimplePhase{meta: phasespkg.PhaseMetadata{ID: "one"}})

	_, err := builder.Build()
	require.Error(t, err)
}

func TestSelectPhasesByTag(t *testing.T) {
	t.Parallel()

	first := SimplePhase{meta: phasespkg.PhaseMetadata{ID: "one", Tags: []string{"ansible"}}}
	second := SimplePhase{meta: phasespkg.PhaseMetadata{ID: "two", Tags: []string{"k8s"}}}

	selected := SelectPhases([]phasespkg.Phase{first, second}, WithTag("Ansible"))
	require.Len(t, selected, 1)
	require.Equal(t, "one", selected[0].Metadata().ID)
}

func TestContextHelpers(t *testing.T) {
	t.Parallel()

	ctx := phasespkg.NewContext()
	key := Namespace("ssh", "client")
	SetContext(ctx, key, "client-value")

	val, ok := GetContext[string](ctx, key)
	require.True(t, ok)
	require.Equal(t, "client-value", val)
}
