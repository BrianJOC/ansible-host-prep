package pythonensure

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/BrianJOC/prep-for-ansible/phases"
	"github.com/BrianJOC/prep-for-ansible/phases/sudoensure"
	"github.com/BrianJOC/prep-for-ansible/utils/pkginstaller"
	"github.com/BrianJOC/prep-for-ansible/utils/privilege"
)

func TestPhaseEnsuresPython(t *testing.T) {
	t.Parallel()

	var called bool
	phase := New().WithInstaller(func(r pkginstaller.Runner, packageName string, opts ...pkginstaller.Option) (*pkginstaller.Result, error) {
		called = true
		require.Equal(t, defaultPackageName, packageName)
		return &pkginstaller.Result{Installed: true}, nil
	})

	ctx := phases.NewContext()
	ctx.Set(sudoensure.ContextKeyElevatedClient, &privilege.ElevatedClient{})

	err := phase.Run(context.Background(), ctx)
	require.NoError(t, err)
	require.True(t, called)

	val, ok := ctx.Get(ContextKeyInstalled)
	require.True(t, ok)
	require.Equal(t, true, val)
}

func TestPhaseRequiresElevatedClient(t *testing.T) {
	t.Parallel()

	phase := New()
	err := phase.Run(context.Background(), phases.NewContext())
	require.Error(t, err)
	var valErr phases.ValidationError
	require.ErrorAs(t, err, &valErr)
}

func TestPhasePropagatesInstallerError(t *testing.T) {
	t.Parallel()

	phase := New().WithInstaller(func(pkginstaller.Runner, string, ...pkginstaller.Option) (*pkginstaller.Result, error) {
		return nil, errors.New("install failed")
	})

	ctx := phases.NewContext()
	ctx.Set(sudoensure.ContextKeyElevatedClient, &privilege.ElevatedClient{})

	err := phase.Run(context.Background(), ctx)
	require.EqualError(t, err, "install failed")
}
