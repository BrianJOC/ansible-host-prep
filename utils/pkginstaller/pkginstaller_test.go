package pkginstaller

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnsureSkipsWhenPackageExists(t *testing.T) {
	t.Parallel()

	r := &fakeRunner{
		responses: []fakeResponse{
			{match: "command -v", err: nil},
		},
	}

	result, err := Ensure(r, "python3")
	require.NoError(t, err)
	require.True(t, result.Skipped)
	require.False(t, result.Installed)
}

func TestEnsureInstallsWhenMissing(t *testing.T) {
	t.Parallel()

	r := &fakeRunner{
		responses: []fakeResponse{
			{match: "command -v", err: errors.New("exit status 1")},
			{match: "apt-get update", err: nil},
		},
	}

	result, err := Ensure(r, "python3")
	require.NoError(t, err)
	require.True(t, result.Installed)
}

func TestEnsureValidatesInputs(t *testing.T) {
	t.Parallel()

	_, err := Ensure(nil, "python3")
	require.Error(t, err)
	require.IsType(t, RunnerError{}, err)

	r := &fakeRunner{}
	_, err = Ensure(r, "")
	require.Error(t, err)
	require.IsType(t, ValidationError{}, err)

	_, err = Ensure(r, "python3", WithCustomCheck(""))
	require.Error(t, err)
	require.IsType(t, OptionError{}, err)
}

func TestEnsurePropagatesInstallErrors(t *testing.T) {
	t.Parallel()

	r := &fakeRunner{
		responses: []fakeResponse{
			{match: "command -v", err: errors.New("exit status 1")},
			{match: "apt-get update", err: errors.New("exit status 100"), stderr: "install failed"},
		},
	}

	_, err := Ensure(r, "python3")
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
