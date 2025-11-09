package ansibleprep

import (
	"github.com/BrianJOC/ansible-host-prep/phases"
	"github.com/BrianJOC/ansible-host-prep/phases/ansibleuser"
	"github.com/BrianJOC/ansible-host-prep/phases/pythonensure"
	"github.com/BrianJOC/ansible-host-prep/phases/sshconnect"
	"github.com/BrianJOC/ansible-host-prep/phases/sudoensure"
)

// Bundle returns the default ansible host preparation phases in execution order.
func Bundle() []phases.Phase {
	return []phases.Phase{
		sshconnect.New(),
		sudoensure.New(),
		pythonensure.New(),
		ansibleuser.New(),
	}
}
