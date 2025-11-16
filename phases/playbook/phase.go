package playbook

import (
	"context"
	"fmt"
	"strings"

	"github.com/BrianJOC/ansible-host-prep/phases"
	"github.com/BrianJOC/ansible-host-prep/phases/ansibleuser"
	"github.com/BrianJOC/ansible-host-prep/phases/sshconnect"
	ansiblepb "github.com/BrianJOC/ansible-host-prep/utils/ansibleplaybook"
	"github.com/BrianJOC/ansible-host-prep/utils/sshkeypair"
	"github.com/BrianJOC/ansible-host-prep/utils/systemuser"
)

const (
	defaultPhaseID = "ansible_playbook"

	// Input identifiers.
	InputTargetHost     = "target_host"
	InputAnsibleUser    = "ansible_user"
	InputPrivateKeyPath = "private_key_path"
	InputPlaybookPath   = "playbook_path"

	// Context keys for sharing resolved values.
	ContextKeyTargetHost     = "playbook:target_host"
	ContextKeyAnsibleUser    = "playbook:ansible_user"
	ContextKeyPrivateKeyPath = "playbook:key_path"
	ContextKeyPlaybookPath   = "playbook:path"
)

// Runner executes the ansible playbook.
type Runner func(context.Context, ansiblepb.RunRequest, ...ansiblepb.Option) error

// Config describes a reusable playbook phase.
type Config struct {
	ID           string
	Title        string
	Description  string
	PlaybookPath string
	Tags         []string
	Options      []ansiblepb.Option
}

// Phase coordinates collecting target/user/key details and running an ansible playbook.
type Phase struct {
	meta         phases.PhaseMetadata
	playbookPath string
	options      []ansiblepb.Option
	run          Runner
}

// New constructs a reusable ansible playbook phase based on the provided config.
func New(cfg Config) *Phase {
	id := strings.TrimSpace(cfg.ID)
	if id == "" {
		id = defaultPhaseID
	}

	title := strings.TrimSpace(cfg.Title)
	if title == "" {
		title = "Run Ansible Playbook"
	}

	desc := strings.TrimSpace(cfg.Description)
	if desc == "" {
		desc = "Execute an Ansible playbook against the target host."
		if cfg.PlaybookPath != "" {
			desc = fmt.Sprintf("Execute %s against the target host.", strings.TrimSpace(cfg.PlaybookPath))
		}
	}

	playbookPath := strings.TrimSpace(cfg.PlaybookPath)

	meta := phases.PhaseMetadata{
		ID:          id,
		Title:       title,
		Description: desc,
		Inputs:      inputDefinitions(playbookPath == ""),
		Tags:        append([]string{}, cfg.Tags...),
	}

	return &Phase{
		meta:         meta,
		playbookPath: playbookPath,
		options:      append([]ansiblepb.Option{}, cfg.Options...),
		run:          ansiblepb.Run,
	}
}

// Metadata returns the configured phase metadata.
func (p *Phase) Metadata() phases.PhaseMetadata {
	return p.meta
}

// WithRunner overrides the ansible playbook executor (useful for tests).
func (p *Phase) WithRunner(r Runner) *Phase {
	if r != nil {
		p.run = r
	}
	return p
}

// WithOptions appends ansibleplaybook options applied during execution.
func (p *Phase) WithOptions(opts ...ansiblepb.Option) *Phase {
	if len(opts) == 0 {
		return p
	}
	p.options = append(p.options, opts...)
	return p
}

// Run resolves playbook inputs (preferring prior phases) and executes the playbook.
func (p *Phase) Run(ctx context.Context, phaseCtx *phases.Context) error {
	if p.run == nil {
		p.run = ansiblepb.Run
	}
	if phaseCtx == nil {
		phaseCtx = phases.NewContext()
	}

	target, err := p.resolveTarget(phaseCtx)
	if err != nil {
		return err
	}

	user, err := p.resolveUser(phaseCtx)
	if err != nil {
		return err
	}

	keyPath, err := p.resolveKeyPath(phaseCtx)
	if err != nil {
		return err
	}

	playbookPath, err := p.resolvePlaybookPath(phaseCtx)
	if err != nil {
		return err
	}

	req := ansiblepb.RunRequest{
		User:           user,
		Target:         target,
		PlaybookPath:   playbookPath,
		PrivateKeyPath: keyPath,
	}

	if err := p.run(ctx, req, p.options...); err != nil {
		return fmt.Errorf("playbook phase: run ansible playbook: %w", err)
	}

	phaseCtx.Set(ContextKeyTargetHost, target)
	phaseCtx.Set(ContextKeyAnsibleUser, user)
	phaseCtx.Set(ContextKeyPrivateKeyPath, keyPath)
	phaseCtx.Set(ContextKeyPlaybookPath, playbookPath)

	return nil
}

