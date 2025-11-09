// Package phasedapp exposes a reusable Bubble Tea orchestrator for phased
// bootstrap pipelines. It wires the phases.Manager, observers, and input
// handling behind a simple lifecycle API so other binaries can embed the
// interactive workflow without copying UI code.
package phasedapp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/spinner"
	textinput "github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/BrianJOC/ansible-host-prep/phases"
)

var (
	// ErrNoPhases indicates no phases were supplied when constructing an App.
	ErrNoPhases = errors.New("phasedapp: at least one phase must be registered")
	// ErrProgramRunning reports that Start was invoked while the program is already running.
	ErrProgramRunning = errors.New("phasedapp: program already running")
)

// Config controls how an App should be assembled.
type Config struct {
	Phases         []phases.Phase
	ManagerOptions []phases.ManagerOption
	ProgramOptions []tea.ProgramOption
}

// Option mutates Config during construction.
type Option func(*Config)

// WithPhases sets the ordered phases the app should execute.
func WithPhases(phases ...phases.Phase) Option {
	return func(cfg *Config) {
		if cfg == nil {
			return
		}
		cfg.Phases = append(cfg.Phases, phases...)
	}
}

// WithManagerOptions appends custom manager options.
func WithManagerOptions(opts ...phases.ManagerOption) Option {
	return func(cfg *Config) {
		if cfg == nil {
			return
		}
		cfg.ManagerOptions = append(cfg.ManagerOptions, opts...)
	}
}

// WithProgramOptions appends tea.Program options.
func WithProgramOptions(opts ...tea.ProgramOption) Option {
	return func(cfg *Config) {
		if cfg == nil {
			return
		}
		cfg.ProgramOptions = append(cfg.ProgramOptions, opts...)
	}
}

// App hosts the Bubble Tea-driven phase runner.
type App struct {
	cfg      Config
	mu       sync.Mutex
	program  *tea.Program
	inFlight bool
}

// New constructs an App from the provided options.
func New(opts ...Option) (*App, error) {
	cfg := Config{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if len(cfg.Phases) == 0 {
		return nil, ErrNoPhases
	}
	return &App{cfg: cfg}, nil
}

// Start begins executing the TUI pipeline from the first registered phase.
func (a *App) Start(ctx context.Context) error {
	return a.start(ctx, 0)
}

// StartFrom begins executing the TUI pipeline from the provided phase index.
func (a *App) StartFrom(ctx context.Context, start int) error {
	if start < 0 {
		start = 0
	}
	return a.start(ctx, start)
}

// Stop signals the running TUI program (if any) to exit.
func (a *App) Stop() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.program == nil {
		return nil
	}
	a.program.Quit()
	return nil
}

func (a *App) start(ctx context.Context, start int) error {
	if ctx == nil {
		ctx = context.Background()
	}
	model, err := newModel(a.cfg, start, ctx)
	if err != nil {
		return err
	}
	program := tea.NewProgram(model, a.cfg.ProgramOptions...)

	a.mu.Lock()
	if a.inFlight {
		a.mu.Unlock()
		return ErrProgramRunning
	}
	a.program = program
	a.inFlight = true
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		a.program = nil
		a.inFlight = false
		a.mu.Unlock()
	}()

	_, runErr := program.Run()
	return runErr
}

type phaseStatus int

const (
	statusPending phaseStatus = iota
	statusRunning
	statusSuccess
	statusFailed
)

func (s phaseStatus) String() string {
	return statusLabel(s)
}

type focusArea int

const (
	focusPhases focusArea = iota
	focusPrompt
)

type phaseState struct {
	meta   phases.PhaseMetadata
	status phaseStatus
	err    error
	logs   []string
}

type model struct {
	manager      *phases.Manager
	phaseCtx     *phases.Context
	observer     *phaseObserver
	inputHandler *bubbleInputHandler
	runCtx       context.Context

	phases map[string]*phaseState
	order  []string

	spinner spinner.Model

	prompt       textinput.Model
	activePrompt *inputRequestMsg
	prompting    bool
	selectIndex  int

	savedInputs  map[string]map[string]any
	secretValues map[string]struct{}

	selectedPhase  int
	focus          focusArea
	helpVisible    bool
	pipelineActive bool
	actionsVisible bool

	statusMsg string
	done      error

	width  int
	height int

	initialStartIndex int
}

