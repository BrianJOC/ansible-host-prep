package sshkeypair

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnsureKeyPairCreatesNewPair(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	private := filepath.Join(dir, "id_test")

	info, err := EnsureKeyPair(private, WithComment("test@example.com"))
	require.NoError(t, err)
	require.True(t, info.KeyGenerated)
	require.True(t, info.PublicCreated)

	privBytes, err := os.ReadFile(info.PrivatePath)
	require.NoError(t, err)
	require.Contains(t, string(privBytes), "BEGIN RSA PRIVATE KEY")

	pubBytes, err := os.ReadFile(info.PublicPath)
	require.NoError(t, err)
	require.Contains(t, string(pubBytes), "test@example.com")
}

func TestEnsureKeyPairReusesExisting(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	private := filepath.Join(dir, "id_reuse")

	first, err := EnsureKeyPair(private)
	require.NoError(t, err)
	info, err := EnsureKeyPair(private)
	require.NoError(t, err)
	require.False(t, info.KeyGenerated)
	require.Equal(t, first.PrivatePath, info.PrivatePath)
}

func TestEnsureKeyPairRegeneratesMissingPublic(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	private := filepath.Join(dir, "id_missing_pub")

	_, err := EnsureKeyPair(private)
	require.NoError(t, err)

	require.NoError(t, os.Remove(private+".pub"))

	info, err := EnsureKeyPair(private)
	require.NoError(t, err)
	require.False(t, info.KeyGenerated)
	require.True(t, info.PublicCreated)
}

func TestEnsureKeyPairValidatesInput(t *testing.T) {
	t.Parallel()

	_, err := EnsureKeyPair("")
	require.Error(t, err)
	require.IsType(t, PathError{}, err)

	_, err = EnsureKeyPair("some", WithKeyBits(1024))
	require.Error(t, err)
	require.IsType(t, OptionError{}, err)
}
