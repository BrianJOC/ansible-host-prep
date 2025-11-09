## Phase Authoring & Bundling Ideas

### 1. SimplePhase Helper

Provide a helper constructor so contributors don't need to declare a struct just to satisfy `phases.Phase`.

```go
// pkg/phasedapp/simplephase.go
type SimplePhase struct {
	meta phases.PhaseMetadata
	run  func(ctx context.Context, phaseCtx *phases.Context) error
}

func NewPhase(meta phases.PhaseMetadata, run func(context.Context, *phases.Context) error) phases.Phase {
	return SimplePhase{meta: meta, run: run}
}

func (p SimplePhase) Metadata() phases.PhaseMetadata { return p.meta }
func (p SimplePhase) Run(ctx context.Context, phaseCtx *phases.Context) error {
	return p.run(ctx, phaseCtx)
}
```

**Usage**

```go
phase := phasedapp.NewPhase(
	phases.PhaseMetadata{
		ID:          "greet",
		Title:       "Greet Host",
		Description: "Collect name and greet target",
		Inputs: []phases.InputDefinition{
			phasedapp.TextInput("operator", "Operator Name", phases.InputOption{Required: true}),
		},
	},
	func(ctx context.Context, phaseCtx *phases.Context) error {
		name, _ := phases.GetInput[string](phaseCtx, "greet", "operator")
		log.Printf("Hello %s!", name)
		return nil
	},
)
```

### 2. Input Definition Helpers

Expose utilities to standardize common input kinds.

```go
// pkg/phasedapp/input_helpers.go
func TextInput(id, label string, opts ...InputOpt) phases.InputDefinition
func SecretInput(id, label string, opts ...InputOpt) phases.InputDefinition
func SelectInput(id, label string, options []phases.InputOption, opts ...InputOpt) phases.InputDefinition
```

**Usage**

```go
meta.Inputs = []phases.InputDefinition{
	phasedapp.TextInput("host", "Target Host", phasedapp.DefaultValue("server.local")),
	phasedapp.SecretInput("password", "Sudo Password"),
}
```

### 3. Phase Bundles

Encourage bundling related phases for reuse.

```go
// phases/ansibleprep/bundle.go
package ansibleprep

func Bundle() []phases.Phase {
	return []phases.Phase{
		sshconnect.New(),
		sudoensure.New(),
		pythonensure.New(),
		ansibleuser.New(),
	}
}
```

Add an option helper so consumers can pass bundles cleanly:

```go
func WithBundle(bundle func() []phases.Phase) Option {
	return func(cfg *Config) {
		cfg.Phases = append(cfg.Phases, bundle()...)
	}
}
```

**Usage**

```go
app, _ := phasedapp.New(
	phasedapp.WithBundle(ansibleprep.Bundle),
	phasedapp.WithPhases(customPhase),
)
```

### 4. Builder DSL

Offer a builder for assembling pipelines with validation.

```go
builder := phasedapp.NewBuilder().
	AddPhase(sshconnect.New()).
	AddPhase(sudoensure.New()).
	AddPhase(phasedapp.NewPhase(meta, runFn))

phases := builder.Build() // returns []phases.Phase or error on duplicates
app, _ := phasedapp.New(phasedapp.WithPhases(phases...))
```

### 5. Metadata Tags

Extend `phases.PhaseMetadata` with optional tags so bundles/UIs can filter.

```go
type PhaseMetadata struct {
	ID          string
	Title       string
	Description string
	Inputs      []InputDefinition
	Tags        []string // e.g., {"ansible", "bootstrap"}
}
```

Consumers could then select bundles dynamically:

```go
bundle := phasedapp.SelectPhases(allPhases, phasedapp.WithTag("ansible"))
app, _ := phasedapp.New(phasedapp.WithPhases(bundle...))
```

### 6. Context Helpers

Sharing rich data (SSH clients, elevated shells, etc.) currently requires hand-managed keys and type assertions. Provide helpers that standardize keys and introduce typed accessors.

```go
// pkg/phasedapp/context_keys.go
type ContextKey string

func Namespace(ns, name string) ContextKey {
	return ContextKey(fmt.Sprintf("%s:%s", ns, name))
}

func Set[T any](ctx *phases.Context, key ContextKey, value T) {
	ctx.Set(string(key), value)
}

func Get[T any](ctx *phases.Context, key ContextKey) (T, bool) {
	val, ok := ctx.Get(string(key))
	if !ok {
		var zero T
		return zero, false
	}
	casted, ok := val.(T)
	if !ok {
		var zero T
		return zero, false
	}
	return casted, true
}
```

**Usage**

```go
var (
	ContextSSHClient = phasedapp.Namespace("ssh", "client")
	ContextSSHHost   = phasedapp.Namespace("ssh", "host")
)

// producer phase
phasedapp.Set(ctx, ContextSSHClient, client)
phasedapp.Set(ctx, ContextSSHHost, host)

// consumer phase
client, ok := phasedapp.Get[*ssh.Client](ctx, ContextSSHClient)
if !ok {
	return fmt.Errorf("missing ssh client")
}
```

For bundles, publish the keys alongside the phase constructors so downstream consumers know exactly which artifacts they can rely on (`ansibleprep.Keys.SSHClient`, etc.).

These additions would reduce boilerplate for new phases, improve consistency for inputs, and make it easy to package/share cohesive pipelines such as the default Ansible prep flow.