func newModel(cfg Config, startIndex int, runCtx context.Context) (*model, error) {
	if len(cfg.Phases) == 0 {
		return nil, ErrNoPhases
	}

	phaseCtx := phases.NewContext()
	inputHandler := newBubbleInputHandler()
	observer := newPhaseObserver()

	managerOpts := append([]phases.ManagerOption{}, cfg.ManagerOptions...)
	managerOpts = append(managerOpts,
		phases.WithObserver(observer),
		phases.WithInputHandler(inputHandler),
	)
	manager := phases.NewManager(managerOpts...)

	if err := manager.Register(cfg.Phases...); err != nil {
		return nil, err
	}

	states := make(map[string]*phaseState, len(cfg.Phases))
	order := make([]string, 0, len(cfg.Phases))
	for _, ph := range cfg.Phases {
		if ph == nil {
			continue
		}
		meta := ph.Metadata()
		states[meta.ID] = &phaseState{meta: meta, status: statusPending}
		order = append(order, meta.ID)
	}

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	ti := textinput.New()
	ti.Placeholder = "enter value"
	ti.Blur()

	if runCtx == nil {
		runCtx = context.Background()
	}

	return &model{
		manager:           manager,
		phaseCtx:          phaseCtx,
		observer:          observer,
		inputHandler:      inputHandler,
		runCtx:            runCtx,
		phases:            states,
		order:             order,
		spinner:           sp,
		prompt:            ti,
		focus:             focusPhases,
		selectedPhase:     0,
		savedInputs:       make(map[string]map[string]any),
		secretValues:      make(map[string]struct{}),
		statusMsg:         "Awaiting phase events…",
		pipelineActive:    false,
		initialStartIndex: startIndex,
	}, nil
}

func (m *model) Init() tea.Cmd {
	return m.startPipelineFrom(m.initialStartIndex)
}

func (m *model) startPipeline() tea.Cmd {
	return m.startPipelineFrom(0)
}

func (m *model) startPipelineFrom(start int) tea.Cmd {
	start = m.clampStartIndex(start)
	m.pipelineActive = true
	m.actionsVisible = false
	return tea.Batch(
		runManagerCmd(m.runCtx, m.manager, m.phaseCtx, start),
		waitPhaseEventCmd(m.observer),
		waitInputRequestCmd(m.inputHandler),
		m.spinner.Tick,
	)
}

func (m *model) clampStartIndex(idx int) int {
	if len(m.order) == 0 {
		return 0
	}
	if idx < 0 {
		return 0
	}
	if idx >= len(m.order) {
		return len(m.order) - 1
	}
	return idx
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		prevWidth := m.width
		prevHeight := m.height
		m.width = msg.Width
		m.height = msg.Height
		if (prevWidth > 0 && msg.Width < prevWidth) || (prevHeight > 0 && msg.Height < prevHeight) {
			return m, tea.ClearScreen
		}
		return m, nil
	case tea.KeyMsg:
		if m.actionsVisible {
			if handled, cmd := m.handleActionKeys(msg); handled {
				return m, cmd
			}
			return m, nil
		}
		if m.handleSelectPromptNavigation(msg) {
			return m, nil
		}
		if m.handlePhaseNavigation(msg) {
			return m, nil
		}
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyCtrlR:
			return m, m.restartPipeline()
		case tea.KeyEnter:
			if m.prompting && m.focus == focusPrompt {
				return m, m.submitPrompt()
			}
			if !m.prompting && m.focus == focusPhases {
				m.actionsVisible = !m.actionsVisible
				m.helpVisible = false
				return m, nil
			}
		case tea.KeyEsc:
			return m, m.handleEscape()
		case tea.KeyTab, tea.KeyShiftTab:
			if m.prompting {
				m.toggleFocus()
			}
			return m, nil
		case tea.KeyRunes:
			if len(msg.Runes) == 1 {
				switch msg.Runes[0] {
				case 'r', 'R':
					return m, m.restartPipeline()
				case '?', 'h', 'H':
					m.helpVisible = !m.helpVisible
					return m, nil
				}
			}
		}

		if m.prompting && m.focus == focusPrompt && !m.isSelectPrompt() {
			var cmd tea.Cmd
			m.prompt, cmd = m.prompt.Update(msg)
			return m, cmd
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case phaseStartedMsg:
		m.handlePhaseStarted(msg)
		return m, tea.Batch(waitPhaseEventCmd(m.observer), m.spinner.Tick)

	case phaseCompletedMsg:
		m.handlePhaseCompleted(msg)
		return m, tea.Batch(waitPhaseEventCmd(m.observer), m.spinner.Tick)

	case inputRequestMsg:
		m.preparePrompt(msg)
		return m, nil

	case phasesFinishedMsg:
		m.pipelineActive = false
		m.done = msg.err
		if msg.err != nil {
			m.setStatus(msg.err.Error())
		} else {
			m.setStatus("All phases completed")
		}
		return m, nil
	}

	return m, nil
}

