package sudoensure

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/BrianJOC/ansible-host-prep/phases"
	"github.com/BrianJOC/ansible-host-prep/phases/sshconnect"
	"github.com/BrianJOC/ansible-host-prep/utils/privilege"
)

func TestPhaseUsesExistingPassword(t *testing.T) {
	t.Parallel()

	expectedPassword := "secret"
	called := false
	fakeClient := &privilege.ElevatedClient{}

	phase := New().WithEnsurer(func(client *ssh.Client, password privilege.Password) (*privilege.ElevatedClient, error) {
		require.Equal(t, expectedPassword, password.Value)
		called = true
		return fakeClient, nil
	})

	ctx := phases.NewContext()
	ctx.Set(sshconnect.ContextKeySSHClient, &ssh.Client{})
	ctx.Set(sshconnect.ContextKeySSHPassword, expectedPassword)

	err := phase.Run(context.Background(), ctx)
	require.NoError(t, err)
	require.True(t, called)

	val, ok := ctx.Get(ContextKeyElevatedClient)
	require.True(t, ok)
	require.Equal(t, fakeClient, val)
}

func TestPhaseRequestsPasswordWhenMissing(t *testing.T) {
	t.Parallel()

	phase := New()
	ctx := phases.NewContext()
	ctx.Set(sshconnect.ContextKeySSHClient, &ssh.Client{})

	err := phase.Run(context.Background(), ctx)
	require.Error(t, err)
	var inputErr phases.InputRequestError
	require.ErrorAs(t, err, &inputErr)
	require.Equal(t, phaseID, inputErr.PhaseID)
}

func TestPhaseRequestsNewPasswordOnAuthFailure(t *testing.T) {
	t.Parallel()

	phase := New().WithEnsurer(func(client *ssh.Client, password privilege.Password) (*privilege.ElevatedClient, error) {
		return nil, privilege.SudoAuthenticationError{Err: errors.New("bad password")}
	})

	ctx := phases.NewContext()
	ctx.Set(sshconnect.ContextKeySSHClient, &ssh.Client{})
	ctx.Set(sshconnect.ContextKeySSHPassword, "wrong")

	err := phase.Run(context.Background(), ctx)
	require.Error(t, err)
	var inputErr phases.InputRequestError
	require.ErrorAs(t, err, &inputErr)
	require.Equal(t, phaseID, inputErr.PhaseID)
}

func TestPhasePropagatesOtherErrors(t *testing.T) {
	t.Parallel()

	phase := New().WithEnsurer(func(client *ssh.Client, password privilege.Password) (*privilege.ElevatedClient, error) {
		return nil, errors.New("network down")
	})

	ctx := phases.NewContext()
	ctx.Set(sshconnect.ContextKeySSHClient, &ssh.Client{})
	ctx.Set(sshconnect.ContextKeySSHPassword, "secret")

	err := phase.Run(context.Background(), ctx)
	require.EqualError(t, err, "network down")
}
