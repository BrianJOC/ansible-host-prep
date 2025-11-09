package sudoensure

import (
	"context"
	"errors"

	"golang.org/x/crypto/ssh"

	"github.com/BrianJOC/ansible-host-prep/phases"
	"github.com/BrianJOC/ansible-host-prep/phases/sshconnect"
	"github.com/BrianJOC/ansible-host-prep/utils/privilege"
)

const (
	phaseID = "sudo_ensure"

	// Input identifiers
	InputPassword = "password"

	// Context keys
	ContextKeyElevatedClient = "sudo:elevated_client"
)

// Ensurer wraps privilege escalation.
type Ensurer func(client *ssh.Client, password privilege.Password) (*privilege.ElevatedClient, error)

// Phase ensures sudo/root access is available.
type Phase struct {
	ensure Ensurer
}

// New creates a Phase that uses privilege.EnsureElevatedClient.
func New() *Phase {
	return &Phase{
		ensure: func(client *ssh.Client, password privilege.Password) (*privilege.ElevatedClient, error) {
			return privilege.EnsureElevatedClient(client, password)
		},
	}
}

// WithEnsurer allows injecting a custom ensurer for testing.
func (p *Phase) WithEnsurer(fn Ensurer) *Phase {
	if fn != nil {
		p.ensure = fn
	}
	return p
}

func (p *Phase) Metadata() phases.PhaseMetadata {
	return phases.PhaseMetadata{
		ID:          phaseID,
		Title:       "Ensure Sudo",
		Description: "Validate sudo access and install sudo if required.",
		Inputs: []phases.InputDefinition{
			{
				ID:          InputPassword,
				Label:       "Sudo Password",
				Description: "Password used when elevating privileges (prompted only when required).",
				Kind:        phases.InputKindSecret,
				Secret:      true,
				Required:    false,
			},
		},
	}
}

func (p *Phase) Run(ctx context.Context, phaseCtx *phases.Context) error {
	if p.ensure == nil {
		p.ensure = func(client *ssh.Client, password privilege.Password) (*privilege.ElevatedClient, error) {
			return privilege.EnsureElevatedClient(client, password)
		}
	}
	if phaseCtx == nil {
		phaseCtx = phases.NewContext()
	}

	clientVal, ok := phaseCtx.Get(sshconnect.ContextKeySSHClient)
	if !ok {
		return phases.ValidationError{Reason: "SSH connection phase must complete before sudo phase"}
	}

	client, ok := clientVal.(*ssh.Client)
	if !ok || client == nil {
		return phases.ValidationError{Reason: "invalid ssh client in context"}
	}

	password, inputErr := p.resolvePassword(phaseCtx)
	if inputErr != nil {
		return inputErr
	}

	elevated, err := p.ensure(client, privilege.Password{Value: password})
	if err != nil {
		if shouldRequestPassword(err) {
			phaseCtx.Set(sshconnect.ContextKeySSHPassword, nil)
			return phases.InputRequestError{
				PhaseID: phaseID,
				Input:   passwordInputDefinition(),
				Reason:  "password rejected; please enter a new password",
			}
		}
		return err
	}

	phaseCtx.Set(ContextKeyElevatedClient, elevated)
	phaseCtx.Set(sshconnect.ContextKeySSHPassword, password)

	return nil
}

func (p *Phase) resolvePassword(ctx *phases.Context) (string, error) {
	if val, ok := ctx.Get(sshconnect.ContextKeySSHPassword); ok {
		if str, ok := val.(string); ok && str != "" {
			return str, nil
		}
	}

	if val, ok := phases.GetInput(ctx, phaseID, InputPassword); ok {
		if str, ok := val.(string); ok && str != "" {
			return str, nil
		}
	}

	return "", phases.InputRequestError{
		PhaseID: phaseID,
		Input:   passwordInputDefinition(),
		Reason:  "sudo password required",
	}
}

func shouldRequestPassword(err error) bool {
	var sudoAuth privilege.SudoAuthenticationError
	if errors.As(err, &sudoAuth) {
		return true
	}
	var suAuth privilege.SuAuthenticationError
	return errors.As(err, &suAuth)
}

func passwordInputDefinition() phases.InputDefinition {
	return phases.InputDefinition{
		ID:          InputPassword,
		Label:       "Sudo Password",
		Description: "Password for privilege escalation",
		Kind:        phases.InputKindSecret,
		Secret:      true,
		Required:    true,
	}
}
