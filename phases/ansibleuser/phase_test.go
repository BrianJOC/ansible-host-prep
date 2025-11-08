package ansibleuser

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/BrianJOC/ansible-host-prep/phases"
	"github.com/BrianJOC/ansible-host-prep/phases/sudoensure"
	"github.com/BrianJOC/ansible-host-prep/utils/privilege"
	"github.com/BrianJOC/ansible-host-prep/utils/sshkeypair"
	"github.com/BrianJOC/ansible-host-prep/utils/systemuser"
)

func TestPhaseCreatesUserWithKey(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	privatePath := filepath.Join(tempDir, "id_ansible")
	publicPath := privatePath + ".pub"
	require.NoError(t, os.WriteFile(publicPath, []byte("ssh-rsa AAA ansible\n"), 0o600))

	phase := New().
		WithKeyPairEnsurer(func(path string, opts ...sshkeypair.Option) (*sshkeypair.KeyPairInfo, error) {
			require.Equal(t, privatePath, path)
			return &sshkeypair.KeyPairInfo{
				PrivatePath: privatePath,
				PublicPath:  publicPath,
			}, nil
		}).
		WithUserEnsurer(func(r systemuser.Runner, username string, publicKey string, opts ...systemuser.Option) (*systemuser.Result, error) {
			require.Equal(t, defaultUsername, username)
			require.Contains(t, publicKey, "ssh-rsa AAA")
			return &systemuser.Result{
				Username:               username,
				UserCreated:            true,
				PasswordlessConfigured: true,
			}, nil
		})

	ctx := phases.NewContext()
	ctx.Set(sudoensure.ContextKeyElevatedClient, &privilege.ElevatedClient{})
	phases.SetInput(ctx, phaseID, InputKeyPath, privatePath)

	err := phase.Run(context.Background(), ctx)
	require.NoError(t, err)

	val, ok := ctx.Get(ContextKeyUserResult)
	require.True(t, ok)
	result := val.(*systemuser.Result)
	require.True(t, result.UserCreated)
}

func TestPhaseRequestsKeyPath(t *testing.T) {
	t.Parallel()

	phase := New()
	ctx := phases.NewContext()
	ctx.Set(sudoensure.ContextKeyElevatedClient, &privilege.ElevatedClient{})

	err := phase.Run(context.Background(), ctx)
	require.Error(t, err)
	var inputErr phases.InputRequestError
	require.ErrorAs(t, err, &inputErr)
	require.Equal(t, phaseID, inputErr.PhaseID)
}

func TestPhaseRequiresElevatedClient(t *testing.T) {
	t.Parallel()

	phase := New()
	ctx := phases.NewContext()
	phases.SetInput(ctx, phaseID, InputKeyPath, "/tmp/test")

	err := phase.Run(context.Background(), ctx)
	require.Error(t, err)
	var valErr phases.ValidationError
	require.ErrorAs(t, err, &valErr)
}
