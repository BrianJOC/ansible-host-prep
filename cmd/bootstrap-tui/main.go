package main

import (
	"context"
	"log"

	"github.com/BrianJOC/ansible-host-prep/pkg/phasedapp"
	"github.com/BrianJOC/ansible-host-prep/pkg/phasedapp/bundles/ansibleprep"
)

func main() {
	app, err := phasedapp.New(
		phasedapp.WithBundle(ansibleprep.Bundle),
	)
	if err != nil {
		log.Fatalf("failed to initialize phased app: %v", err)
	}

	if err := app.Start(context.Background()); err != nil {
		log.Fatalf("tui exited with error: %v", err)
	}
}
