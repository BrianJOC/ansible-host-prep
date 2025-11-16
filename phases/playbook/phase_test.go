package playbook

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/BrianJOC/ansible-host-prep/phases"
	"github.com/BrianJOC/ansible-host-prep/phases/ansibleuser"
	"github.com/BrianJOC/ansible-host-prep/phases/sshconnect"
	ansiblepb "github.com/BrianJOC/ansible-host-prep/utils/ansibleplaybook"
	"github.com/BrianJOC/ansible-host-prep/utils/sshkeypair"
	"github.com/BrianJOC/ansible-host-prep/utils/systemuser"
)

func TestRunUsesContextValues(t *testing.T) {
	t.Parallel()

	ctx := phases.NewContext()
	ctx.Set(sshconnect.ContextKeyTargetHost, "10.0.0.5")
	ctx.Set(sshconnect.ContextKeyTargetUser, "ubuntu")
	ctx.Set(ansibleuser.ContextKeyKeyInfo, &sshkeypair.KeyPairInfo{PrivatePath: "/home/ubuntu/.ssh/id_ansible"})
	ctx.Set(ansibleuser.ContextKeyUserResult, &systemuser.Result{Username: "ansible"})

	runCalled := false
	phase := New(Config{PlaybookPath: "/tmp/site.yml"}).WithRunner(func(ctx context.Context, req ansiblepb.RunRequest, opts ...ansiblepb.Option) error {
		runCalled = true
		require.Equal(t, ansiblepb.RunRequest{
			User:           "ansible",
			Target:         "10.0.0.5",
			PlaybookPath:   "/tmp/site.yml",
			PrivateKeyPath: "/home/ubuntu/.ssh/id_ansible",
		}, req)
		require.Empty(t, opts)
		return nil
	})

	err := phase.Run(context.Background(), ctx)
	require.NoError(t, err)
	require.True(t, runCalled)

	targetVal, ok := ctx.Get(ContextKeyTargetHost)
	require.True(t, ok)
	require.Equal(t, "10.0.0.5", targetVal)

	userVal, ok := ctx.Get(ContextKeyAnsibleUser)
	require.True(t, ok)
	require.Equal(t, "ansible", userVal)

	keyVal, ok := ctx.Get(ContextKeyPrivateKeyPath)
	require.True(t, ok)
	require.Equal(t, "/home/ubuntu/.ssh/id_ansible", keyVal)
}

func TestRunUsesInputsWhenContextMissing(t *testing.T) {
	t.Parallel()

	ctx := phases.NewContext()
	phase := New(Config{}).WithRunner(func(ctx context.Context, req ansiblepb.RunRequest, opts ...ansiblepb.Option) error {
		require.Equal(t, ansiblepb.RunRequest{
			User:           "ansible",
			Target:         "10.0.0.10",
			PlaybookPath:   "/tmp/site.yml",
			PrivateKeyPath: "/tmp/id_ansible",
		}, req)
		require.Empty(t, opts)
		return nil
	})

	phaseID := phase.Metadata().ID
	phases.SetInput(ctx, phaseID, InputTargetHost, " 10.0.0.10 ")
	phases.SetInput(ctx, phaseID, InputAnsibleUser, " ansible ")
	phases.SetInput(ctx, phaseID, InputPrivateKeyPath, " /tmp/id_ansible ")
	phases.SetInput(ctx, phaseID, InputPlaybookPath, "/tmp/site.yml")

	err := phase.Run(context.Background(), ctx)
	require.NoError(t, err)
}

func TestRunRequestsMissingInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		config        Config
		setup         func(*phases.Context, phases.PhaseMetadata)
		expectedInput string
	}{
		{
			name:   "missing target",
			config: Config{PlaybookPath: "/tmp/site.yml"},
			setup: func(ctx *phases.Context, meta phases.PhaseMetadata) {
				phases.SetInput(ctx, meta.ID, InputAnsibleUser, "ansible")
				phases.SetInput(ctx, meta.ID, InputPrivateKeyPath, "/tmp/id_ansible")
			},
			expectedInput: InputTargetHost,
		},
		{
			name:   "missing user",
			config: Config{PlaybookPath: "/tmp/site.yml"},
			setup: func(ctx *phases.Context, meta phases.PhaseMetadata) {
				phases.SetInput(ctx, meta.ID, InputTargetHost, "10.0.0.5")
				phases.SetInput(ctx, meta.ID, InputPrivateKeyPath, "/tmp/id_ansible")
			},
			expectedInput: InputAnsibleUser,
		},
		{
			name:   "missing key path",
			config: Config{PlaybookPath: "/tmp/site.yml"},
			setup: func(ctx *phases.Context, meta phases.PhaseMetadata) {
				phases.SetInput(ctx, meta.ID, InputTargetHost, "10.0.0.5")
				phases.SetInput(ctx, meta.ID, InputAnsibleUser, "ansible")
			},
			expectedInput: InputPrivateKeyPath,
		},
		{
			name:   "missing playbook path",
			config: Config{},
			setup: func(ctx *phases.Context, meta phases.PhaseMetadata) {
				phases.SetInput(ctx, meta.ID, InputTargetHost, "10.0.0.5")
				phases.SetInput(ctx, meta.ID, InputAnsibleUser, "ansible")
				phases.SetInput(ctx, meta.ID, InputPrivateKeyPath, "/tmp/id_ansible")
			},
			expectedInput: InputPlaybookPath,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := phases.NewContext()
			phase := New(tt.config).WithRunner(func(ctx context.Context, req ansiblepb.RunRequest, opts ...ansiblepb.Option) error {
				t.Fatalf("runner should not be called when input is missing")
				return nil
			})
			meta := phase.Metadata()
			if tt.setup != nil {
				tt.setup(ctx, meta)
			}

			err := phase.Run(context.Background(), ctx)

			require.Error(t, err)
			var inputErr phases.InputRequestError
			require.ErrorAs(t, err, &inputErr)
			require.Equal(t, meta.ID, inputErr.PhaseID)
			require.Equal(t, tt.expectedInput, inputErr.Input.ID)
		})
	}
}

func TestRunAppliesOptions(t *testing.T) {
	t.Parallel()

	expectedOpt := ansiblepb.WithBinary("/usr/bin/true")
	ctx := phases.NewContext()
	ctx.Set(sshconnect.ContextKeyTargetHost, "10.0.0.5")
	ctx.Set(sshconnect.ContextKeyTargetUser, "ansible")
	ctx.Set(ansibleuser.ContextKeyKeyInfo, &sshkeypair.KeyPairInfo{PrivatePath: "/tmp/id_ansible"})

	phase := New(Config{PlaybookPath: "/tmp/site.yml"}).
		WithOptions(expectedOpt).
		WithRunner(func(ctx context.Context, req ansiblepb.RunRequest, opts ...ansiblepb.Option) error {
			require.Len(t, opts, 1)
			require.Equal(t, reflect.ValueOf(expectedOpt).Pointer(), reflect.ValueOf(opts[0]).Pointer())
			return nil
		})

	err := phase.Run(context.Background(), ctx)
	require.NoError(t, err)
}
