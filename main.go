/*
author: bashbunni, Evann Regnault
licence: MIT
*/
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/charm/kv"
	"github.com/charmbracelet/lipgloss"
)

/* GLOBAL */
type status int

const divisor = 4

const (
	todo status = iota
	inProgress
	done
)

/* STYLING */
var (
	columnStyle = lipgloss.NewStyle().
			Padding(1, 2).
			Border(lipgloss.HiddenBorder())
	focusedStyle = lipgloss.NewStyle().
			Padding(1, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62"))
	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
)

/* HELP BAR */
type keyMap struct {
	Up     key.Binding
	Down   key.Binding
	Left   key.Binding
	Right  key.Binding
	New    key.Binding
	Delete key.Binding
	Quit   key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Left, k.Down, k.New, k.Delete, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Up, k.Down, k.Left, k.Down}, {k.New, k.Delete, k.Quit}}
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "move up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "move down"),
	),
	Left: key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("←/h", "move left"),
	),
	Right: key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("→/l", "move right"),
	),
	New: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "create task"),
	),
	Delete: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "delete task"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "esc"),
		key.WithHelp("q/esc", "quit"),
	),
}

type helpModel struct {
	keys       keyMap
	help       help.Model
	inputStyle lipgloss.Style
}

func newHelpModel() helpModel {
	return helpModel{
		keys:       keys,
		help:       help.New(),
		inputStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("#FF75B7")),
	}
}

func (m helpModel) Init() tea.Cmd {
	return nil
}

func (m helpModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.help.ShowAll = true
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.help.Width = msg.Width
	}
	return m, nil
}

func (m helpModel) View() string {
	return helpStyle.Render(m.help.View(keys))
}

/* MODEL MANAGEMENT */
var models []tea.Model

const (
	model status = iota
	form
)

// Task /* CUSTOM ITEM */
type Task struct {
	Estatus      status `json:"status"`
	Etitle       string `json:"title"`
	Edescription string `json:"description"`
}

func NewTask(status status, title, description string) Task {
	return Task{Estatus: status, Etitle: title, Edescription: description}
}

func (t *Task) Next() {
	if t.Estatus == done {
		t.Estatus = todo
	} else {
		t.Estatus++
	}
}

// implement the list.Item interface

func (t Task) FilterValue() string {
	return t.Etitle
}

func (t Task) Title() string {
	return t.Etitle
}

func (t Task) Description() string {
	return t.Edescription
}

/* MAIN MODEL */

type Model struct {
	database *kv.KV
	loaded   bool
	focused  status
	lists    []list.Model
	quitting bool
}

func New() *Model {
	return &Model{}
}

func (m *Model) ConvertToTaskList() [][]Task {
	taskList := [][]Task{{}, {}, {}}
	for status, tl := range m.lists {
		for _, t := range tl.Items() {
			taskList[status] = append(taskList[status], t.(Task))
		}
	}
	return taskList
}

func (m *Model) ImportTaskList(tasks [][]Task) {
	for status, tl := range tasks {
		m.lists[status].SetItems([]list.Item{})
		for i, task := range tl {
			m.lists[status].InsertItem(i, task)
		}
	}
}

func (m *Model) MoveToNext() tea.Msg {
	selectedItem := m.lists[m.focused].SelectedItem()
	if selectedItem == nil {
		return nil
	}
	selectedTask := selectedItem.(Task)

	m.lists[selectedTask.Estatus].RemoveItem(m.lists[m.focused].Index())
	selectedTask.Next()
	m.lists[selectedTask.Estatus].InsertItem(len(m.lists[selectedTask.Estatus].Items())-1, list.Item(selectedTask))
	return nil
}

func (m *Model) DeleteTask() tea.Msg {
	selectedItem := m.lists[m.focused].SelectedItem()
	if selectedItem == nil {
		return nil
	}
	selectedTask := selectedItem.(Task)
	m.lists[selectedTask.Estatus].RemoveItem(m.lists[m.focused].Index())
	return nil
}

func (m *Model) Next() {
	if m.focused == done {

		m.focused = todo
	} else {
		m.focused++
	}
}

func (m *Model) Prev() {
	if m.focused == todo {
		m.focused = done
	} else {
		m.focused--
	}
}

// DB RELATED
func (m *Model) initDB() {
	var err error
	m.database, err = kv.OpenWithDefaults("kancli")
	if err != nil {
		panic(err)
	}
	keys, _ := m.database.Keys()
	for _, keyName := range keys {
		if string(keyName) == "lists" {
			return
		}
	}
	m.updateDB()
}

