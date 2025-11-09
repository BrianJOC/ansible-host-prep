package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	textinput "github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/BrianJOC/ansible-host-prep/phases"
	"github.com/BrianJOC/ansible-host-prep/phases/ansibleuser"
	"github.com/BrianJOC/ansible-host-prep/phases/pythonensure"
	"github.com/BrianJOC/ansible-host-prep/phases/sshconnect"
	"github.com/BrianJOC/ansible-host-prep/phases/sudoensure"
)

func main() {
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		log.Fatalf("tui exited with error: %v", err)
	}
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

	phases map[string]*phaseState
	order  []string

	spinner spinner.Model

	prompt       textinput.Model
	activePrompt *inputRequestMsg
	prompting    bool
	selectIndex  int

	savedInputs map[string]map[string]any

	selectedPhase  int
	focus          focusArea
	helpVisible    bool
	pipelineActive bool

	statusMsg string
	done      error
}

func newModel() *model {
	phaseCtx := phases.NewContext()
	inputHandler := newBubbleInputHandler()
	observer := newPhaseObserver()

	manager := phases.NewManager(
		phases.WithObserver(observer),
		phases.WithInputHandler(inputHandler),
	)

	phaseList := []phases.Phase{
		sshconnect.New(),
		sudoensure.New(),
		pythonensure.New(),
		ansibleuser.New(),
	}

	if err := manager.Register(phaseList...); err != nil {
		log.Fatalf("failed to register phases: %v", err)
	}

	states := make(map[string]*phaseState, len(phaseList))
	order := make([]string, 0, len(phaseList))
	for _, ph := range phaseList {
		meta := ph.Metadata()
		states[meta.ID] = &phaseState{meta: meta, status: statusPending}
		order = append(order, meta.ID)
	}

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	ti := textinput.New()
	ti.Placeholder = "enter value"
	ti.Blur()

	return &model{
		manager:        manager,
		phaseCtx:       phaseCtx,
		observer:       observer,
		inputHandler:   inputHandler,
		phases:         states,
		order:          order,
		spinner:        sp,
		prompt:         ti,
		focus:          focusPhases,
		selectedPhase:  0,
		savedInputs:    make(map[string]map[string]any),
		statusMsg:      "Awaiting phase events…",
		pipelineActive: false,
	}
}

func (m *model) Init() tea.Cmd {
	return m.startPipeline()
}

func (m *model) startPipeline() tea.Cmd {
	m.pipelineActive = true
	return tea.Batch(
		runManagerCmd(m.manager, m.phaseCtx),
		waitPhaseEventCmd(m.observer),
		waitInputRequestCmd(m.inputHandler),
		m.spinner.Tick,
	)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
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
			m.statusMsg = msg.err.Error()
		} else {
			m.statusMsg = "All phases completed"
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
	m.statusMsg = fmt.Sprintf("Running %s", msg.meta.Title)
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
		m.statusMsg = fmt.Sprintf("%s failed — %v", msg.meta.Title, msg.err)
	} else {
		state.status = statusSuccess
		state.err = nil
		m.appendLog(state, fmt.Sprintf("%s completed", msg.meta.Title))
		m.statusMsg = fmt.Sprintf("%s completed", msg.meta.Title)
	}
}

func (m *model) preparePrompt(msg inputRequestMsg) {
	m.activePrompt = &msg
	m.prompting = true
	m.focus = focusPrompt
	m.helpVisible = false
	m.selectIndex = 0

	prevVal, _ := m.lookupInputString(msg.meta.ID, msg.input.ID)

	switch msg.input.Kind {
	case phases.InputKindSelect:
		if idx := m.optionIndex(prevVal); idx >= 0 {
			m.selectIndex = idx
		}
		m.prompt.Blur()
		if len(msg.input.Options) == 0 {
			m.statusMsg = fmt.Sprintf("%s requested %s but no options available", msg.meta.Title, msg.input.Label)
		} else {
			m.statusMsg = fmt.Sprintf("%s: choose %s (arrows, j/k, numbers)", msg.meta.Title, msg.input.Label)
		}
	default:
		m.prompt.Placeholder = msg.input.Label
		if msg.input.Kind != phases.InputKindSecret && prevVal != "" {
			m.prompt.SetValue(prevVal)
		} else {
			m.prompt.SetValue("")
		}
		m.prompt.CursorEnd()
		m.prompt.Focus()
		m.statusMsg = fmt.Sprintf("%s needs %s", msg.meta.Title, msg.input.Label)
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
		m.focus = focusPhases
	}()

	if m.isSelectPrompt() {
		value, ok := m.currentSelectionValue()
		if !ok {
			m.statusMsg = "No options available"
			return nil
		}
		m.recordInput(value)
		m.inputHandler.respond(value, nil)
	} else {
		value := strings.TrimSpace(m.prompt.Value())
		if value == "" && m.activePrompt.input.Required {
			m.statusMsg = "Input required"
			return nil
		}
		m.recordInput(value)
		m.inputHandler.respond(value, nil)
	}

	m.statusMsg = "Input submitted"
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
	phases.SetInput(m.phaseCtx, m.activePrompt.meta.ID, m.activePrompt.input.ID, value)
}

