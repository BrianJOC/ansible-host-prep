package systemuser

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnsureUserCreatesAndConfigures(t *testing.T) {
	t.Parallel()

	r := &fakeRunner{
		responses: []fakeResponse{
			{match: "id -u", err: errors.New("exit status 1"), stderr: "no such user"},
			{match: "useradd -m", err: nil},
			{match: "install -o", err: nil},
			{match: "usermod -aG", err: nil},
			{match: "sudoers.d", err: nil},
		},
	}

	res, err := EnsureUser(r, "deploy", "ssh-rsa AAA...", WithSudoAccess(), WithPasswordlessSudo())
	require.NoError(t, err)
	require.True(t, res.UserCreated)
	require.True(t, res.AuthorizedKeyUpdated)
	require.True(t, res.AddedToSudo)
	require.True(t, res.PasswordlessConfigured)
	require.Equal(t, "/home/deploy", res.HomeDir)
}

func TestEnsureUserSkipsExistingUser(t *testing.T) {
	t.Parallel()

	r := &fakeRunner{
		responses: []fakeResponse{
			{match: "id -u", err: nil},
			{match: "install -o", err: nil},
		},
	}

	res, err := EnsureUser(r, "deploy", "ssh-rsa AAA...")
	require.NoError(t, err)
	require.False(t, res.UserCreated)
	require.True(t, res.AuthorizedKeyUpdated)
}

func TestEnsureUserValidation(t *testing.T) {
	t.Parallel()

	_, err := EnsureUser(nil, "deploy", "ssh-rsa AAA")
	require.Error(t, err)
	require.IsType(t, RunnerError{}, err)

	r := &fakeRunner{}
	_, err = EnsureUser(r, "", "ssh-rsa AAA")
	require.Error(t, err)
	require.IsType(t, ValidationError{}, err)

	_, err = EnsureUser(r, "deploy", "")
	require.Error(t, err)
	require.IsType(t, ValidationError{}, err)

	_, err = EnsureUser(r, "deploy user", "ssh-rsa AAA")
	require.Error(t, err)
	require.IsType(t, ValidationError{}, err)

	_, err = EnsureUser(r, "deploy", "ssh-rsa AAA", WithShell(""))
	require.Error(t, err)
	require.IsType(t, OptionError{}, err)
}

func TestEnsureUserPropagatesCommandErrors(t *testing.T) {
	t.Parallel()

	r := &fakeRunner{
		responses: []fakeResponse{
			{match: "id -u", err: errors.New("exit status 1"), stderr: "no such user"},
			{match: "useradd -m", err: errors.New("exit status 2"), stderr: "useradd failed"},
		},
	}

	_, err := EnsureUser(r, "deploy", "ssh-rsa AAA")
	require.Error(t, err)
	require.IsType(t, CommandError{}, err)
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

func (f *fakeRunner) Run(cmd string) (string, string, error) {
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
