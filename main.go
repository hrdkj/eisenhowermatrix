package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ─── Data Models ─────────────────────────────────────────────────────────────

type Task struct {
	Text      string
	Date      string
	Completed bool
}

type Quadrant struct {
	Header      string
	Tasks       []Task
	SelectedIdx int
	ScrollOff   int
}

type AppState struct {
	Quadrants [4][]Task `json:"quadrants"`
}

const (
	modeNormal    = "normal"
	modeAdd       = "add"
	modeEdit      = "edit"
	stateFileName = "tasks.json"
)

// ─── App Model ───────────────────────────────────────────────────────────────

type Model struct {
	quadrants   [4]Quadrant
	focusedQuad int
	width       int
	height      int
	mode        string
	textInput   textinput.Model
	statePath   string
	status      string
}

// ─── Styles ──────────────────────────────────────────────────────────────────

var (
	subtle    = lipgloss.AdaptiveColor{Light: "#555555", Dark: "#777777"}
	highlight = lipgloss.AdaptiveColor{Light: "#333333", Dark: "#EEEEEE"}
	blue      = lipgloss.Color("#61AFEF")
	faint     = lipgloss.AdaptiveColor{Light: "#AAAAAA", Dark: "#555555"}

	focusedStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(highlight).
			Padding(0, 1)

	unfocusedStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(subtle).
			Padding(0, 1)

	selectedTaskStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#3E4451")).
				Foreground(lipgloss.Color("#FFFFFF"))

	normalTaskStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ABB2BF"))

	completedTaskStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#555555"))

	dateStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#61AFEF")).
			Faint(true)

	placeholderStyle = lipgloss.NewStyle().
				Faint(true).
				Foreground(faint)

	modalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(blue).
			Padding(1, 2)
)

// ─── Initialization ──────────────────────────────────────────────────────────

func defaultQuadrants() [4]Quadrant {
	return [4]Quadrant{
		{
			Header: "IMPORTANT, NOT URGENT -> SCHEDULE",
			Tasks: []Task{
				{Text: "3b1b Image Video Gen Lecture"},
				{Text: "3b1b Linear Algebra"},
				{Text: "Micrograd Project"},
				{Text: "Six Easy Pieces Reading"},
			},
		},
		{
			Header: "IMPORTANT, URGENT -> DO NOW",
			Tasks: []Task{
				{Text: "Maths 2 Multivar calculus"},
				{Text: "Mindmap of Calculus"},
			},
		},
		{
			Header: "NOT IMPORTANT, NOT URGENT -> DELETE",
		},
		{
			Header: "URGENT, NOT IMPORTANT -> BATCH / DELAY",
			Tasks: []Task{
				{Text: "Gradient Theory", Date: "Apr 14"},
				{Text: "Project based learning"},
				{Text: "Get a Floss"},
			},
		},
	}
}

func initialModel() Model {
	ti := textinput.New()
	ti.Placeholder = "Task | Date (optional)"
	ti.CharLimit = 120
	ti.Width = 40

	model := Model{
		quadrants:   defaultQuadrants(),
		focusedQuad: 0,
		mode:        modeNormal,
		textInput:   ti,
		status:      "Tasks auto-save locally",
	}

	statePath, err := stateFilePath()
	if err != nil {
		model.status = "Autosave unavailable: " + err.Error()
		return model
	}

	model.statePath = statePath
	loaded, err := model.loadState()
	if err != nil {
		model.status = "Load failed: " + err.Error()
	} else if loaded {
		model.status = "Loaded saved tasks"
	}

	return model
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func (m Model) quadWidth() int {
	w := m.width/2 - 4
	if w < 8 {
		w = 8
	}
	return w
}

func (m Model) footerHeight() int {
	return 2
}

func (m Model) quadHeight() int {
	usableHeight := max(0, m.height-m.footerHeight())
	h := usableHeight/2 - 2
	if h < 4 {
		h = 4
	}
	return h
}

func (m Model) visibleTasksCount() int {
	h := m.quadHeight() - 1 // subtract header line
	if h < 1 {
		return 1
	}
	return h
}

func (m *Model) ensureScrollVisible(qIdx int) {
	q := &m.quadrants[qIdx]
	vis := m.visibleTasksCount()

	if q.SelectedIdx < 0 {
		q.SelectedIdx = 0
	}
	maxIdx := len(q.Tasks)
	if q.SelectedIdx > maxIdx {
		q.SelectedIdx = maxIdx
	}
	if len(q.Tasks) == 0 {
		q.ScrollOff = 0
		return
	}
	if q.SelectedIdx < q.ScrollOff {
		q.ScrollOff = q.SelectedIdx
	}
	if q.SelectedIdx >= q.ScrollOff+vis {
		q.ScrollOff = q.SelectedIdx - vis + 1
	}
}

func stateFilePath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err == nil && configDir != "" {
		return filepath.Join(configDir, "eisenhowermatrix", stateFileName), nil
	}

	wd, wdErr := os.Getwd()
	if wdErr != nil {
		if err != nil {
			return "", err
		}
		return "", wdErr
	}

	return filepath.Join(wd, "."+stateFileName), nil
}

