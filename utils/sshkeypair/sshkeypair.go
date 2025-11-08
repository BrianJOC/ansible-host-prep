package sshkeypair

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

const (
	defaultBits    = 4096
	minKeyBits     = 2048
	defaultComment = "prep-for-ansible"
)

// KeyPairInfo describes the ensured key pair.
type KeyPairInfo struct {
	PrivatePath   string
	PublicPath    string
	KeyGenerated  bool
	PublicCreated bool
}

// Option configures EnsureKeyPair behavior.
type Option func(*ensureOptions) error

type ensureOptions struct {
	bits    int
	comment string
	mode    os.FileMode
}

// WithKeyBits overrides the RSA key size.
func WithKeyBits(bits int) Option {
	return func(opts *ensureOptions) error {
		if bits < minKeyBits {
			return OptionError{Reason: fmt.Sprintf("bits must be >= %d", minKeyBits)}
		}
		opts.bits = bits
		return nil
	}
}

// WithComment overrides the comment appended to the public key line.
func WithComment(comment string) Option {
	return func(opts *ensureOptions) error {
		comment = strings.TrimSpace(comment)
		if comment == "" {
			return OptionError{Reason: "comment must not be empty"}
		}
		opts.comment = comment
		return nil
	}
}

// EnsureKeyPair checks for an RSA SSH key pair and creates it when missing.
func EnsureKeyPair(privatePath string, opts ...Option) (*KeyPairInfo, error) {
	privatePath = strings.TrimSpace(privatePath)
	if privatePath == "" {
		return nil, PathError{Reason: "private key path is required"}
	}

	pubPath := privatePath + ".pub"
	cfg := ensureOptions{
		bits:    defaultBits,
		comment: defaultComment,
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}

	info := &KeyPairInfo{
		PrivatePath: privatePath,
		PublicPath:  pubPath,
	}

	privExists, err := fileExists(privatePath)
	if err != nil {
		return nil, FileStatError{Path: privatePath, Err: err}
	}

	pubExists, err := fileExists(pubPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, FileStatError{Path: pubPath, Err: err}
	}

	if privExists {
		privKey, err := readPrivateKey(privatePath)
		if err != nil {
			return nil, err
		}

		if !pubExists {
			if err := writePublicKey(pubPath, privKey, cfg.comment); err != nil {
				return nil, err
			}
			info.PublicCreated = true
		}

		return info, nil
	}

	if err := generateAndWritePair(privatePath, pubPath, cfg.bits, cfg.comment); err != nil {
		return nil, err
	}

	info.KeyGenerated = true
	info.PublicCreated = true

	return info, nil
}

func generateAndWritePair(privatePath, publicPath string, bits int, comment string) error {
	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return KeyGenerateError{Err: err}
	}

	if err := writePrivateKey(privatePath, key); err != nil {
		return err
	}

	if err := writePublicKey(publicPath, key, comment); err != nil {
		return err
	}

	return nil
}

func writePrivateKey(path string, key *rsa.PrivateKey) error {
	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}

	var buf bytes.Buffer
	if err := pem.Encode(&buf, block); err != nil {
		return KeyWriteError{Path: path, Err: err}
	}

	if err := ensureDir(path); err != nil {
		return err
	}

	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		return KeyWriteError{Path: path, Err: err}
	}

	return nil
}

func writePublicKey(path string, key *rsa.PrivateKey, comment string) error {
	pub, err := ssh.NewPublicKey(&key.PublicKey)
	if err != nil {
		return KeyWriteError{Path: path, Err: err}
	}

	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub)))
	if comment != "" {
		line = fmt.Sprintf("%s %s", line, comment)
	}
	line += "\n"

	if err := ensureDir(path); err != nil {
		return err
	}

	if err := os.WriteFile(path, []byte(line), 0o644); err != nil {
		return KeyWriteError{Path: path, Err: err}
	}

	return nil
}

func readPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, KeyReadError{Path: path, Err: err}
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, KeyParseError{Path: path, Err: fmt.Errorf("missing PEM block")}
	}

	var parsed any
	switch block.Type {
	case "RSA PRIVATE KEY":
		parsed, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	default:
		parsed, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	}
	if err != nil {
		return nil, KeyParseError{Path: path, Err: err}
	}

	rsaKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, KeyParseError{Path: path, Err: fmt.Errorf("unsupported private key type %T", parsed)}
	}

	return rsaKey, nil
}

func ensureDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return KeyWriteError{Path: path, Err: err}
	}
	return nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