func (m *model) handlePhaseStarted(msg phaseStartedMsg) {
	if state, ok := m.phases[msg.meta.ID]; ok {
		state.status = statusRunning
		state.err = nil
		m.appendLog(state, fmt.Sprintf("%s started", msg.meta.Title))
	}
	m.setStatusf("Running %s", msg.meta.Title)
}

func (m *model) handlePhaseCompleted(msg phaseCompletedMsg) {
	state, ok := m.phases[msg.meta.ID]
	if !ok {
		return
	}
	if msg.err != nil {
		state.status = statusFailed
		state.err = msg.err
		m.appendLog(state, fmt.Sprintf("%s failed: %v", msg.meta.Title, msg.err))
		m.setStatusf("%s failed — %v", msg.meta.Title, msg.err)
	} else {
		state.status = statusSuccess
		state.err = nil
		m.appendLog(state, fmt.Sprintf("%s completed", msg.meta.Title))
		m.setStatusf("%s completed", msg.meta.Title)
	}
}

func (m *model) preparePrompt(msg inputRequestMsg) {
	m.actionsVisible = false
	msg.reason = sanitizeInputReason(msg.input, msg.reason)
	m.activePrompt = &msg
	m.prompting = true
	m.focus = focusPrompt
	m.helpVisible = false
	m.selectIndex = 0

	prevVal, _ := m.lookupInputString(msg.meta.ID, msg.input.ID)
	defaultValue := defaultString(msg.input.Default)

	if msg.input.Kind == phases.InputKindSelect && prevVal == "" && defaultValue != "" {
		prevVal = defaultValue
	}

	m.prompt.EchoMode = textinput.EchoNormal
	m.prompt.EchoCharacter = '*'
	if msg.input.Kind == phases.InputKindSecret {
		m.prompt.EchoMode = textinput.EchoPassword
		m.prompt.EchoCharacter = '•'
	}

	switch msg.input.Kind {
	case phases.InputKindSelect:
		if idx := m.optionIndex(prevVal); idx >= 0 {
			m.selectIndex = idx
		}
		m.prompt.Blur()
		if len(msg.input.Options) == 0 {
			m.setStatusf("%s requested %s but no options available", msg.meta.Title, msg.input.Label)
		} else {
			m.setStatusf("%s: choose %s (arrows, j/k, numbers)", msg.meta.Title, msg.input.Label)
		}
	default:
		m.prompt.Placeholder = placeholderText(msg.input, defaultValue)
		if prevVal != "" {
			m.prompt.SetValue(prevVal)
		} else {
			m.prompt.SetValue("")
		}
		m.prompt.CursorEnd()
		m.prompt.Focus()
		m.setStatusf("%s needs %s", msg.meta.Title, msg.input.Label)
	}
}

func (m *model) submitPrompt() tea.Cmd {
	if !m.prompting || m.activePrompt == nil {
		return nil
	}

	defer func() {
		m.prompting = false
		m.activePrompt = nil
		m.prompt.SetValue("")
		m.prompt.EchoMode = textinput.EchoNormal
		m.focus = focusPhases
	}()

	if m.isSelectPrompt() {
		value, ok := m.currentSelectionValue()
		if !ok {
			m.setStatus("No options available")
			return nil
		}
		m.recordInput(value)
		m.inputHandler.respond(value, nil)
	} else {
		value := strings.TrimSpace(m.prompt.Value())
		if value == "" {
			defaultValue := defaultString(m.activePrompt.input.Default)
			if defaultValue != "" && m.activePrompt.input.Kind != phases.InputKindSecret {
				value = defaultValue
			}
		}
		if value == "" && m.activePrompt.input.Required {
			m.setStatus("Input required")
			return nil
		}
		m.recordInput(value)
		m.inputHandler.respond(value, nil)
	}

	m.setStatus("Input submitted")
	return waitInputRequestCmd(m.inputHandler)
}

