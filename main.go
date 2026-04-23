package main

import (
	"fmt"
	"os"
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

// ─── App Model ───────────────────────────────────────────────────────────────

type Model struct {
	quadrants   [4]Quadrant
	focusedQuad int
	width       int
	height      int
	mode        string // "normal", "add", "edit"
	textInput   textinput.Model
	dragging    bool
	dragQuad    int
	dragIdx     int
	dragX       int
	dragY       int
}

// ─── Styles ──────────────────────────────────────────────────────────────────

var (
	subtle    = lipgloss.AdaptiveColor{Light: "#555555", Dark: "#777777"}
	highlight = lipgloss.AdaptiveColor{Light: "#333333", Dark: "#EEEEEE"}
	blue      = lipgloss.Color("#61AFEF")
	yellow    = lipgloss.Color("#E5C07B")
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

	dragStyle = lipgloss.NewStyle().
			Foreground(yellow).
			Bold(true)
)

// ─── Initialization ──────────────────────────────────────────────────────────

func initialModel() Model {
	ti := textinput.New()
	ti.Placeholder = "Task | Date (optional)"
	ti.CharLimit = 120
	ti.Width = 40

	return Model{
		quadrants: [4]Quadrant{
			{
				Header: "IMP BUT NOT URGENT -> SCHEDULE (MOST IMP)",
				Tasks: []Task{
					{Text: "3b1b Image Video Gen Lecture"},
					{Text: "3b1b Linear Algebra"},
					{Text: "Micrograd Project"},
					{Text: "Six Easy Pieces Reading"},
				},
			},
			{
				Header: "IMPORTANT & URGENT - FOCUS",
				Tasks: []Task{
					{Text: "Maths 2 Multivar calculus"},
					{Text: "Mindmap of Calculus"},
				},
			},
			{
				Header: "NOT IMP or URGENT -> DELETE",
				Tasks:  []Task{},
			},
			{
				Header: "URGENT BUT NOT IMP -> BATCH + DELAY",
				Tasks: []Task{
					{Text: "Gradient Theory", Date: "Apr 14"},
					{Text: "Project based learning"},
					{Text: "Get a Floss"},
				},
			},
		},
		focusedQuad: 0,
		mode:        "normal",
		textInput:   ti,
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func (m Model) quadWidth() int {
	w := m.width/2 - 4
	if w < 8 {
		w = 8
	}
	return w
}

func (m Model) quadHeight() int {
	h := m.height/2 - 2
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

func (m *Model) moveTask(fromQuad, fromIdx, toQuad int) {
	if fromQuad == toQuad {
		return
	}
	if fromIdx < 0 || fromIdx >= len(m.quadrants[fromQuad].Tasks) {
		return
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
	m.ensureScrollVisible(fromQuad)
	m.ensureScrollVisible(toQuad)
}

func (m *Model) deleteTask(qIdx, tIdx int) {
	if tIdx < 0 || tIdx >= len(m.quadrants[qIdx].Tasks) {
		return
	}
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
}

func parseTaskInput(input string) (text, date string) {
	parts := strings.SplitN(input, " | ", 2)
	text = strings.TrimSpace(parts[0])
	if len(parts) > 1 {
		date = strings.TrimSpace(parts[1])
	}
	return
}

func (m *Model) addTask(qIdx int, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	taskText, date := parseTaskInput(text)
	if taskText == "" {
		return
	}
	m.quadrants[qIdx].Tasks = append(m.quadrants[qIdx].Tasks, Task{Text: taskText, Date: date})
	m.quadrants[qIdx].SelectedIdx = len(m.quadrants[qIdx].Tasks) - 1
	m.ensureScrollVisible(qIdx)
}

func (m *Model) updateTask(qIdx, tIdx int, text string) {
	if tIdx < 0 || tIdx >= len(m.quadrants[qIdx].Tasks) {
		return
	}
	taskText, date := parseTaskInput(text)
	if taskText == "" {
		return
	}
	m.quadrants[qIdx].Tasks[tIdx].Text = taskText
	m.quadrants[qIdx].Tasks[tIdx].Date = date
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
		m.textInput.Width = min(50, m.width-10)
		for i := range m.quadrants {
			m.ensureScrollVisible(i)
		}

	case tea.KeyMsg:
		switch m.mode {
		case "add", "edit":
			switch msg.Type {
			case tea.KeyEsc:
				m.mode = "normal"
				m.textInput.SetValue("")
				return m, nil
			case tea.KeyEnter:
				val := m.textInput.Value()
				if m.mode == "add" {
					m.addTask(m.focusedQuad, val)
				} else {
					m.updateTask(m.focusedQuad, m.quadrants[m.focusedQuad].SelectedIdx, val)
				}
				m.mode = "normal"
				m.textInput.SetValue("")
				return m, nil
			}
			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
			return m, cmd

		default: // normal mode
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
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
					m.mode = "edit"
				} else {
					m.textInput.SetValue("")
					m.mode = "add"
				}
				m.textInput.Focus()
				return m, textinput.Blink
			case "d", "delete":
				m.deleteTask(m.focusedQuad, m.quadrants[m.focusedQuad].SelectedIdx)
			case " ":
				q := &m.quadrants[m.focusedQuad]
				if q.SelectedIdx >= 0 && q.SelectedIdx < len(q.Tasks) {
					q.Tasks[q.SelectedIdx].Completed = !q.Tasks[q.SelectedIdx].Completed
				}
			}
		}

	case tea.MouseMsg:
		if m.mode != "normal" {
			break
		}
		switch msg.Action {
		case tea.MouseActionPress:
			if msg.Button == tea.MouseButtonLeft {
				q := quadFromXY(msg.X, msg.Y, m.width, m.height)
				if q != m.focusedQuad {
					m.focusedQuad = q
					m.ensureScrollVisible(q)
				}

				quadY := 0
				if q >= 2 {
					quadY = m.height / 2
				}
				relY := msg.Y - quadY - 1
				if relY > 0 {
					idx := relY - 1 + m.quadrants[q].ScrollOff
					maxIdx := len(m.quadrants[q].Tasks)
					if idx >= 0 && idx <= maxIdx {
						m.quadrants[q].SelectedIdx = idx
						m.ensureScrollVisible(q)
					}
				}

				if m.quadrants[q].SelectedIdx < len(m.quadrants[q].Tasks) {
					m.dragging = true
					m.dragQuad = q
					m.dragIdx = m.quadrants[q].SelectedIdx
					m.dragX = msg.X
					m.dragY = msg.Y
				}
			}
		case tea.MouseActionRelease:
			if m.dragging && msg.Button == tea.MouseButtonLeft {
				targetQ := quadFromXY(msg.X, msg.Y, m.width, m.height)
				m.moveTask(m.dragQuad, m.dragIdx, targetQ)
				if targetQ != m.focusedQuad {
					m.focusedQuad = targetQ
				}
				m.dragging = false
			}
		case tea.MouseActionMotion:
			if m.dragging {
				m.dragX = msg.X
				m.dragY = msg.Y
			}
		}
	}

	if m.mode == "add" || m.mode == "edit" {
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
		header := headerStyle.Render(truncateText(q.Header, qw-2))

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
			tasks = append(tasks, placeholderStyle.Render("  Tap to add task"))
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

	if m.dragging {
		taskText := m.quadrants[m.dragQuad].Tasks[m.dragIdx].Text
		indicator := dragStyle.Render("↻  " + truncateText(taskText, m.width-6))
		grid = lipgloss.JoinVertical(lipgloss.Left, grid, indicator)
	}

	if m.mode == "add" || m.mode == "edit" {
		label := "New Task"
		if m.mode == "edit" {
			label = "Edit Task"
		}
		hint := lipgloss.NewStyle().Faint(true).MarginTop(1).Render("Use  |  to add a date")
		modalContent := lipgloss.JoinVertical(lipgloss.Center,
			lipgloss.NewStyle().Bold(true).MarginBottom(1).Render(label),
			m.textInput.View(),
			hint,
		)
		modal := modalStyle.Render(modalContent)
		grid = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
	}

	return grid
}

func min(a, b int) int {
	if a < b {
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