func (p *Phase) resolveTarget(ctx *phases.Context) (string, error) {
	if ctx != nil {
		if val, ok := ctx.Get(sshconnect.ContextKeyTargetHost); ok {
			host := strings.TrimSpace(fmt.Sprint(val))
			if host != "" {
				return host, nil
			}
		}
	}

	if host, ok := getInput(ctx, p.meta.ID, InputTargetHost); ok && host != "" {
		return host, nil
	}

	return "", p.inputRequestError(InputTargetHost, "target host is required to run the playbook")
}

func (p *Phase) resolveUser(ctx *phases.Context) (string, error) {
	if ctx != nil {
		if val, ok := ctx.Get(ansibleuser.ContextKeyUserResult); ok {
			if res, ok := val.(*systemuser.Result); ok && res != nil {
				user := strings.TrimSpace(res.Username)
				if user != "" {
					return user, nil
				}
			}
		}

		if val, ok := ctx.Get(sshconnect.ContextKeyTargetUser); ok {
			user := strings.TrimSpace(fmt.Sprint(val))
			if user != "" {
				return user, nil
			}
		}
	}

	if user, ok := getInput(ctx, p.meta.ID, InputAnsibleUser); ok && user != "" {
		return user, nil
	}

	return "", p.inputRequestError(InputAnsibleUser, "ansible user is required to run the playbook")
}

func (p *Phase) resolveKeyPath(ctx *phases.Context) (string, error) {
	if ctx != nil {
		if val, ok := ctx.Get(ansibleuser.ContextKeyKeyInfo); ok {
			if info, ok := val.(*sshkeypair.KeyPairInfo); ok && info != nil {
				keyPath := strings.TrimSpace(info.PrivatePath)
				if keyPath != "" {
					return keyPath, nil
				}
			}
		}
	}

	if keyPath, ok := getInput(ctx, p.meta.ID, InputPrivateKeyPath); ok && keyPath != "" {
		return keyPath, nil
	}

	return "", p.inputRequestError(InputPrivateKeyPath, "private key path is required for ansible SSH access")
}

func (p *Phase) resolvePlaybookPath(ctx *phases.Context) (string, error) {
	if p.playbookPath != "" {
		return p.playbookPath, nil
	}

	if path, ok := getInput(ctx, p.meta.ID, InputPlaybookPath); ok && path != "" {
		return path, nil
	}

	return "", p.inputRequestError(InputPlaybookPath, "playbook path is required")
}

func (p *Phase) inputRequestError(inputID, reason string) phases.InputRequestError {
	return phases.InputRequestError{
		PhaseID: p.meta.ID,
		Input:   inputDefinition(inputID),
		Reason:  reason,
	}
}

func inputDefinitions(includePlaybook bool) []phases.InputDefinition {
	inputs := []phases.InputDefinition{
		targetDefinition(),
		userDefinition(),
		keyPathDefinition(),
	}

	if includePlaybook {
		inputs = append(inputs, playbookPathDefinition())
	}

	return inputs
}

func inputDefinition(inputID string) phases.InputDefinition {
	switch inputID {
	case InputTargetHost:
		return targetDefinition()
	case InputAnsibleUser:
		return userDefinition()
	case InputPrivateKeyPath:
		return keyPathDefinition()
	case InputPlaybookPath:
		return playbookPathDefinition()
	default:
		return phases.InputDefinition{
			ID:    inputID,
			Label: inputID,
			Kind:  phases.InputKindText,
		}
	}
}

func targetDefinition() phases.InputDefinition {
	return phases.InputDefinition{
		ID:          InputTargetHost,
		Label:       "Target Host",
		Description: "Hostname or IP of the target to run the playbook against.",
		Kind:        phases.InputKindText,
		Required:    true,
	}
}

func userDefinition() phases.InputDefinition {
	return phases.InputDefinition{
		ID:          InputAnsibleUser,
		Label:       "Ansible User",
		Description: "Remote user Ansible should connect as.",
		Kind:        phases.InputKindText,
		Required:    true,
	}
}

func keyPathDefinition() phases.InputDefinition {
	return phases.InputDefinition{
		ID:          InputPrivateKeyPath,
		Label:       "Private Key Path",
		Description: "Path to the private key for the ansible user.",
		Kind:        phases.InputKindText,
		Required:    true,
	}
}

func playbookPathDefinition() phases.InputDefinition {
	return phases.InputDefinition{
		ID:          InputPlaybookPath,
		Label:       "Playbook Path",
		Description: "Filesystem path to the Ansible playbook to execute.",
		Kind:        phases.InputKindText,
		Required:    true,
	}
}

func getInput(ctx *phases.Context, phaseID, inputID string) (string, bool) {
	val, ok := phases.GetInput(ctx, phaseID, inputID)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(fmt.Sprint(val)), true
}