func (m *model) recordInput(value any) {
	if m.activePrompt == nil {
		return
	}
	if _, ok := m.savedInputs[m.activePrompt.meta.ID]; !ok {
		m.savedInputs[m.activePrompt.meta.ID] = make(map[string]any)
	}
	m.savedInputs[m.activePrompt.meta.ID][m.activePrompt.input.ID] = value
	if m.activePrompt.input.Kind == phases.InputKindSecret {
		m.trackSecretValue(value)
	}
	phases.SetInput(m.phaseCtx, m.activePrompt.meta.ID, m.activePrompt.input.ID, value)
}

func (m *model) handleEscape() tea.Cmd {
	if m.actionsVisible {
		m.actionsVisible = false
		return nil
	}
	if m.helpVisible {
		m.helpVisible = false
		return nil
	}
	if m.prompting {
		m.prompting = false
		if m.activePrompt != nil {
			m.inputHandler.respond("", errors.New("input cancelled"))
		}
		m.activePrompt = nil
		m.prompt.SetValue("")
		m.prompt.EchoMode = textinput.EchoNormal
		m.focus = focusPhases
		m.setStatus("Input cancelled")
		return waitInputRequestCmd(m.inputHandler)
	}
	return nil
}

func (m *model) toggleFocus() {
	if m.focus == focusPrompt {
		m.focus = focusPhases
	} else {
		m.focus = focusPrompt
	}
}

func (m *model) restartPipeline() tea.Cmd {
	if m.pipelineActive {
		m.setStatus("Pipeline already running")
		return nil
	}

	m.phaseCtx = phases.NewContext()
	for phaseID, inputs := range m.savedInputs {
		for inputID, value := range inputs {
			phases.SetInput(m.phaseCtx, phaseID, inputID, value)
		}
	}

	for _, id := range m.order {
		if state, ok := m.phases[id]; ok {
			state.status = statusPending
			state.err = nil
			state.logs = nil
		}
	}
	m.selectedPhase = 0
	m.done = nil
	m.setStatus("Restarting pipeline")
	return m.startPipeline()
}

func (m *model) retrySelectedPhase() tea.Cmd {
	if m.pipelineActive {
		m.setStatus("Pipeline already running")
		return nil
	}
	state := m.currentPhaseState()
	if state == nil {
		return nil
	}
	start := m.clampStartIndex(m.selectedPhase)
	for idx := start; idx < len(m.order); idx++ {
		id := m.order[idx]
		if st, ok := m.phases[id]; ok && st != nil {
			st.status = statusPending
			st.err = nil
			st.logs = nil
		}
	}
	m.done = nil
	m.setStatusf("Retrying from %s", state.meta.Title)
	return m.startPipelineFrom(start)
}

func (m *model) currentPhaseState() *phaseState {
	if len(m.order) == 0 {
		return nil
	}
	idx := m.clampStartIndex(m.selectedPhase)
	id := m.order[idx]
	return m.phases[id]
}

func (m *model) handleActionKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.actionsVisible = false
		return true, nil
	case tea.KeyEnter:
		m.actionsVisible = false
		return true, nil
	}
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		switch msg.Runes[0] {
		case '1', 'v', 'V':
			m.actionsVisible = false
			return true, nil
		case '2', 'r', 'R':
			if !m.pipelineActive {
				cmd := m.retrySelectedPhase()
				m.actionsVisible = false
				return true, cmd
			}
			m.setStatus("Cannot retry while pipeline is running")
			m.actionsVisible = false
			return true, nil
		case '3', 'c', 'C':
			m.copySelectedError()
			m.actionsVisible = false
			return true, nil
		}
	}
	return false, nil
}

func (m *model) copySelectedError() {
	state := m.currentPhaseState()
	if state == nil || state.err == nil {
		m.setStatus("No error to copy")
		return
	}
	if err := clipboard.WriteAll(state.err.Error()); err != nil {
		m.setStatus("Failed to copy error")
		return
	}
	m.setStatus("Error copied to clipboard")
}

func (m *model) handlePhaseNavigation(msg tea.KeyMsg) bool {
	if m.actionsVisible {
		return false
	}
	if m.prompting && m.focus != focusPhases {
		return false
	}
	switch msg.Type {
	case tea.KeyUp:
		m.movePhaseSelection(-1)
		return true
	case tea.KeyDown:
		m.movePhaseSelection(1)
		return true
	}
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		switch msg.Runes[0] {
		case 'k':
			m.movePhaseSelection(-1)
			return true
		case 'j':
			m.movePhaseSelection(1)
			return true
		}
	}
	return false
}

func (m *model) movePhaseSelection(delta int) {
	if len(m.order) == 0 {
		return
	}
	m.selectedPhase = (m.selectedPhase + delta) % len(m.order)
	if m.selectedPhase < 0 {
		m.selectedPhase += len(m.order)
	}
}

