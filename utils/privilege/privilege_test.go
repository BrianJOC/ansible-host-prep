package privilege

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnsureElevationPrefersSudo(t *testing.T) {
	t.Parallel()

	r := &fakeRunner{
		responses: []fakeResponse{
			{match: "sudo -S", err: nil},
			{match: "sudo -S", err: nil},
		},
	}

	method, err := ensureElevation(r, "password")
	require.NoError(t, err)
	require.Equal(t, methodSudo, method)
}

func TestEnsureElevationFallsBackToSuOnPermissionError(t *testing.T) {
	t.Parallel()

	permissionErr := errors.New("exit status 1")
	r := &fakeRunner{
		responses: []fakeResponse{
			{match: "sudo -S", stderr: "user is not in the sudoers file", err: permissionErr},
			{match: "su - root -c", err: nil},
			{match: "su - root -c", err: nil},
		},
	}

	method, err := ensureElevation(r, "password")
	require.NoError(t, err)
	require.Equal(t, methodSu, method)
}

func TestEnsureElevationInstallsSudoWhenMissing(t *testing.T) {
	t.Parallel()

	missingErr := errors.New("exit status 127")
	r := &fakeRunner{
		responses: []fakeResponse{
			{match: "sudo -S", stderr: "sudo: command not found", err: missingErr},
			{match: "su - root -c \"true\"", err: nil},
			{match: "su - root -c", err: nil},
			{match: "sudo -S", err: nil},
			{match: "sudo -S", err: nil},
		},
	}

	method, err := ensureElevation(r, "password")
	require.NoError(t, err)
	require.Equal(t, methodSudo, method)
}

func TestEnsureElevatedClientValidatesInputs(t *testing.T) {
	t.Parallel()

	_, err := EnsureElevatedClient(nil, Password{Value: "secret"})
	require.Error(t, err)
	require.IsType(t, NilClientError{}, err)

	_, err = Password{}.validate()
	require.Error(t, err)
	require.IsType(t, PasswordError{}, err)
}

type fakeRunner struct {
	responses []fakeResponse
}

type fakeResponse struct {
	match  string
	stdout string
	stderr string
	err    error
}

func (f *fakeRunner) Run(cmd string, stdin string) (string, string, error) {
	if len(f.responses) == 0 {
		return "", "", fmt.Errorf("unexpected command: %s", cmd)
	}

	resp := f.responses[0]
	f.responses = f.responses[1:]

	if resp.match != "" && !strings.Contains(cmd, resp.match) {
		return "", "", fmt.Errorf("unexpected command %q; expected substring %q", cmd, resp.match)
	}

	return resp.stdout, resp.stderr, resp.err
}
