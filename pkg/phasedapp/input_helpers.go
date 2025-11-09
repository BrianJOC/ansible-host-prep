package phasedapp

import "github.com/BrianJOC/ansible-host-prep/phases"

// InputOpt customizes input definitions produced by helper constructors.
type InputOpt func(*phases.InputDefinition)

// WithDescription sets the operator-facing description.
func WithDescription(desc string) InputOpt {
	return func(def *phases.InputDefinition) {
		if def != nil {
			def.Description = desc
		}
	}
}

// WithDefault sets the default value shown/applied by prompts.
func WithDefault(value any) InputOpt {
	return func(def *phases.InputDefinition) {
		if def != nil {
			def.Default = value
		}
	}
}

// Required marks the input as mandatory.
func Required() InputOpt {
	return func(def *phases.InputDefinition) {
		if def != nil {
			def.Required = true
		}
	}
}

// Optional clears the required flag for clarity at call sites.
func Optional() InputOpt {
	return func(def *phases.InputDefinition) {
		if def != nil {
			def.Required = false
		}
	}
}

// TextInput builds a basic text input definition.
func TextInput(id, label string, opts ...InputOpt) phases.InputDefinition {
	def := phases.InputDefinition{
		ID:    id,
		Label: label,
		Kind:  phases.InputKindText,
	}
	applyInputOpts(&def, opts...)
	return def
}

// SecretInput builds a secret/password input definition.
func SecretInput(id, label string, opts ...InputOpt) phases.InputDefinition {
	def := phases.InputDefinition{
		ID:     id,
		Label:  label,
		Kind:   phases.InputKindSecret,
		Secret: true,
	}
	applyInputOpts(&def, opts...)
	return def
}

// SelectInput builds a select/dropdown definition with the provided options.
func SelectInput(id, label string, options []phases.InputOption, opts ...InputOpt) phases.InputDefinition {
	def := phases.InputDefinition{
		ID:      id,
		Label:   label,
		Kind:    phases.InputKindSelect,
		Options: append([]phases.InputOption{}, options...),
	}
	applyInputOpts(&def, opts...)
	return def
}

func applyInputOpts(def *phases.InputDefinition, opts ...InputOpt) {
	for _, opt := range opts {
		if opt != nil {
			opt(def)
		}
	}
}
