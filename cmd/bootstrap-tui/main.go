package main

import (
	"context"
	"log"

	"github.com/BrianJOC/ansible-host-prep/phases/ansibleuser"
	"github.com/BrianJOC/ansible-host-prep/phases/pythonensure"
	"github.com/BrianJOC/ansible-host-prep/phases/sshconnect"
	"github.com/BrianJOC/ansible-host-prep/phases/sudoensure"
	"github.com/BrianJOC/ansible-host-prep/pkg/phasedapp"
)

func main() {
	app, err := phasedapp.New(
		phasedapp.WithPhases(
			sshconnect.New(),
			sudoensure.New(),
			pythonensure.New(),
			ansibleuser.New(),
		),
	)
	if err != nil {
		log.Fatalf("failed to initialize phased app: %v", err)
	}

	if err := app.Start(context.Background()); err != nil {
		log.Fatalf("tui exited with error: %v", err)
	}
}
