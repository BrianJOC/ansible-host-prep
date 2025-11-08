package ansibleuser

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/BrianJOC/prep-for-ansible/phases"
	"github.com/BrianJOC/prep-for-ansible/phases/sudoensure"
	"github.com/BrianJOC/prep-for-ansible/utils/privilege"
	"github.com/BrianJOC/prep-for-ansible/utils/sshkeypair"
	"github.com/BrianJOC/prep-for-ansible/utils/systemuser"
)

const (
	phaseID = "ansible_user"

	// Input identifiers
	InputKeyPath = "key_path"

	// Context keys
	ContextKeyUserResult = "ansible:user_result"
	ContextKeyKeyInfo    = "ansible:keypair_info"

	defaultUsername = "ansible"
)

// KeyPairEnsurer wraps sshkeypair.EnsureKeyPair.
type KeyPairEnsurer func(privatePath string, opts ...sshkeypair.Option) (*sshkeypair.KeyPairInfo, error)

// UserEnsurer wraps systemuser.EnsureUser.
type UserEnsurer func(r systemuser.Runner, username string, publicKey string, opts ...systemuser.Option) (*systemuser.Result, error)

// Phase creates the ansible user with passwordless sudo and SSH access.
type Phase struct {
	ensureKeyPair KeyPairEnsurer
	ensureUser    UserEnsurer
	username      string
}

// New constructs the ansible user phase.
func New() *Phase {
	return &Phase{
		ensureKeyPair: sshkeypair.EnsureKeyPair,
		ensureUser:    systemuser.EnsureUser,
		username:      defaultUsername,
	}
}

// WithKeyPairEnsurer overrides the key pair function (useful for testing).
func (p *Phase) WithKeyPairEnsurer(fn KeyPairEnsurer) *Phase {
	if fn != nil {
		p.ensureKeyPair = fn
	}
	return p
}

// WithUserEnsurer overrides the system user ensure function.
func (p *Phase) WithUserEnsurer(fn UserEnsurer) *Phase {
	if fn != nil {
		p.ensureUser = fn
	}
	return p
}

func (p *Phase) Metadata() phases.PhaseMetadata {
	return phases.PhaseMetadata{
		ID:          phaseID,
		Title:       "Ensure Ansible User",
		Description: fmt.Sprintf("Provision the %s user with passwordless sudo and SSH access.", p.username),
		Inputs: []phases.InputDefinition{
			keyPathDefinition(),
		},
	}
}

func (p *Phase) Run(ctx context.Context, phaseCtx *phases.Context) error {
	if phaseCtx == nil {
		phaseCtx = phases.NewContext()
	}

	if p.ensureKeyPair == nil {
		p.ensureKeyPair = sshkeypair.EnsureKeyPair
	}
	if p.ensureUser == nil {
		p.ensureUser = systemuser.EnsureUser
	}

	keyPath, err := p.resolveKeyPath(phaseCtx)
	if err != nil {
		return err
	}

	keyInfo, err := p.ensureKeyPair(keyPath)
	if err != nil {
		return err
	}

	publicKeyBytes, err := os.ReadFile(keyInfo.PublicPath)
	if err != nil {
		return err
	}
	publicKey := strings.TrimSpace(string(publicKeyBytes))
	if publicKey == "" {
		return phases.ValidationError{Reason: "public key content empty"}
	}

	elevatedVal, ok := phaseCtx.Get(sudoensure.ContextKeyElevatedClient)
	if !ok {
		return phases.ValidationError{Reason: "sudo phase must complete before creating ansible user"}
	}
	elevatedClient, ok := elevatedVal.(*privilege.ElevatedClient)
	if !ok || elevatedClient == nil {
		return phases.ValidationError{Reason: "invalid elevated client in context"}
	}

	runner := &sudoRunner{client: elevatedClient}

	result, err := p.ensureUser(
		runner,
		p.username,
		publicKey,
		systemuser.WithSudoAccess(),
		systemuser.WithPasswordlessSudo(),
	)
	if err != nil {
		return err
	}

	phaseCtx.Set(ContextKeyKeyInfo, keyInfo)
	phaseCtx.Set(ContextKeyUserResult, result)

	return nil
}

func (p *Phase) resolveKeyPath(ctx *phases.Context) (string, error) {
	val, ok := phases.GetInput(ctx, phaseID, InputKeyPath)
	if !ok {
		return "", phases.InputRequestError{
			PhaseID: phaseID,
			Input:   keyPathDefinition(),
			Reason:  "key path required to create ansible SSH key pair",
		}
	}
	path := strings.TrimSpace(fmt.Sprint(val))
	if path == "" {
		return "", phases.InputRequestError{
			PhaseID: phaseID,
			Input:   keyPathDefinition(),
			Reason:  "key path cannot be empty",
		}
	}
	return path, nil
}

func keyPathDefinition() phases.InputDefinition {
	return phases.InputDefinition{
		ID:          InputKeyPath,
		Label:       "Ansible SSH Key Path",
		Description: "Local path for the ansible user's SSH private key (e.g., ~/.ssh/ansible_id).",
		Kind:        phases.InputKindText,
		Required:    true,
	}
}

type sudoRunner struct {
	client *privilege.ElevatedClient
}

func (r *sudoRunner) Run(cmd string) (string, string, error) {
	return r.client.Run(cmd)
}
