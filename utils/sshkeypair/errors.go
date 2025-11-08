package sshkeypair

import "fmt"

// PathError represents invalid input paths.
type PathError struct {
	Reason string
}

func (e PathError) Error() string {
	return fmt.Sprintf("ssh key path error: %s", e.Reason)
}

// FileStatError wraps os.Stat failures.
type FileStatError struct {
	Path string
	Err  error
}

func (e FileStatError) Error() string {
	return fmt.Sprintf("stat %s failed: %v", e.Path, e.Err)
}

func (e FileStatError) Unwrap() error {
	return e.Err
}

// KeyReadError indicates failure to load a key from disk.
type KeyReadError struct {
	Path string
	Err  error
}

func (e KeyReadError) Error() string {
	return fmt.Sprintf("read key %s failed: %v", e.Path, e.Err)
}

func (e KeyReadError) Unwrap() error {
	return e.Err
}

// KeyWriteError wraps write/permission problems when writing keys.
type KeyWriteError struct {
	Path string
	Err  error
}

func (e KeyWriteError) Error() string {
	return fmt.Sprintf("write key %s failed: %v", e.Path, e.Err)
}

func (e KeyWriteError) Unwrap() error {
	return e.Err
}

// KeyParseError indicates unsupported or corrupt key data.
type KeyParseError struct {
	Path string
	Err  error
}

func (e KeyParseError) Error() string {
	return fmt.Sprintf("parse key %s failed: %v", e.Path, e.Err)
}

func (e KeyParseError) Unwrap() error {
	return e.Err
}

// KeyGenerateError wraps RSA key generation failures.
type KeyGenerateError struct {
	Err error
}

func (e KeyGenerateError) Error() string {
	return fmt.Sprintf("generate RSA key failed: %v", e.Err)
}

func (e KeyGenerateError) Unwrap() error {
	return e.Err
}

// OptionError surfaces invalid option configurations.
type OptionError struct {
	Reason string
}

func (e OptionError) Error() string {
	return fmt.Sprintf("option error: %s", e.Reason)
}