func (m *Model) updateDB() {
	byteTasks, _ := json.Marshal(m.ConvertToTaskList())
	err := m.database.Set([]byte("lists"), byteTasks)
	if err != nil {
		panic(err)
	}
}

func (m *Model) getListsFromDB() {
	byteTasks, _ := m.database.Get([]byte("lists"))
	var remoteTasks [][]Task
	err := json.Unmarshal(byteTasks, &remoteTasks)
	if err != nil {
		panic(err)
	}
	m.ImportTaskList(remoteTasks)
}

// LIST RELATED
func (m *Model) initLists(width, height int) {
	defaultList := list.New([]list.Item{}, list.NewDefaultDelegate(), width/divisor, height/2)
	defaultList.SetShowHelp(false)
	m.lists = []list.Model{defaultList, defaultList, defaultList}
	m.initDB()
	m.getListsFromDB()
}

// Init MODEL RELATED
func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if !m.loaded {
			columnStyle.Width(msg.Width / divisor)
			focusedStyle.Width(msg.Width / divisor)
			columnStyle.Height(msg.Height - divisor + 1)
			focusedStyle.Height(msg.Height - divisor + 1)
			m.initLists(msg.Width, msg.Height)
			m.loaded = true
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			m.updateDB()
			err := m.database.Close()
			if err != nil {
				panic(err)
			}
			m.quitting = true
			return m, tea.Quit
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "left", "h":
			m.Prev()
		case "right", "l":
			m.Next()
		case "enter":
			return m, m.MoveToNext
		case "d", "backspace":
			return m, m.DeleteTask
		case "n":
			m.updateDB()
			models[model] = m // save the state of the current model
			models[form] = NewForm(m.focused)
			return models[form].Update(nil)
		}
	case Task:
		task := msg
		return m, m.lists[task.Estatus].InsertItem(len(m.lists[task.Estatus].Items()), task)
	}
	var cmd tea.Cmd
	m.lists[m.focused], cmd = m.lists[m.focused].Update(msg)
	return m, cmd
}

func (m Model) View() string {

	if m.quitting {
		return ""
	}
	if m.loaded {
		enc, err := json.Marshal(m.ConvertToTaskList())
		if err != nil {
			panic(err)
		}
		var test [][]Task
		err = json.Unmarshal(enc, &test)
		if err != nil {
			panic(err)
		}

		todoView := m.lists[todo].View()
		inProgView := m.lists[inProgress].View()
		doneView := m.lists[done].View()
		var lg string
		switch m.focused {
		case inProgress:
			lg = lipgloss.JoinHorizontal(
				lipgloss.Center,
				columnStyle.Render(todoView),
				focusedStyle.Render(inProgView),
				columnStyle.Render(doneView),
			)
		case done:
			lg = lipgloss.JoinHorizontal(
				lipgloss.Center,
				columnStyle.Render(todoView),
				columnStyle.Render(inProgView),
				focusedStyle.Render(doneView),
			)
		default:
			lg = lipgloss.JoinHorizontal(
				lipgloss.Center,
				focusedStyle.Render(todoView),
				columnStyle.Render(inProgView),
				columnStyle.Render(doneView),
			)
		}
		return lipgloss.JoinVertical(
			lipgloss.Bottom,
			lg,
			newHelpModel().View(),
		)
	} else {
		return "loading..."
	}
}

// Form /* FORM MODEL */
type Form struct {
	quitting    bool
	focused     status
	title       textinput.Model
	description textarea.Model
}

func NewForm(focused status) *Form {
	form := &Form{focused: focused}
	form.title = textinput.New()
	form.title.Focus()
	form.description = textarea.New()
	return form
}

func (m Form) CreateTask() tea.Msg {
	task := NewTask(m.focused, m.title.Value(), m.description.Value())
	return task
}

func (m Form) Init() tea.Cmd {
	return nil
}

func (m Form) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			err := models[model].(Model).database.Close()
			if err != nil {
				panic(err)
			}
			m.quitting = true
			return m, tea.Quit
		case "enter":
			if m.title.Focused() {
				m.title.Blur()
				m.description.Focus()
				return m, textarea.Blink
			} else {
				models[form] = m
				return models[model], m.CreateTask
			}
		}
	}
	if m.title.Focused() {
		m.title, cmd = m.title.Update(msg)
		return m, cmd
	} else {
		m.description, cmd = m.description.Update(msg)
		return m, cmd
	}
}

func (m Form) View() string {
	if m.quitting {
		return ""
	}
	return lipgloss.JoinVertical(lipgloss.Left, m.title.View(), m.description.View())
}

// MAIN
func main() {
	models = []tea.Model{New(), NewForm(todo)}
	m := models[model]
	p := tea.NewProgram(m)
	if err := p.Start(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
