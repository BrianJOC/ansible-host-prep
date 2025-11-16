package ansibleplaybook

import (
	"bytes"
	"context"
	"testing"

	"github.com/apenella/go-ansible/pkg/execute"
	"github.com/apenella/go-ansible/pkg/options"
	"github.com/stretchr/testify/require"
)

func TestBuildCommandValidation(t *testing.T) {
	t.Parallel()

	_, err := BuildCommand(RunRequest{})
	require.Error(t, err)

	var valErr ValidationError
	require.ErrorAs(t, err, &valErr)
	require.Equal(t, "user", valErr.Field)
}

func TestBuildCommandPopulatesCommand(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	cmd, err := BuildCommand(
		RunRequest{
			User:           " ansible ",
			Target:         "10.0.0.5 ",
			PlaybookPath:   "/tmp/site.yml ",
			PrivateKeyPath: "/tmp/id_ansible ",
		},
		WithStdout(stdout),
		WithStderr(stderr),
		WithEnvVar("ANSIBLE_STDOUT_CALLBACK", "json"),
	)
	require.NoError(t, err)

	require.Equal(t, []string{"/tmp/site.yml"}, cmd.Playbooks)
	require.Equal(t, "10.0.0.5,", cmd.Options.Inventory)
	require.Equal(t, "10.0.0.5", cmd.Options.Limit)

	require.Equal(t, "ansible", cmd.ConnectionOptions.User)
	require.Equal(t, "/tmp/id_ansible", cmd.ConnectionOptions.PrivateKey)

	require.True(t, cmd.PrivilegeEscalationOptions.Become)
	require.Equal(t, becomeMethod, cmd.PrivilegeEscalationOptions.BecomeMethod)
	require.Equal(t, becomeUser, cmd.PrivilegeEscalationOptions.BecomeUser)

	exec, ok := cmd.Exec.(*execute.DefaultExecute)
	require.True(t, ok)
	require.Equal(t, stdout, exec.Write)
	require.Equal(t, stderr, exec.WriterError)
	require.Equal(t, "false", exec.EnvVars[options.AnsibleHostKeyCheckingEnv])
	require.Equal(t, "json", exec.EnvVars["ANSIBLE_STDOUT_CALLBACK"])
}

func TestRunWithCustomBinary(t *testing.T) {
	t.Parallel()

	req := RunRequest{
		User:           "ansible",
		Target:         "10.0.0.5",
		PlaybookPath:   "site.yml",
		PrivateKeyPath: "/tmp/id_ansible",
	}

	err := Run(context.Background(), req, WithBinary("/usr/bin/true"))
	require.NoError(t, err)
}
