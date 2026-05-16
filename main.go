package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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
	modeSearch    = "search"
	stateFileName = "tasks.json"
	maxHistory    = 100
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

	undoStack []AppState
	redoStack []AppState

	sortByDate bool
	filterWeek bool

	searchQuery string
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

	overdueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E06C75")).
			Bold(true)

	dueThisWeekStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#E5C07B"))
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
	if m.mode == modeSearch {
		return 3
	}
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

	if m.isFilterActive() {
		visList := m.visibleIndices(qIdx)
		if len(visList) == 0 {
			q.ScrollOff = 0
			return
		}
		pos := -1
		for i, idx := range visList {
			if idx == q.SelectedIdx {
				pos = i
				break
			}
		}
		if pos == -1 {
			if len(visList) > 0 {
				q.SelectedIdx = visList[0]
			}
			q.ScrollOff = 0
			return
		}
		if pos < q.ScrollOff {
			q.ScrollOff = pos
		}
		if pos >= q.ScrollOff+vis {
			q.ScrollOff = pos - vis + 1
		}
		return
	}

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
	m.pushUndo()
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
	m.pushUndo()
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

func tryParseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	switch strings.ToLower(s) {
	case "today":
		return today, true
	case "tomorrow":
		return today.AddDate(0, 0, 1), true
	case "yesterday":
		return today.AddDate(0, 0, -1), true
	}

	formats := []string{
		"Jan 2",
		"Jan 2, 2006",
		"2 Jan",
		"2 Jan 2006",
		"2006-01-02",
		"01/02/2006",
		"01/02/06",
		"January 2",
		"January 2, 2006",
		"2 January",
		"2 January 2006",
	}

	for _, f := range formats {
		t, err := time.Parse(f, s)
		if err != nil {
			continue
		}
		if t.Year() == 0 {
			t = time.Date(now.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		}
		return t, true
	}

	return time.Time{}, false
}

