package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	textinput "github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/BrianJOC/prep-for-ansible/phases"
	"github.com/BrianJOC/prep-for-ansible/phases/ansibleuser"
	"github.com/BrianJOC/prep-for-ansible/phases/pythonensure"
	"github.com/BrianJOC/prep-for-ansible/phases/sshconnect"
	"github.com/BrianJOC/prep-for-ansible/phases/sudoensure"
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

type phaseState struct {
	meta   phases.PhaseMetadata
	status phaseStatus
	err    error
}

type model struct {
	manager      *phases.Manager
	phaseCtx     *phases.Context
	observer     *phaseObserver
	inputHandler *bubbleInputHandler

	phases map[string]*phaseState
	order  []string

	prompt       textinput.Model
	activePrompt *inputRequestMsg
	prompting    bool
	statusMsg    string

	done error
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
		states[meta.ID] = &phaseState{
			meta:   meta,
			status: statusPending,
		}
		order = append(order, meta.ID)
	}

	ti := textinput.New()
	ti.Placeholder = "enter value"
	ti.Focus()

	return &model{
		manager:      manager,
		phaseCtx:     phaseCtx,
		observer:     observer,
		inputHandler: inputHandler,
		phases:       states,
		order:        order,
		prompt:       ti,
	}
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(
		runManagerCmd(m.manager, m.phaseCtx),
		waitPhaseEventCmd(m.observer),
		waitInputRequestCmd(m.inputHandler),
	)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			if m.prompting {
				m.inputHandler.respond("", errors.New("input cancelled"))
			}
			return m, tea.Quit
		case tea.KeyEnter:
			if m.prompting {
				value := strings.TrimSpace(m.prompt.Value())
				if value == "" && m.activePrompt != nil && m.activePrompt.input.Required {
					m.statusMsg = "Input required"
					return m, nil
				}
				m.inputHandler.respond(value, nil)
				m.prompting = false
				m.activePrompt = nil
				m.prompt.SetValue("")
				m.statusMsg = "Submitted input"
				return m, waitInputRequestCmd(m.inputHandler)
			}
		}
		if m.prompting {
			var cmd tea.Cmd
			m.prompt, cmd = m.prompt.Update(msg)
			return m, cmd
		}
	case phaseStartedMsg:
		if state, ok := m.phases[msg.meta.ID]; ok {
			state.status = statusRunning
			state.err = nil
		}
		return m, waitPhaseEventCmd(m.observer)
	case phaseCompletedMsg:
		if state, ok := m.phases[msg.meta.ID]; ok {
			if msg.err != nil {
				state.status = statusFailed
				state.err = msg.err
			} else {
				state.status = statusSuccess
				state.err = nil
			}
		}
		return m, waitPhaseEventCmd(m.observer)
	case inputRequestMsg:
		m.activePrompt = &msg
		m.prompting = true
		m.prompt.Placeholder = msg.input.Label
		m.prompt.SetValue("")
		m.prompt.Focus()
		m.statusMsg = fmt.Sprintf("Phase %s requests %s", msg.meta.Title, msg.input.Label)
		return m, nil
	case phasesFinishedMsg:
		m.done = msg.err
		if msg.err != nil {
			m.statusMsg = msg.err.Error()
		} else {
			m.statusMsg = "All phases completed"
		}
		return m, nil
	}

	if m.prompting {
		var cmd tea.Cmd
		m.prompt, cmd = m.prompt.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *model) View() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Prep for Ansible\n\n")
	for _, id := range m.order {
		state := m.phases[id]
		fmt.Fprintf(&b, "[%s] %s\n", statusLabel(state.status), state.meta.Title)
		if state.err != nil {
			fmt.Fprintf(&b, "  error: %v\n", state.err)
		} else if state.status == statusRunning {
			fmt.Fprintf(&b, "  %s\n", state.meta.Description)
		}
	}
	fmt.Fprintf(&b, "\nStatus: %s\n", m.statusMsg)
	if m.prompting && m.activePrompt != nil {
		fmt.Fprintf(&b, "\n%s\n", m.activePrompt.input.Description)
		if m.activePrompt.reason != "" {
			fmt.Fprintf(&b, "(reason: %s)\n", m.activePrompt.reason)
		}
		fmt.Fprintf(&b, "> %s\n", m.prompt.View())
	}
	if m.done != nil {
		fmt.Fprintf(&b, "\nPress Ctrl+C to exit.\n")
	} else if allFinished(m.phases) {
		fmt.Fprintf(&b, "\nAll phases complete! Press Ctrl+C to exit.\n")
	}
	return b.String()
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

func allFinished(states map[string]*phaseState) bool {
	for _, st := range states {
		if st.status != statusSuccess && st.status != statusFailed {
			return false
		}
	}
	return true
}

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