func (m *model) handleEscape() tea.Cmd {
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
		m.focus = focusPhases
		m.statusMsg = "Input cancelled"
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
	m.statusMsg = "Restarting pipeline"
	return m.startPipeline()
}

func (m *model) handlePhaseNavigation(msg tea.KeyMsg) bool {
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
	statusBar := statusBarStyle.Render(m.statusMsg)
	footer := footerStyle.Render("↑/↓ or j/k move • Enter submit • Tab switch focus • r restart • ? help • Ctrl+C quit")

	if m.helpVisible {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			header,
			body,
			promptPanel,
			statusBar,
			renderHelp(),
		)
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		body,
		promptPanel,
		statusBar,
		footer,
	)
}

func renderHeader(done, total int) string {
	title := titleStyle.Render("Ansible Host Prep")
	progress := subtitleStyle.Render(fmt.Sprintf("Progress: %d/%d complete", done, total))
	return lipgloss.JoinHorizontal(lipgloss.Top, title, "  ", progress)
}

func (m *model) renderBody() string {
	phaseList := m.renderPhaseList()
	details := m.renderPhaseDetails()
	return lipgloss.JoinHorizontal(lipgloss.Top, phaseList, details)
}

func (m *model) renderPhaseList() string {
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
	style := listPanelStyle
	if m.focus == focusPhases && (!m.prompting || m.focus == focusPhases) {
		style = style.Copy().BorderForeground(activeBorderColor)
	}
	return style.Render(content)
}

func (m *model) renderPhaseDetails() string {
	if len(m.order) == 0 {
		return detailPanelStyle.Render("No phases registered")
	}
	if m.selectedPhase >= len(m.order) {
		m.selectedPhase = len(m.order) - 1
	}
	state := m.phases[m.order[m.selectedPhase]]
	if state == nil {
		return detailPanelStyle.Render("No phase data")
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
	return detailPanelStyle.Render(strings.Join(body, "\n"))
}

func (m *model) renderPromptPanel() string {
	style := promptPanelStyle
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
		"  Enter        Submit input / confirm selection",
		"  Tab          Switch focus between phases and prompt",
		"  r / Ctrl+R   Restart pipeline",
		"  Esc          Cancel prompt or hide help",
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

// ---- Styling helpers ----

var (
	titleStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E0AAFF"))
	subtitleStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8"))
	listPanelStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#4C566A")).Padding(0, 1).Width(38)
	detailPanelStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#4C566A")).Padding(0, 1).Width(60).MarginLeft(2)
	promptPanelStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#4C566A")).Padding(0, 1).MarginTop(1)
	statusBarStyle    = lipgloss.NewStyle().Bold(true).Padding(0, 1).Background(lipgloss.Color("#312E81")).Foreground(lipgloss.Color("#E0E7FF"))
	footerStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8")).Padding(0, 1).MarginTop(1)
	helpStyle         = lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(lipgloss.Color("#7C3AED")).Padding(1, 2).MarginTop(1)
	detailTitleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FDE047"))
	infoTextStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#CBD5F5"))
	errorTextStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171"))
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

func runManagerCmd(manager *phases.Manager, ctx *phases.Context) tea.Cmd {
	return func() tea.Msg {
		err := manager.Run(context.Background(), ctx)
		return phasesFinishedMsg{err: err}
	}
}