func isOverdue(task Task) bool {
	if task.Completed || task.Date == "" {
		return false
	}
	t, ok := tryParseDate(task.Date)
	if !ok {
		return false
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return t.Before(today)
}

func isDueThisWeek(task Task) bool {
	if task.Date == "" {
		return false
	}
	t, ok := tryParseDate(task.Date)
	if !ok {
		return false
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	endOfWeek := today.AddDate(0, 0, 8)
	return !t.Before(today) && t.Before(endOfWeek)
}

func (m Model) isFilterActive() bool {
	return (m.mode == modeSearch && m.searchQuery != "") || m.filterWeek || m.sortByDate
}

func (m Model) visibleIndices(qIdx int) []int {
	tasks := m.quadrants[qIdx].Tasks
	result := make([]int, 0, len(tasks))
	for i, t := range tasks {
		if m.mode == modeSearch && m.searchQuery != "" {
			if !strings.Contains(strings.ToLower(t.Text), strings.ToLower(m.searchQuery)) {
				continue
			}
		}
		if m.filterWeek && !isDueThisWeek(t) {
			continue
		}
		result = append(result, i)
	}
	if m.sortByDate && m.mode != modeSearch {
		sort.SliceStable(result, func(i, j int) bool {
			a := result[i]
			b := result[j]
			ta, oka := tryParseDate(tasks[a].Date)
			tb, okb := tryParseDate(tasks[b].Date)
			if oka && okb {
				return ta.Before(tb)
			}
			if oka {
				return true
			}
			return false
		})
	}
	return result
}

func (m *Model) addTask(qIdx int, text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	taskText, date := parseTaskInput(text)
	if taskText == "" {
		return false
	}
	m.pushUndo()
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
	m.pushUndo()
	m.quadrants[qIdx].Tasks[tIdx].Text = taskText
	m.quadrants[qIdx].Tasks[tIdx].Date = date
	m.persist(fmt.Sprintf("Updated %q", taskText))
	return true
}

func (m *Model) toggleTask(qIdx, tIdx int) bool {
	if tIdx < 0 || tIdx >= len(m.quadrants[qIdx].Tasks) {
		return false
	}
	m.pushUndo()
	task := &m.quadrants[qIdx].Tasks[tIdx]
	task.Completed = !task.Completed
	if task.Completed {
		m.persist(fmt.Sprintf("Completed %q", task.Text))
	} else {
		m.persist(fmt.Sprintf("Reopened %q", task.Text))
	}
	return true
}

func (m *Model) pushUndo() {
	if len(m.undoStack) >= maxHistory {
		m.undoStack = m.undoStack[1:]
	}
	m.undoStack = append(m.undoStack, snapshotState(m.quadrants))
	m.redoStack = nil
}

func (m *Model) undo() {
	if len(m.undoStack) == 0 {
		m.status = "Nothing to undo"
		return
	}
	m.redoStack = append(m.redoStack, snapshotState(m.quadrants))
	state := m.undoStack[len(m.undoStack)-1]
	m.undoStack = m.undoStack[:len(m.undoStack)-1]
	applyState(&m.quadrants, state)
	for i := range m.quadrants {
		m.ensureScrollVisible(i)
	}
	m.persist("Undo")
}

func (m *Model) redo() {
	if len(m.redoStack) == 0 {
		m.status = "Nothing to redo"
		return
	}
	m.undoStack = append(m.undoStack, snapshotState(m.quadrants))
	state := m.redoStack[len(m.redoStack)-1]
	m.redoStack = m.redoStack[:len(m.redoStack)-1]
	applyState(&m.quadrants, state)
	for i := range m.quadrants {
		m.ensureScrollVisible(i)
	}
	m.persist("Redo")
}

func (m *Model) swapTaskUp(qIdx, tIdx int) bool {
	if tIdx <= 0 || tIdx >= len(m.quadrants[qIdx].Tasks) {
		return false
	}
	m.pushUndo()
	tasks := m.quadrants[qIdx].Tasks
	tasks[tIdx], tasks[tIdx-1] = tasks[tIdx-1], tasks[tIdx]
	m.quadrants[qIdx].SelectedIdx = tIdx - 1
	m.ensureScrollVisible(qIdx)
	m.persist("Moved task up")
	return true
}

func (m *Model) swapTaskDown(qIdx, tIdx int) bool {
	if tIdx < 0 || tIdx >= len(m.quadrants[qIdx].Tasks)-1 {
		return false
	}
	m.pushUndo()
	tasks := m.quadrants[qIdx].Tasks
	tasks[tIdx], tasks[tIdx+1] = tasks[tIdx+1], tasks[tIdx]
	m.quadrants[qIdx].SelectedIdx = tIdx + 1
	m.ensureScrollVisible(qIdx)
	m.persist("Moved task down")
	return true
}

func (m *Model) moveSelectionUp() {
	q := &m.quadrants[m.focusedQuad]
	if m.isFilterActive() {
		vis := m.visibleIndices(m.focusedQuad)
		if len(vis) == 0 {
			return
		}
		pos := -1
		for i, idx := range vis {
			if idx == q.SelectedIdx {
				pos = i
				break
			}
		}
		if pos > 0 {
			q.SelectedIdx = vis[pos-1]
		}
	} else {
		if q.SelectedIdx > 0 {
			q.SelectedIdx--
		}
	}
	m.ensureScrollVisible(m.focusedQuad)
}

func (m *Model) moveSelectionDown() {
	q := &m.quadrants[m.focusedQuad]
	if m.isFilterActive() {
		vis := m.visibleIndices(m.focusedQuad)
		if len(vis) == 0 {
			return
		}
		pos := -1
		for i, idx := range vis {
			if idx == q.SelectedIdx {
				pos = i
				break
			}
		}
		if pos == -1 {
			q.SelectedIdx = vis[0]
		} else if pos < len(vis)-1 {
			q.SelectedIdx = vis[pos+1]
		}
	} else {
		maxIdx := len(q.Tasks)
		if q.SelectedIdx < maxIdx {
			q.SelectedIdx++
		}
	}
	m.ensureScrollVisible(m.focusedQuad)
}

func (m *Model) sortAllByDate() {
	for i := range m.quadrants {
		selectedIdx := m.quadrants[i].SelectedIdx
		var selectedTask Task
		hasSelection := selectedIdx >= 0 && selectedIdx < len(m.quadrants[i].Tasks)
		if hasSelection {
			selectedTask = m.quadrants[i].Tasks[selectedIdx]
		}

		tasks := m.quadrants[i].Tasks
		sort.SliceStable(tasks, func(a, b int) bool {
			ta, oka := tryParseDate(tasks[a].Date)
			tb, okb := tryParseDate(tasks[b].Date)
			if oka && okb {
				return ta.Before(tb)
			}
			if oka {
				return true
			}
			return false
		})

		if hasSelection {
			for j, t := range tasks {
				if t.Text == selectedTask.Text && t.Date == selectedTask.Date {
					m.quadrants[i].SelectedIdx = j
					break
				}
			}
		}
		m.quadrants[i].ScrollOff = 0
	}
	m.persist("Sorted by date")
}

func (m *Model) maxSelectedIdx(qIdx int) int {
	if m.isFilterActive() {
		vis := m.visibleIndices(qIdx)
		return len(vis)
	}
	return len(m.quadrants[qIdx].Tasks)
}

func (m *Model) searchStatus() string {
	q := m.searchQuery
	if q == "" {
		return "Search mode - type to filter"
	}
	return "Search: \"" + q + "\""
}

func (m *Model) handleSearchOperation(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case " ":
		if !m.toggleTask(m.focusedQuad, m.quadrants[m.focusedQuad].SelectedIdx) {
			m.status = "No task selected to toggle"
		}
	case "d", "delete":
		if !m.deleteTask(m.focusedQuad, m.quadrants[m.focusedQuad].SelectedIdx) {
			m.status = "No task selected to delete"
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
	case "a", "n":
		m.textInput.SetValue("")
		m.textInput.Focus()
		m.mode = modeAdd
		m.searchQuery = ""
		m.status = "Adding a task"
		return textinput.Blink
	case "tab":
		m.focusedQuad = (m.focusedQuad + 1) % 4
		m.ensureScrollVisible(m.focusedQuad)
	case "shift+tab":
		m.focusedQuad = (m.focusedQuad + 3) % 4
		m.ensureScrollVisible(m.focusedQuad)
	case "ctrl+z":
		m.undo()
	case "ctrl+y":
		m.redo()
	case "ctrl+j":
		if m.sortByDate {
			m.status = "Cannot reorder when sorted by date"
		} else {
			m.swapTaskDown(m.focusedQuad, m.quadrants[m.focusedQuad].SelectedIdx)
		}
	case "ctrl+k":
		if m.sortByDate {
			m.status = "Cannot reorder when sorted by date"
		} else {
			m.swapTaskUp(m.focusedQuad, m.quadrants[m.focusedQuad].SelectedIdx)
		}
	}
	return nil
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

		case modeSearch:
			switch msg.String() {
			case "esc":
				m.mode = modeNormal
				m.searchQuery = ""
				for i := range m.quadrants {
					m.quadrants[i].ScrollOff = 0
				}
				m.status = "Search cancelled"
				return m, nil
			case "enter":
				m.mode = modeNormal
				m.status = "Search done"
				return m, nil
			case "backspace":
				if len(m.searchQuery) > 0 {
					m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
					for i := range m.quadrants {
						m.quadrants[i].SelectedIdx = 0
						m.quadrants[i].ScrollOff = 0
					}
					m.status = m.searchStatus()
				}
			case "up", "k":
				m.moveSelectionUp()
			case "down", "j":
				m.moveSelectionDown()
			case " ", "d", "delete", "1", "2", "3", "4",
				"tab", "shift+tab", "a", "n",
				"ctrl+z", "ctrl+y", "ctrl+j", "ctrl+k":
				if cmd := m.handleSearchOperation(msg); cmd != nil {
					return m, cmd
				}
			default:
				if len(msg.Runes) == 1 && msg.Runes[0] >= 32 {
					m.searchQuery += string(msg.Runes[0])
					for i := range m.quadrants {
						m.quadrants[i].SelectedIdx = 0
						m.quadrants[i].ScrollOff = 0
					}
					m.status = m.searchStatus()
				}
			}
			return m, nil

		default:
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "ctrl+z":
				m.undo()
			case "ctrl+y":
				m.redo()
			case "ctrl+j":
				if m.sortByDate {
					m.status = "Cannot reorder when sorted by date"
				} else if !m.swapTaskDown(m.focusedQuad, m.quadrants[m.focusedQuad].SelectedIdx) {
					m.status = "Cannot move task further down"
				}
			case "ctrl+k":
				if m.sortByDate {
					m.status = "Cannot reorder when sorted by date"
				} else if !m.swapTaskUp(m.focusedQuad, m.quadrants[m.focusedQuad].SelectedIdx) {
					m.status = "Cannot move task further up"
				}
			case "/":
				m.mode = modeSearch
				m.searchQuery = ""
				for i := range m.quadrants {
					m.quadrants[i].ScrollOff = 0
				}
				m.status = "Search mode - type to filter, Enter/Esc to exit"
				return m, nil
			case "s":
				m.sortByDate = !m.sortByDate
				if m.sortByDate {
					m.sortAllByDate()
					m.status = "Sorted by date (press s to unsort)"
				} else {
					m.status = "Natural order"
				}
			case "w":
				m.filterWeek = !m.filterWeek
				for i := range m.quadrants {
					m.quadrants[i].SelectedIdx = 0
					m.quadrants[i].ScrollOff = 0
				}
				if m.filterWeek {
					m.status = "Week filter on - showing tasks due within 7 days"
				} else {
					m.status = "Week filter off"
				}
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
				m.moveSelectionUp()
			case "down", "j":
				m.moveSelectionDown()
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
		if m.mode != modeNormal && m.mode != modeSearch {
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
	visCount := m.visibleTasksCount()
	filterActive := m.isFilterActive()

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

		if filterActive {
			visList := m.visibleIndices(i)
			start := q.ScrollOff
			end := start + visCount
			if end > len(visList) {
				end = len(visList)
			}
			for _, actualIdx := range visList[start:end] {
				task := q.Tasks[actualIdx]
				selected := actualIdx == q.SelectedIdx && i == m.focusedQuad
				tasks = append(tasks, m.renderTaskLine(task, selected, qw))
			}
			if len(visList) == 0 {
				if m.mode == modeSearch {
					tasks = append(tasks, placeholderStyle.Render("  No matches"))
				} else if len(q.Tasks) == 0 {
					tasks = append(tasks, placeholderStyle.Render("  Press a or Enter to add"))
				} else {
					tasks = append(tasks, placeholderStyle.Render("  No tasks visible"))
				}
			}
			for len(tasks) < visCount {
				tasks = append(tasks, " ")
			}
		} else {
			start := q.ScrollOff
			end := start + visCount
			if end > len(q.Tasks) {
				end = len(q.Tasks)
			}
			for j := start; j < end; j++ {
				task := q.Tasks[j]
				selected := j == q.SelectedIdx && i == m.focusedQuad
				tasks = append(tasks, m.renderTaskLine(task, selected, qw))
			}
			if len(q.Tasks) == 0 {
				tasks = append(tasks, placeholderStyle.Render("  Press a or Enter to add"))
			} else if i == m.focusedQuad && q.SelectedIdx == len(q.Tasks) && len(tasks) < visCount {
				tasks = append(tasks, placeholderStyle.Render("  ── add task ──"))
			}
			for len(tasks) < visCount {
				tasks = append(tasks, " ")
			}
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

	if m.mode == modeSearch {
		searchBar := lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(blue).
			Render(lipgloss.NewStyle().Bold(true).Foreground(blue).Render("/ ") + m.searchQuery + "█")
		grid = lipgloss.JoinVertical(lipgloss.Left, grid, searchBar)
	}

	indicators := " "
	if m.sortByDate {
		indicators += "[S:sort] "
	}
	if m.filterWeek {
		indicators += "[W:week] "
	}

	helpLine := indicators +
		"h/j/k/l nav • a add • Enter edit • Space done • 1-4 move • d del • / search • s sort • w week • Ctrl+Z undo • Ctrl+K/J reorder • q quit"
	help := placeholderStyle.Render(truncateText(helpLine, m.width))
	status := dateStyle.Render(truncateText(m.status, m.width))

	return lipgloss.JoinVertical(lipgloss.Left, grid, help, status)
}

func (m Model) renderTaskLine(task Task, selected bool, qw int) string {
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
		if isOverdue(task) {
			dateRendered = overdueStyle.Render("· " + task.Date)
		} else if isDueThisWeek(task) {
			dateRendered = dueThisWeekStyle.Render("· " + task.Date)
		} else {
			dateRendered = dateStyle.Render("· " + task.Date)
		}
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

	var lineStyle lipgloss.Style
	if selected {
		lineStyle = selectedTaskStyle
	} else if task.Completed {
		return completedTaskStyle.Strikethrough(true).Render(line)
	} else if isOverdue(task) {
		lineStyle = overdueStyle
	} else {
		lineStyle = normalTaskStyle
	}

	return lineStyle.Render(line)
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
