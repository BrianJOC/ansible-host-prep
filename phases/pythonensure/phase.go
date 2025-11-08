package pythonensure

import (
	"context"

	"github.com/BrianJOC/ansible-host-prep/phases"
	"github.com/BrianJOC/ansible-host-prep/phases/sudoensure"
	"github.com/BrianJOC/ansible-host-prep/utils/pkginstaller"
	"github.com/BrianJOC/ansible-host-prep/utils/privilege"
)

const (
	phaseID             = "python_ensure"
	defaultPackageName  = "python3"
	defaultBinaryName   = "python3"
	ContextKeyInstalled = "python:installed"
)

// InstallerFunc wraps pkginstaller.Ensure for dependency injection.
type InstallerFunc func(r pkginstaller.Runner, packageName string, opts ...pkginstaller.Option) (*pkginstaller.Result, error)

// Phase ensures Python 3 is present on the remote target.
type Phase struct {
	install InstallerFunc
}

// New creates a Python ensure phase.
func New() *Phase {
	return &Phase{
		install: pkginstaller.Ensure,
	}
}

// WithInstaller allows providing a custom installer (for tests).
func (p *Phase) WithInstaller(fn InstallerFunc) *Phase {
	if fn != nil {
		p.install = fn
	}
	return p
}

func (p *Phase) Metadata() phases.PhaseMetadata {
	return phases.PhaseMetadata{
		ID:          phaseID,
		Title:       "Ensure Python 3",
		Description: "Install or verify python3 on the target system.",
	}
}

func (p *Phase) Run(ctx context.Context, phaseCtx *phases.Context) error {
	if p.install == nil {
		p.install = pkginstaller.Ensure
	}
	if phaseCtx == nil {
		phaseCtx = phases.NewContext()
	}

	elevatedVal, ok := phaseCtx.Get(sudoensure.ContextKeyElevatedClient)
	if !ok {
		return phases.ValidationError{Reason: "sudo phase must complete before ensuring python"}
	}

	elevatedClient, ok := elevatedVal.(*privilege.ElevatedClient)
	if !ok || elevatedClient == nil {
		return phases.ValidationError{Reason: "invalid elevated client in context"}
	}

	runner := &sudoRunner{client: elevatedClient}

	_, err := p.install(runner, defaultPackageName, pkginstaller.WithCustomCheck("command -v "+defaultBinaryName+" >/dev/null 2>&1"))
	if err != nil {
		return err
	}

	phaseCtx.Set(ContextKeyInstalled, true)
	return nil
}

type sudoRunner struct {
	client *privilege.ElevatedClient
}

func (r *sudoRunner) Run(cmd string) (string, string, error) {
	return r.client.Run(cmd)
}