func (m *model) handleSelectPromptNavigation(msg tea.KeyMsg) bool {
	if !m.prompting || m.focus != focusPrompt || !m.isSelectPrompt() {
		return false
	}
	switch msg.Type {
	case tea.KeyUp:
		m.moveSelection(-1)
		return true
	case tea.KeyDown:
		m.moveSelection(1)
		return true
	}
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		switch msg.Runes[0] {
		case 'k':
			m.moveSelection(-1)
			return true
		case 'j':
			m.moveSelection(1)
			return true
		case '1', '2', '3', '4', '5', '6', '7', '8', '9':
			idx := int(msg.Runes[0] - '1')
			if options := m.activePrompt.input.Options; idx >= 0 && idx < len(options) {
				m.selectIndex = idx
				return true
			}
		}
	}
	return false
}

func (m *model) View() string {
	header := renderHeader(completedCount(m.phases), len(m.order))
	body := m.renderBody()
	promptPanel := m.renderPromptPanel()
	var actionsPanel string
	if m.actionsVisible {
		actionsPanel = m.renderActionsPanel()
	}
	statusBar := statusBarStyle.Render(m.statusMsg)
	footer := footerStyle.Render("↑/↓ or j/k move • Enter actions • Tab switch focus • r restart • ? help • Ctrl+C quit")

	sections := []string{header, body}
	if actionsPanel != "" {
		sections = append(sections, actionsPanel)
	}
	sections = append(sections, promptPanel, statusBar)

	if m.helpVisible {
		sections = append(sections, renderHelp())
	} else {
		sections = append(sections, footer)
	}

	view := lipgloss.JoinVertical(lipgloss.Left, sections...)
	renderWidth := m.terminalWidth()
	if renderWidth <= 0 {
		renderWidth = lipgloss.Width(view)
	}
	renderHeight := lipgloss.Height(view)
	if viewportHeight := m.viewportHeight(); viewportHeight > renderHeight {
		renderHeight = viewportHeight
	}
	return lipgloss.Place(renderWidth, renderHeight, lipgloss.Left, lipgloss.Top, view)
}

func renderHeader(done, total int) string {
	title := titleStyle.Render("Ansible Host Prep")
	progress := subtitleStyle.Render(fmt.Sprintf("Progress: %d/%d complete", done, total))
	return lipgloss.JoinHorizontal(lipgloss.Top, title, "  ", progress)
}

func (m *model) renderBody() string {
	width := m.viewportWidth()
	if width < 80 {
		list := m.renderPhaseList(width)
		detail := m.renderPhaseDetails(width)
		return lipgloss.JoinVertical(lipgloss.Left, list, detail)
	}
	left := width/2 - 1
	if left < 30 {
		left = 30
	}
	right := width - left - 2
	if right < 30 {
		right = 30
	}
	list := m.renderPhaseList(left)
	detail := m.renderPhaseDetails(right)
	gap := lipgloss.NewStyle().Width(2).Render(" ")
	return lipgloss.JoinHorizontal(lipgloss.Top, list, gap, detail)
}

func (m *model) renderPhaseList(width int) string {
	items := make([]string, 0, len(m.order))
	for idx, id := range m.order {
		state := m.phases[id]
		if state == nil {
			continue
		}
		selected := idx == m.selectedPhase
		items = append(items, phaseItemView(state, selected, m.focus == focusPhases && (!m.prompting || m.focus == focusPhases)))
	}
	content := strings.Join(items, "\n")
	style := styleForWidth(listPanelStyle, width)
	if m.focus == focusPhases && (!m.prompting || m.focus == focusPhases) {
		style = style.Copy().BorderForeground(activeBorderColor)
	}
	return style.Render(content)
}

func (m *model) renderPhaseDetails(width int) string {
	if len(m.order) == 0 {
		return styleForWidth(detailPanelStyle, width).Render("No phases registered")
	}
	if m.selectedPhase >= len(m.order) {
		m.selectedPhase = len(m.order) - 1
	}
	state := m.phases[m.order[m.selectedPhase]]
	if state == nil {
		return styleForWidth(detailPanelStyle, width).Render("No phase data")
	}

	title := detailTitleStyle.Render(state.meta.Title)
	description := infoTextStyle.Render(state.meta.Description)
	statusLine := infoTextStyle.Render(fmt.Sprintf("Status: %s", statusDisplay(state.status)))

	var errLine string
	if state.err != nil {
		errLine = errorTextStyle.Render(fmt.Sprintf("Error: %v", state.err))
	}

	logLines := ""
	if len(state.logs) > 0 {
		logLines = logSectionStyle.Render("Recent events:")
		entries := state.logs
		if len(entries) > 5 {
			entries = entries[len(entries)-5:]
		}
		for _, line := range entries {
			logLines += "\n" + logTextStyle.Render("• "+line)
		}
	}

	body := []string{title, description, statusLine}
	if errLine != "" {
		body = append(body, errLine)
	}
	if logLines != "" {
		body = append(body, logLines)
	}
	return styleForWidth(detailPanelStyle, width).Render(strings.Join(body, "\n"))
}

