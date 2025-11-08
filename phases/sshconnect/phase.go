package sshconnect

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"

	"github.com/BrianJOC/prep-for-ansible/phases"
	"github.com/BrianJOC/prep-for-ansible/utils/sshconnection"
)

const (
	phaseID = "ssh_connection"

	// Input identifiers
	InputHost       = "host"
	InputPort       = "port"
	InputUsername   = "username"
	InputAuthMethod = "auth_method"
	InputPassword   = "password"
	InputKeyPath    = "key_path"

	// Context keys for downstream phases
	ContextKeySSHClient   = "ssh:client"
	ContextKeySSHPassword = "ssh:password"
	ContextKeyTargetHost  = "ssh:target_host"
	ContextKeyTargetUser  = "ssh:target_user"
	ContextKeyAuthMethod  = "ssh:auth_method"
)

const (
	authMethodPassword = "password"
	authMethodKeyPath  = "private_key"
)

// Connector establishes SSH clients.
type Connector func(host string, port int, username string, cred sshconnection.Credential, opts ...sshconnection.Option) (*ssh.Client, error)

// Phase establishes an SSH client based on operator-provided inputs.
type Phase struct {
	connect Connector
}

// New creates a Phase that uses sshconnection.Connect.
func New() *Phase {
	return &Phase{
		connect: sshconnection.Connect,
	}
}

// WithConnector allows injecting a custom connector (useful for tests).
func (p *Phase) WithConnector(conn Connector) *Phase {
	if conn != nil {
		p.connect = conn
	}
	return p
}

func (p *Phase) Metadata() phases.PhaseMetadata {
	return phases.PhaseMetadata{
		ID:          phaseID,
		Title:       "SSH Connection",
		Description: "Collect target details and establish an SSH session.",
		Inputs: []phases.InputDefinition{
			{
				ID:          InputHost,
				Label:       "Target Host",
				Description: "Hostname or IP of the remote system.",
				Kind:        phases.InputKindText,
				Required:    true,
			},
			{
				ID:          InputPort,
				Label:       "Port",
				Description: "SSH port (defaults to 22).",
				Kind:        phases.InputKindText,
				Required:    false,
			},
			{
				ID:          InputUsername,
				Label:       "Username",
				Description: "Remote user for the SSH session.",
				Kind:        phases.InputKindText,
				Required:    true,
			},
			{
				ID:          InputAuthMethod,
				Label:       "Authentication Method",
				Description: "Choose password or existing private key.",
				Kind:        phases.InputKindSelect,
				Required:    true,
				Options: []phases.InputOption{
					{Value: authMethodPassword, Label: "Password"},
					{Value: authMethodKeyPath, Label: "Private Key"},
				},
			},
			{
				ID:          InputPassword,
				Label:       "Password",
				Description: "Password for SSH authentication (if applicable).",
				Kind:        phases.InputKindSecret,
				Secret:      true,
				Required:    false,
			},
			{
				ID:          InputKeyPath,
				Label:       "Private Key Path",
				Description: "Absolute path to an existing private key.",
				Kind:        phases.InputKindText,
				Required:    false,
			},
		},
	}
}

func (p *Phase) Run(ctx context.Context, phaseCtx *phases.Context) error {
	if p.connect == nil {
		p.connect = sshconnection.Connect
	}
	if phaseCtx == nil {
		phaseCtx = phases.NewContext()
	}

	host, err := getRequiredInput(phaseCtx, InputHost)
	if err != nil {
		return err
	}
	username, err := getRequiredInput(phaseCtx, InputUsername)
	if err != nil {
		return err
	}

	port := 22
	if raw, ok := getInput(phaseCtx, InputPort); ok {
		if str := strings.TrimSpace(fmt.Sprint(raw)); str != "" {
			value, convErr := strconv.Atoi(str)
			if convErr != nil || value <= 0 {
				return phases.ValidationError{Reason: "port must be a positive integer"}
			}
			port = value
		}
	}

	authMethod, err := getRequiredInput(phaseCtx, InputAuthMethod)
	if err != nil {
		return err
	}

	var credential sshconnection.Credential
	switch authMethod {
	case authMethodPassword:
		password, pErr := getRequiredInput(phaseCtx, InputPassword)
		if pErr != nil {
			return phases.ValidationError{Reason: "password is required for password authentication"}
		}
		credential = sshconnection.Credential{Password: password}
		phaseCtx.Set(ContextKeySSHPassword, password)
	case authMethodKeyPath:
		keyPath, kErr := getRequiredInput(phaseCtx, InputKeyPath)
		if kErr != nil {
			return phases.ValidationError{Reason: "key path is required for private key authentication"}
		}
		credential = sshconnection.Credential{KeyPath: keyPath}
	default:
		return phases.ValidationError{Reason: "unsupported authentication method"}
	}

	client, err := p.connect(host, port, username, credential)
	if err != nil {
		return err
	}

	phaseCtx.Set(ContextKeySSHClient, client)
	phaseCtx.Set(ContextKeyTargetHost, host)
	phaseCtx.Set(ContextKeyTargetUser, username)
	phaseCtx.Set(ContextKeyAuthMethod, authMethod)

	return nil
}

func getInput(ctx *phases.Context, inputID string) (string, bool) {
	val, ok := phases.GetInput(ctx, phaseID, inputID)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(fmt.Sprint(val)), true
}

func getRequiredInput(ctx *phases.Context, inputID string) (string, error) {
	value, ok := getInput(ctx, inputID)
	if !ok || value == "" {
		return "", phases.ValidationError{Reason: fmt.Sprintf("%s is required", inputID)}
	}
	return value, nil
}