func snapshotState(quadrants [4]Quadrant) AppState {
	var state AppState
	for i := range quadrants {
		state.Quadrants[i] = append([]Task(nil), quadrants[i].Tasks...)
	}
	return state
}

func applyState(quadrants *[4]Quadrant, state AppState) {
	for i := range quadrants {
		quadrants[i].Tasks = append([]Task(nil), state.Quadrants[i]...)
		quadrants[i].SelectedIdx = 0
		quadrants[i].ScrollOff = 0
	}
}

func loadStateFile(path string) (AppState, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return AppState{}, false, nil
		}
		return AppState{}, false, err
	}

	var state AppState
	if err := json.Unmarshal(data, &state); err != nil {
		return AppState{}, false, err
	}

	return state, true, nil
}

func saveStateFile(path string, state AppState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func (m *Model) loadState() (bool, error) {
	if m.statePath == "" {
		return false, nil
	}

	state, loaded, err := loadStateFile(m.statePath)
	if err != nil || !loaded {
		return loaded, err
	}

	applyState(&m.quadrants, state)
	for i := range m.quadrants {
		m.ensureScrollVisible(i)
	}

	return true, nil
}

func (m *Model) persist(successStatus string) {
	if m.statePath == "" {
		return
	}
	if err := saveStateFile(m.statePath, snapshotState(m.quadrants)); err != nil {
		m.status = "Save failed: " + err.Error()
		return
	}
	m.status = successStatus
}

func (m *Model) moveTask(fromQuad, fromIdx, toQuad int) bool {
	if fromQuad == toQuad {
		return false
	}
	if fromIdx < 0 || fromIdx >= len(m.quadrants[fromQuad].Tasks) {
		return false
	}
	task := m.quadrants[fromQuad].Tasks[fromIdx]
	m.quadrants[fromQuad].Tasks = append(
		m.quadrants[fromQuad].Tasks[:fromIdx],
		m.quadrants[fromQuad].Tasks[fromIdx+1:]...,
	)
	m.quadrants[toQuad].Tasks = append(m.quadrants[toQuad].Tasks, task)

	if m.quadrants[fromQuad].SelectedIdx >= len(m.quadrants[fromQuad].Tasks) {
		m.quadrants[fromQuad].SelectedIdx = len(m.quadrants[fromQuad].Tasks) - 1
	}
	if m.quadrants[fromQuad].SelectedIdx < 0 {
		m.quadrants[fromQuad].SelectedIdx = 0
	}
	m.quadrants[toQuad].SelectedIdx = len(m.quadrants[toQuad].Tasks) - 1
	m.focusedQuad = toQuad
	m.ensureScrollVisible(fromQuad)
	m.ensureScrollVisible(toQuad)
	m.persist(fmt.Sprintf("Moved %q to quadrant %d", task.Text, toQuad+1))
	return true
}

func (m *Model) deleteTask(qIdx, tIdx int) bool {
	if tIdx < 0 || tIdx >= len(m.quadrants[qIdx].Tasks) {
		return false
	}
	taskText := m.quadrants[qIdx].Tasks[tIdx].Text
	m.quadrants[qIdx].Tasks = append(
		m.quadrants[qIdx].Tasks[:tIdx],
		m.quadrants[qIdx].Tasks[tIdx+1:]...,
	)
	if m.quadrants[qIdx].SelectedIdx >= len(m.quadrants[qIdx].Tasks) {
		m.quadrants[qIdx].SelectedIdx = len(m.quadrants[qIdx].Tasks) - 1
	}
	if m.quadrants[qIdx].SelectedIdx < 0 {
		m.quadrants[qIdx].SelectedIdx = 0
	}
	m.ensureScrollVisible(qIdx)
	m.persist(fmt.Sprintf("Deleted %q", taskText))
	return true
}

func parseTaskInput(input string) (text, date string) {
	text, date, found := strings.Cut(input, "|")
	text = strings.TrimSpace(text)
	if found {
		date = strings.TrimSpace(date)
	}
	return
}

func (m *Model) addTask(qIdx int, text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	taskText, date := parseTaskInput(text)
	if taskText == "" {
		return false
	}
	m.quadrants[qIdx].Tasks = append(m.quadrants[qIdx].Tasks, Task{Text: taskText, Date: date})
	m.quadrants[qIdx].SelectedIdx = len(m.quadrants[qIdx].Tasks) - 1
	m.ensureScrollVisible(qIdx)
	m.persist(fmt.Sprintf("Added %q", taskText))
	return true
}

func (m *Model) updateTask(qIdx, tIdx int, text string) bool {
	if tIdx < 0 || tIdx >= len(m.quadrants[qIdx].Tasks) {
		return false
	}
	taskText, date := parseTaskInput(text)
	if taskText == "" {
		return false
	}
	m.quadrants[qIdx].Tasks[tIdx].Text = taskText
	m.quadrants[qIdx].Tasks[tIdx].Date = date
	m.persist(fmt.Sprintf("Updated %q", taskText))
	return true
}

func (m *Model) toggleTask(qIdx, tIdx int) bool {
	if tIdx < 0 || tIdx >= len(m.quadrants[qIdx].Tasks) {
		return false
	}
	task := &m.quadrants[qIdx].Tasks[tIdx]
	task.Completed = !task.Completed
	if task.Completed {
		m.persist(fmt.Sprintf("Completed %q", task.Text))
	} else {
		m.persist(fmt.Sprintf("Reopened %q", task.Text))
	}
	return true
}

func quadFromXY(x, y, w, h int) int {
	halfW := w / 2
	halfH := h / 2
	if x < halfW && y < halfH {
		return 0
	}
	if x >= halfW && y < halfH {
		return 1
	}
	if x < halfW && y >= halfH {
		return 2
	}
	return 3
}

func truncateText(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	for lipgloss.Width(string(runes)) > maxLen-1 && len(runes) > 0 {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}

// ─── Bubble Tea Interface ────────────────────────────────────────────────────

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textInput.Width = max(20, min(50, m.width-10))
		for i := range m.quadrants {
			m.ensureScrollVisible(i)
		}

	case tea.KeyMsg:
		switch m.mode {
		case modeAdd, modeEdit:
			switch msg.Type {
			case tea.KeyEsc:
				m.mode = modeNormal
				m.textInput.SetValue("")
				m.textInput.Blur()
				m.status = "Cancelled editor"
				return m, nil
			case tea.KeyEnter:
				val := m.textInput.Value()
				var ok bool
				if m.mode == modeAdd {
					ok = m.addTask(m.focusedQuad, val)
				} else {
					ok = m.updateTask(m.focusedQuad, m.quadrants[m.focusedQuad].SelectedIdx, val)
				}
				if !ok {
					m.status = "Task text cannot be empty"
					return m, nil
				}
				m.mode = modeNormal
				m.textInput.SetValue("")
				m.textInput.Blur()
				return m, nil
			}
			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
			return m, cmd

		default:
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "a", "n":
				m.textInput.SetValue("")
				m.textInput.Focus()
				m.mode = modeAdd
				m.status = "Adding a task"
				return m, textinput.Blink
			case "tab":
				m.focusedQuad = (m.focusedQuad + 1) % 4
				m.ensureScrollVisible(m.focusedQuad)
			case "shift+tab":
				m.focusedQuad = (m.focusedQuad + 3) % 4
				m.ensureScrollVisible(m.focusedQuad)
			case "up", "k":
				if m.quadrants[m.focusedQuad].SelectedIdx > 0 {
					m.quadrants[m.focusedQuad].SelectedIdx--
				}
				m.ensureScrollVisible(m.focusedQuad)
			case "down", "j":
				maxIdx := len(m.quadrants[m.focusedQuad].Tasks)
				if m.quadrants[m.focusedQuad].SelectedIdx < maxIdx {
					m.quadrants[m.focusedQuad].SelectedIdx++
				}
				m.ensureScrollVisible(m.focusedQuad)
			case "left", "h":
				m.focusedQuad = [4]int{2, 0, 3, 1}[m.focusedQuad]
				m.ensureScrollVisible(m.focusedQuad)
			case "right", "l":
				m.focusedQuad = [4]int{1, 3, 0, 2}[m.focusedQuad]
				m.ensureScrollVisible(m.focusedQuad)
			case "enter":
				q := &m.quadrants[m.focusedQuad]
				if q.SelectedIdx >= 0 && q.SelectedIdx < len(q.Tasks) {
					val := q.Tasks[q.SelectedIdx].Text
					if q.Tasks[q.SelectedIdx].Date != "" {
						val += " | " + q.Tasks[q.SelectedIdx].Date
					}
					m.textInput.SetValue(val)
					m.mode = modeEdit
					m.status = "Editing selected task"
				} else {
					m.textInput.SetValue("")
					m.mode = modeAdd
					m.status = "Adding a task"
				}
				m.textInput.Focus()
				return m, textinput.Blink
			case "d", "delete":
				if !m.deleteTask(m.focusedQuad, m.quadrants[m.focusedQuad].SelectedIdx) {
					m.status = "No task selected to delete"
				}
			case " ":
				if !m.toggleTask(m.focusedQuad, m.quadrants[m.focusedQuad].SelectedIdx) {
					m.status = "No task selected to toggle"
				}
			case "1", "2", "3", "4":
				targetQuad := int(msg.String()[0] - '1')
				if !m.moveTask(m.focusedQuad, m.quadrants[m.focusedQuad].SelectedIdx, targetQuad) {
					if targetQuad == m.focusedQuad {
						m.status = fmt.Sprintf("Already in quadrant %d", targetQuad+1)
					} else {
						m.status = "No task selected to move"
					}
				}
			}
		}

	case tea.MouseMsg:
		if m.mode != modeNormal {
			break
		}
		if msg.Y >= m.height-m.footerHeight() {
			break
		}
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			q := quadFromXY(msg.X, msg.Y, m.width, m.height-m.footerHeight())
			if q != m.focusedQuad {
				m.focusedQuad = q
				m.ensureScrollVisible(q)
			}
		}
	}

	if m.mode == modeAdd || m.mode == modeEdit {
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	quads := make([]string, 4)
	qw := m.quadWidth()
	qh := m.quadHeight()
	vis := m.visibleTasksCount()

	for i := 0; i < 4; i++ {
		q := m.quadrants[i]
		style := unfocusedStyle
		if i == m.focusedQuad {
			style = focusedStyle
		}
		style = style.Width(qw).Height(qh)

		headerStyle := lipgloss.NewStyle().Bold(true).Foreground(highlight)
		header := headerStyle.Render(truncateText(fmt.Sprintf("%d. %s", i+1, q.Header), qw-2))

		var tasks []string
		start := q.ScrollOff
		end := start + vis
		if end > len(q.Tasks) {
			end = len(q.Tasks)
		}

		for j := start; j < end; j++ {
			task := q.Tasks[j]
			selected := j == q.SelectedIdx && i == m.focusedQuad

			checkbox := "[ ]"
			if task.Completed {
				checkbox = "[x]"
			}

			cursor := "  "
			if selected {
				cursor = "▸ "
			}

			dateRendered := ""
			if task.Date != "" {
				dateRendered = dateStyle.Render("· " + task.Date)
			}

			maxTextWidth := qw - 4 - lipgloss.Width(cursor) - 4 - lipgloss.Width(dateRendered)
			if maxTextWidth < 3 {
				maxTextWidth = 3
			}
			text := truncateText(task.Text, maxTextWidth)

			line := cursor + checkbox + " " + text
			if dateRendered != "" {
				line += " " + dateRendered
			}

			var rendered string
			if selected {
				rendered = selectedTaskStyle.Render(line)
			} else if task.Completed {
				rendered = completedTaskStyle.Strikethrough(true).Render(line)
			} else {
				rendered = normalTaskStyle.Render(line)
			}
			tasks = append(tasks, rendered)
		}

		if len(q.Tasks) == 0 {
			tasks = append(tasks, placeholderStyle.Render("  Press a or Enter to add"))
		} else if i == m.focusedQuad && q.SelectedIdx == len(q.Tasks) && len(tasks) < vis {
			tasks = append(tasks, placeholderStyle.Render("  ── add task ──"))
		}

		for len(tasks) < vis {
			tasks = append(tasks, " ")
		}

		content := lipgloss.JoinVertical(lipgloss.Left, append([]string{header}, tasks...)...)
		quads[i] = style.Render(content)
	}

	topRow := lipgloss.JoinHorizontal(lipgloss.Top, quads[0], quads[1])
	bottomRow := lipgloss.JoinHorizontal(lipgloss.Top, quads[2], quads[3])
	grid := lipgloss.JoinVertical(lipgloss.Left, topRow, bottomRow)

	if m.mode == modeAdd || m.mode == modeEdit {
		label := "New Task"
		if m.mode == modeEdit {
			label = "Edit Task"
		}
		hint := lipgloss.NewStyle().Faint(true).MarginTop(1).Render("Use | to add a date")
		modalContent := lipgloss.JoinVertical(lipgloss.Center,
			lipgloss.NewStyle().Bold(true).MarginBottom(1).Render(label),
			m.textInput.View(),
			hint,
		)
		modal := modalStyle.Render(modalContent)
		grid = lipgloss.Place(m.width, max(0, m.height-m.footerHeight()), lipgloss.Center, lipgloss.Center, modal)
	}

	help := placeholderStyle.Render(truncateText("Tab/Shift+Tab or h/j/k/l focus • Enter edit/add • a add • Space done • 1-4 move • d delete • q quit", m.width))
	status := dateStyle.Render(truncateText(m.status, m.width))

	return lipgloss.JoinVertical(lipgloss.Left, grid, help, status)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ─── Main ────────────────────────────────────────────────────────────────────

func main() {
	p := tea.NewProgram(
		initialModel(),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}