func (m *model) renderPromptPanel() string {
	style := styleForWidth(promptPanelStyle, m.viewportWidth())
	if m.prompting && m.focus == focusPrompt {
		style = style.Copy().BorderForeground(activeBorderColor)
	}

	if !m.prompting || m.activePrompt == nil {
		content := "No input requested\n"
		if m.pipelineActive {
			content = "Pipeline running…"
		}
		return style.Render("Prompt\n" + content)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Prompt — %s • %s\n", m.activePrompt.meta.Title, m.activePrompt.input.Label))
	b.WriteString(m.activePrompt.input.Description)
	b.WriteString("\n")
	if m.activePrompt.reason != "" {
		b.WriteString(infoTextStyle.Render(fmt.Sprintf("Reason: %s", m.activePrompt.reason)))
		b.WriteString("\n")
	}

	if m.isSelectPrompt() {
		b.WriteString("Use ↑/↓, j/k, number keys. Enter to confirm.\n\n")
		b.WriteString(m.renderSelectOptions())
	} else {
		b.WriteString("> ")
		b.WriteString(m.prompt.View())
	}

	return style.Render(b.String())
}

func (m *model) renderActionsPanel() string {
	state := m.currentPhaseState()
	if state == nil {
		return ""
	}
	options := []string{
		actionLine("1", "Close", true),
		actionLine("2", "Retry from this phase", !m.pipelineActive),
		actionLine("3", "Copy error message", state.err != nil),
	}
	header := fmt.Sprintf("Actions — %s", state.meta.Title)
	content := header + "\n" + strings.Join(options, "\n")
	return styleForWidth(actionsPanelStyle, m.viewportWidth()).Render(content)
}

func (m *model) renderSelectOptions() string {
	options := m.activePrompt.input.Options
	if len(options) == 0 {
		return "No options available"
	}
	lines := make([]string, 0, len(options))
	for idx, opt := range options {
		cursor := " "
		if idx == m.selectIndex {
			cursor = ">"
		}
		line := fmt.Sprintf("%d. %s", idx+1, opt.Label)
		if opt.Description != "" {
			line = fmt.Sprintf("%s — %s", line, opt.Description)
		}
		lines = append(lines, fmt.Sprintf("%s %s", cursor, line))
	}
	return strings.Join(lines, "\n")
}

func renderHelp() string {
	help := []string{
		"Key Bindings:",
		"  ↑/↓ or j/k  Move phase selection",
		"  Enter        Submit input / open phase actions",
		"  Tab          Switch focus between phases and prompt",
		"  r / Ctrl+R   Restart pipeline",
		"  Esc          Cancel prompt, hide help, or close actions",
		"  ?            Toggle this help",
		"  Ctrl+C       Quit",
	}
	return helpStyle.Render(strings.Join(help, "\n"))
}

func (m *model) isSelectPrompt() bool {
	return m.prompting && m.activePrompt != nil && m.activePrompt.input.Kind == phases.InputKindSelect
}

func (m *model) currentSelectionValue() (string, bool) {
	if !m.isSelectPrompt() {
		return "", false
	}
	options := m.activePrompt.input.Options
	if len(options) == 0 {
		return "", false
	}
	if m.selectIndex < 0 {
		m.selectIndex = 0
	}
	if m.selectIndex >= len(options) {
		m.selectIndex = len(options) - 1
	}
	return options[m.selectIndex].Value, true
}

func (m *model) moveSelection(delta int) {
	options := m.activePrompt.input.Options
	if len(options) == 0 {
		return
	}
	count := len(options)
	m.selectIndex = (m.selectIndex + delta) % count
	if m.selectIndex < 0 {
		m.selectIndex += count
	}
}

func (m *model) optionIndex(value string) int {
	if value == "" || m.activePrompt == nil {
		return -1
	}
	for idx, opt := range m.activePrompt.input.Options {
		if opt.Value == value {
			return idx
		}
	}
	return -1
}

func (m *model) lookupInputString(phaseID, inputID string) (string, bool) {
	if inputs, ok := m.savedInputs[phaseID]; ok {
		if val, ok := inputs[inputID]; ok {
			str := strings.TrimSpace(fmt.Sprint(val))
			if str != "" {
				return str, true
			}
		}
	}
	val, ok := phases.GetInput(m.phaseCtx, phaseID, inputID)
	if !ok {
		return "", false
	}
	str := strings.TrimSpace(fmt.Sprint(val))
	if str == "" {
		return "", false
	}
	return str, true
}

func statusLabel(s phaseStatus) string {
	switch s {
	case statusPending:
		return "pending"
	case statusRunning:
		return "running"
	case statusSuccess:
		return "success"
	case statusFailed:
		return "failed"
	default:
		return "unknown"
	}
}

func completedCount(states map[string]*phaseState) int {
	count := 0
	for _, st := range states {
		if st.status == statusSuccess {
			count++
		}
	}
	return count
}

func (m *model) appendLog(state *phaseState, line string) {
	if state == nil {
		return
	}
	line = m.redactSecrets(line)
	timestamp := time.Now().Format("15:04:05")
	state.logs = append(state.logs, fmt.Sprintf("[%s] %s", timestamp, line))
	if len(state.logs) > 20 {
		state.logs = state.logs[len(state.logs)-20:]
	}
}

var titleCase = cases.Title(language.English)

func statusDisplay(status phaseStatus) string {
	return titleCase.String(status.String())
}

func (m *model) viewportWidth() int {
	if m.width > 0 {
		if m.width < 40 {
			return 40
		}
		return m.width
	}
	return 100
}

func (m *model) viewportHeight() int {
	if m.height > 0 {
		return m.height
	}
	return 0
}

func (m *model) terminalWidth() int {
	if m.width > 0 {
		return m.width
	}
	return 0
}

func (m *model) trackSecretValue(value any) {
	if value == nil {
		return
	}
	str := strings.TrimSpace(fmt.Sprint(value))
	if str == "" || str == "<nil>" {
		return
	}
	m.secretValues[str] = struct{}{}
}

func (m *model) redactSecrets(text string) string {
	if text == "" || len(m.secretValues) == 0 {
		return text
	}
	redacted := text
	for secret := range m.secretValues {
		if secret == "" {
			continue
		}
		redacted = strings.ReplaceAll(redacted, secret, "[secret]")
	}
	return redacted
}

func (m *model) setStatus(msg string) {
	m.statusMsg = m.redactSecrets(msg)
}

func (m *model) setStatusf(format string, args ...any) {
	m.setStatus(fmt.Sprintf(format, args...))
}

func sanitizeInputReason(def phases.InputDefinition, reason string) string {
	if reason == "" {
		return ""
	}
	if def.Kind == phases.InputKindSecret {
		return "Previous entry was rejected; please provide a new value."
	}
	return reason
}

func styleForWidth(base lipgloss.Style, totalWidth int) lipgloss.Style {
	style := base.Copy()
	if totalWidth <= 0 {
		return style.Width(0)
	}
	frameWidth, _ := base.GetFrameSize()
	contentWidth := totalWidth - frameWidth
	if contentWidth < 0 {
		contentWidth = 0
	}
	return style.Width(contentWidth)
}

func placeholderText(def phases.InputDefinition, defaultValue string) string {
	if def.Kind == phases.InputKindSecret {
		return "enter value"
	}
	if defaultValue != "" {
		return defaultValue
	}
	return def.Label
}

func defaultString(value any) string {
	if value == nil {
		return ""
	}
	str := strings.TrimSpace(fmt.Sprint(value))
	if str == "" || str == "<nil>" {
		return ""
	}
	return str
}

func actionLine(key, label string, enabled bool) string {
	line := fmt.Sprintf("[%s] %s", key, label)
	if enabled {
		return infoTextStyle.Render(line)
	}
	return disabledTextStyle.Render(line + " (unavailable)")
}

// ---- Styling helpers ----

var (
	titleStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E0AAFF"))
	subtitleStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8"))
	listPanelStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#4C566A")).Padding(0, 1)
	detailPanelStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#4C566A")).Padding(0, 1)
	promptPanelStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#4C566A")).Padding(0, 1).MarginTop(1)
	actionsPanelStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#7C3AED")).Padding(0, 1).MarginTop(1)
	statusBarStyle    = lipgloss.NewStyle().Bold(true).Padding(0, 1).Background(lipgloss.Color("#312E81")).Foreground(lipgloss.Color("#E0E7FF"))
	footerStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8")).Padding(0, 1).MarginTop(1)
	helpStyle         = lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(lipgloss.Color("#7C3AED")).Padding(1, 2).MarginTop(1)
	detailTitleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FDE047"))
	infoTextStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#CBD5F5"))
	errorTextStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171"))
	disabledTextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#475569"))
	logSectionStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#A5B4FC")).Bold(true)
	logTextStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#E0E7FF"))
	activeBorderColor = lipgloss.Color("#A78BFA")
)

var statusStyles = map[phaseStatus]lipgloss.Style{
	statusPending: lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8")),
	statusRunning: lipgloss.NewStyle().Foreground(lipgloss.Color("#F97316")).Bold(true),
	statusSuccess: lipgloss.NewStyle().Foreground(lipgloss.Color("#34D399")),
	statusFailed:  lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171")),
}

func phaseItemView(state *phaseState, selected bool, focused bool) string {
	icon := map[phaseStatus]string{
		statusPending: "•",
		statusRunning: "⟳",
		statusSuccess: "✔",
		statusFailed:  "✖",
	}[state.status]

	label := fmt.Sprintf("%s %s", icon, state.meta.Title)
	if state.status == statusRunning {
		label = fmt.Sprintf("%s %s", spinnerStyle.Render("⟳"), state.meta.Title)
	}
	if state.err != nil {
		label = fmt.Sprintf("%s — %v", label, state.err)
	}

	style := statusStyles[state.status]
	if selected {
		style = style.Copy().Bold(true)
		if focused {
			style = style.Copy().Underline(true).Foreground(activeBorderColor)
		}
	}
	return style.Render(label)
}

var spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24"))

// ---- Phase orchestration events ----

type phaseStartedMsg struct {
	meta phases.PhaseMetadata
}

type phaseCompletedMsg struct {
	meta phases.PhaseMetadata
	err  error
}

type phasesFinishedMsg struct {
	err error
}

type inputRequestMsg struct {
	meta   phases.PhaseMetadata
	input  phases.InputDefinition
	reason string
}

// ---- Observer & input handler plumbing ----

type phaseObserver struct {
	events chan tea.Msg
}

func newPhaseObserver() *phaseObserver {
	return &phaseObserver{
		events: make(chan tea.Msg),
	}
}

func (o *phaseObserver) PhaseStarted(meta phases.PhaseMetadata) {
	o.events <- phaseStartedMsg{meta: meta}
}

func (o *phaseObserver) PhaseCompleted(meta phases.PhaseMetadata, err error) {
	o.events <- phaseCompletedMsg{meta: meta, err: err}
}

func waitPhaseEventCmd(observer *phaseObserver) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-observer.events
		if !ok {
			return nil
		}
		return msg
	}
}

