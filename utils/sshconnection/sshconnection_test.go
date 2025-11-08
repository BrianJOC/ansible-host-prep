package sshconnection

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCredentialAuthMethodValidation(t *testing.T) {
	t.Parallel()

	missingKeyPath := filepath.Join(t.TempDir(), "missing")

	tests := []struct {
		name    string
		cred    Credential
		errType any
	}{
		{
			name: "password",
			cred: Credential{Password: "secret"},
		},
		{
			name:    "missing credentials",
			cred:    Credential{},
			errType: CredentialError{},
		},
		{
			name:    "both password and key",
			cred:    Credential{Password: "secret", KeyPath: "/tmp/key"},
			errType: CredentialError{},
		},
		{
			name:    "missing key file",
			cred:    Credential{KeyPath: missingKeyPath},
			errType: KeyLoadError{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := tt.cred.authMethod()
			if tt.errType == nil {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			require.IsType(t, tt.errType, err)
		})
	}
}

func TestCredentialAuthMethodKeyParseError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "bad.key")
	require.NoError(t, os.WriteFile(keyPath, []byte("not a key"), 0o600))

	cred := Credential{KeyPath: keyPath}
	_, err := cred.authMethod()
	require.Error(t, err)
	require.IsType(t, KeyParseError{}, err)
}

func TestConnectRejectsMissingParameters(t *testing.T) {
	t.Parallel()

	cred := Credential{Password: "secret"}

	_, err := Connect("", 0, "user", cred)
	require.Error(t, err)
	require.IsType(t, InvalidTargetError{}, err)

	_, err = Connect("example.com", 0, "", cred)
	require.Error(t, err)
	require.IsType(t, InvalidTargetError{}, err)
}

func TestConnectOptionValidation(t *testing.T) {
	t.Parallel()

	cred := Credential{Password: "secret"}

	_, err := Connect("example.com", 22, "user", cred, WithTimeout(0))
	require.Error(t, err)
	require.IsType(t, OptionError{}, err)

	// ensure nil options are ignored and timeout override sticks
	connTimeout := time.Second
	opts := []Option{nil, WithTimeout(connTimeout)}

	config := connectOptions{timeout: defaultDialTimeout}
	for _, o := range opts {
		if o == nil {
			continue
		}
		require.NoError(t, o(&config))
	}

	require.Equal(t, connTimeout, config.timeout)
}
