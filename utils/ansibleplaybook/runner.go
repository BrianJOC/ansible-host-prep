package ansibleplaybook

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/apenella/go-ansible/pkg/execute"
	"github.com/apenella/go-ansible/pkg/options"
	"github.com/apenella/go-ansible/pkg/playbook"
)

const (
	becomeMethod = "sudo"
	becomeUser   = "root"
)

// RunRequest captures the minimum information required to execute a playbook.
type RunRequest struct {
	User           string
	Target         string
	PlaybookPath   string
	PrivateKeyPath string
}

// Option configures how the playbook command is built or executed.
type Option func(*runConfig) error

type runConfig struct {
	stdout          io.Writer
	stderr          io.Writer
	env             map[string]string
	executorFactory func(...execute.ExecuteOptions) execute.Executor
	binary          string
}

// ValidationError indicates an invalid or missing user-supplied value.
type ValidationError struct {
	Field string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("ansibleplaybook: %s is required", e.Field)
}

// WithStdout overrides where ansible stdout is written (default io.Discard).
func WithStdout(w io.Writer) Option {
	return func(cfg *runConfig) error {
		cfg.stdout = w
		return nil
	}
}

// WithStderr overrides where ansible stderr is written (default io.Discard).
func WithStderr(w io.Writer) Option {
	return func(cfg *runConfig) error {
		cfg.stderr = w
		return nil
	}
}

// WithEnvVars merges the provided environment variables into the ansible process environment.
func WithEnvVars(env map[string]string) Option {
	return func(cfg *runConfig) error {
		if len(env) == 0 {
			return nil
		}
		if cfg.env == nil {
			cfg.env = make(map[string]string, len(env))
		}
		for k, v := range env {
			cfg.env[k] = v
		}
		return nil
	}
}

// WithEnvVar adds a single environment variable to the ansible process environment.
func WithEnvVar(key, value string) Option {
	return func(cfg *runConfig) error {
		if cfg.env == nil {
			cfg.env = make(map[string]string, 1)
		}
		cfg.env[key] = value
		return nil
	}
}

// WithExecutorFactory swaps the execute.Executor constructor (useful for tests).
func WithExecutorFactory(factory func(...execute.ExecuteOptions) execute.Executor) Option {
	return func(cfg *runConfig) error {
		if factory == nil {
			return fmt.Errorf("executor factory must not be nil")
		}
		cfg.executorFactory = factory
		return nil
	}
}

// WithBinary overrides the ansible-playbook binary path used to run the command.
func WithBinary(path string) Option {
	return func(cfg *runConfig) error {
		cfg.binary = strings.TrimSpace(path)
		return nil
	}
}

// Run builds and executes an ansible-playbook command for the provided request.
func Run(ctx context.Context, req RunRequest, opts ...Option) error {
	cmd, err := BuildCommand(req, opts...)
	if err != nil {
		return err
	}

	if err := cmd.Run(ctx); err != nil {
		return fmt.Errorf("ansibleplaybook: run playbook: %w", err)
	}

	return nil
}

// BuildCommand constructs a configured ansible-playbook command without executing it.
func BuildCommand(req RunRequest, opts ...Option) (*playbook.AnsiblePlaybookCmd, error) {
	cfg, err := buildConfig(opts...)
	if err != nil {
		return nil, err
	}

	norm, err := normalizeRequest(req)
	if err != nil {
		return nil, err
	}

	cmd := &playbook.AnsiblePlaybookCmd{
		Playbooks: []string{norm.PlaybookPath},
		Options: &playbook.AnsiblePlaybookOptions{
			Inventory: inlineInventory(norm.Target),
			Limit:     norm.Target,
		},
		ConnectionOptions: &options.AnsibleConnectionOptions{
			User:       norm.User,
			PrivateKey: norm.PrivateKeyPath,
		},
		PrivilegeEscalationOptions: &options.AnsiblePrivilegeEscalationOptions{
			Become:       true,
			BecomeMethod: becomeMethod,
			BecomeUser:   becomeUser,
		},
		Exec: cfg.executorFactory(buildExecutorOptions(cfg)...),
	}

	if cfg.binary != "" {
		cmd.Binary = cfg.binary
	}

	return cmd, nil
}

func normalizeRequest(req RunRequest) (RunRequest, error) {
	norm := RunRequest{
		User:           strings.TrimSpace(req.User),
		Target:         strings.TrimSpace(req.Target),
		PlaybookPath:   strings.TrimSpace(req.PlaybookPath),
		PrivateKeyPath: strings.TrimSpace(req.PrivateKeyPath),
	}

	switch {
	case norm.User == "":
		return RunRequest{}, ValidationError{Field: "user"}
	case norm.Target == "":
		return RunRequest{}, ValidationError{Field: "target"}
	case norm.PlaybookPath == "":
		return RunRequest{}, ValidationError{Field: "playbook path"}
	case norm.PrivateKeyPath == "":
		return RunRequest{}, ValidationError{Field: "private key path"}
	}

	return norm, nil
}

func buildConfig(opts ...Option) (*runConfig, error) {
	cfg := &runConfig{
		stdout: io.Discard,
		stderr: io.Discard,
		env: map[string]string{
			options.AnsibleHostKeyCheckingEnv: "false",
		},
		executorFactory: func(options ...execute.ExecuteOptions) execute.Executor {
			return execute.NewDefaultExecute(options...)
		},
	}

	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(cfg); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

func inlineInventory(target string) string {
	if strings.HasSuffix(target, ",") {
		return target
	}
	return target + ","
}

func buildExecutorOptions(cfg *runConfig) []execute.ExecuteOptions {
	var execOpts []execute.ExecuteOptions

	if cfg.stdout != nil {
		execOpts = append(execOpts, execute.WithWrite(cfg.stdout))
	}

	if cfg.stderr != nil {
		execOpts = append(execOpts, execute.WithWriteError(cfg.stderr))
	}

	for key, value := range cfg.env {
		execOpts = append(execOpts, execute.WithEnvVar(key, value))
	}

	return execOpts
}