type inputRequest struct {
	meta   phases.PhaseMetadata
	def    phases.InputDefinition
	reason string
}

type inputResponse struct {
	value any
	err   error
}

type bubbleInputHandler struct {
	requests  chan inputRequest
	responses chan inputResponse
}

func newBubbleInputHandler() *bubbleInputHandler {
	return &bubbleInputHandler{
		requests:  make(chan inputRequest),
		responses: make(chan inputResponse),
	}
}

func (h *bubbleInputHandler) RequestInput(meta phases.PhaseMetadata, input phases.InputDefinition, reason string) (any, error) {
	h.requests <- inputRequest{meta: meta, def: input, reason: reason}
	resp := <-h.responses
	return resp.value, resp.err
}

func (h *bubbleInputHandler) respond(value any, err error) {
	h.responses <- inputResponse{value: value, err: err}
}

func waitInputRequestCmd(handler *bubbleInputHandler) tea.Cmd {
	return func() tea.Msg {
		req, ok := <-handler.requests
		if !ok {
			return nil
		}
		return inputRequestMsg{
			meta:   req.meta,
			input:  req.def,
			reason: req.reason,
		}
	}
}

func runManagerCmd(runCtx context.Context, manager *phases.Manager, ctx *phases.Context, start int) tea.Cmd {
	return func() tea.Msg {
		if runCtx == nil {
			runCtx = context.Background()
		}
		err := manager.RunFrom(runCtx, ctx, start)
		return phasesFinishedMsg{err: err}
	}
}
