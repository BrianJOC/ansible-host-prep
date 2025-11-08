package sshconnect

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/BrianJOC/ansible-host-prep/phases"
	"github.com/BrianJOC/ansible-host-prep/utils/sshconnection"
)

func TestPhaseEstablishesConnectionWithPassword(t *testing.T) {
	t.Parallel()

	expectedClient := &ssh.Client{}
	var capturedCred sshconnection.Credential

	phase := New().WithConnector(func(host string, port int, username string, cred sshconnection.Credential, _ ...sshconnection.Option) (*ssh.Client, error) {
		require.Equal(t, "example.com", host)
		require.Equal(t, 2222, port)
		require.Equal(t, "deploy", username)
		capturedCred = cred
		return expectedClient, nil
	})

	ctx := phases.NewContext()
	setInputs(ctx, map[string]string{
		InputHost:       "example.com",
		InputPort:       "2222",
		InputUsername:   "deploy",
		InputAuthMethod: authMethodPassword,
		InputPassword:   "secret",
	})

	err := phase.Run(context.Background(), ctx)
	require.NoError(t, err)
	require.Equal(t, "secret", capturedCred.Password)

	clientVal, ok := ctx.Get(ContextKeySSHClient)
	require.True(t, ok)
	require.Equal(t, expectedClient, clientVal)

	passwordVal, ok := ctx.Get(ContextKeySSHPassword)
	require.True(t, ok)
	require.Equal(t, "secret", passwordVal)
}

func TestPhaseHandlesKeyAuth(t *testing.T) {
	t.Parallel()

	var capturedCred sshconnection.Credential
	phase := New().WithConnector(func(host string, port int, username string, cred sshconnection.Credential, _ ...sshconnection.Option) (*ssh.Client, error) {
		capturedCred = cred
		return &ssh.Client{}, nil
	})

	ctx := phases.NewContext()
	setInputs(ctx, map[string]string{
		InputHost:       "example.com",
		InputUsername:   "deploy",
		InputAuthMethod: authMethodKeyPath,
		InputKeyPath:    "/tmp/id_rsa",
	})

	err := phase.Run(context.Background(), ctx)
	require.NoError(t, err)
	require.Equal(t, "/tmp/id_rsa", capturedCred.KeyPath)

	_, passwordStored := ctx.Get(ContextKeySSHPassword)
	require.False(t, passwordStored)
}

func TestPhaseValidationError(t *testing.T) {
	t.Parallel()

	phase := New()
	ctx := phases.NewContext()
	setInputs(ctx, map[string]string{
		InputUsername:   "deploy",
		InputAuthMethod: authMethodPassword,
		InputPassword:   "secret",
	})

	err := phase.Run(context.Background(), ctx)
	require.Error(t, err)
	var inputErr phases.InputRequestError
	require.ErrorAs(t, err, &inputErr)
	require.Equal(t, InputHost, inputErr.Input.ID)
}

func TestPhasePropagatesConnectorError(t *testing.T) {
	t.Parallel()

	phase := New().WithConnector(func(string, int, string, sshconnection.Credential, ...sshconnection.Option) (*ssh.Client, error) {
		return nil, errors.New("connect failed")
	})

	ctx := phases.NewContext()
	setInputs(ctx, map[string]string{
		InputHost:       "example.com",
		InputUsername:   "deploy",
		InputAuthMethod: authMethodPassword,
		InputPassword:   "secret",
	})

	err := phase.Run(context.Background(), ctx)
	require.EqualError(t, err, "connect failed")
}

func TestPhaseInvalidPortRequestsInput(t *testing.T) {
	t.Parallel()

	phase := New()
	ctx := phases.NewContext()
	setInputs(ctx, map[string]string{
		InputHost:       "example.com",
		InputUsername:   "deploy",
		InputAuthMethod: authMethodPassword,
		InputPassword:   "secret",
		InputPort:       "abc",
	})

	err := phase.Run(context.Background(), ctx)
	require.Error(t, err)
	var inputErr phases.InputRequestError
	require.ErrorAs(t, err, &inputErr)
	require.Equal(t, InputPort, inputErr.Input.ID)
}

func setInputs(ctx *phases.Context, values map[string]string) {
	for id, value := range values {
		phases.SetInput(ctx, phaseID, id, value)
	}
}
