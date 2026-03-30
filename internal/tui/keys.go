package tui

import "github.com/charmbracelet/bubbles/key"

// Global key bindings shared across views.
type globalKeyMap struct {
	Quit key.Binding
	Back key.Binding
	Help key.Binding
}

var globalKeys = globalKeyMap{
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	Back: key.NewBinding(
		key.WithKeys("u"),
		key.WithHelp("u", "back"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
}

// Dashboard key bindings.
type dashboardKeyMap struct {
	Enter      key.Binding
	Create     key.Binding
	Edit       key.Binding
	Transition key.Binding
	Start      key.Binding
	Close      key.Binding
	Grab       key.Binding
	Refresh    key.Binding
	Open       key.Binding
	Epic       key.Binding
	BackToMine key.Binding
	ToggleAll  key.Binding
}

var dashboardKeys = dashboardKeyMap{
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "view"),
	),
	Create: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "create"),
	),
	Edit: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "edit"),
	),
	Transition: key.NewBinding(
		key.WithKeys("t"),
		key.WithHelp("t", "transition"),
	),
	Start: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "start"),
	),
	Close: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "close/done"),
	),
	Grab: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("g", "grab"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "refresh"),
	),
	Open: key.NewBinding(
		key.WithKeys("o"),
		key.WithHelp("o", "open ticket"),
	),
	Epic: key.NewBinding(
		key.WithKeys("x"),
		key.WithHelp("x", "explore epic"),
	),
	BackToMine: key.NewBinding(
		key.WithKeys("m"),
		key.WithHelp("m", "my tickets"),
	),
	ToggleAll: key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "show/hide closed"),
	),
}

// Detail view key bindings.
type detailKeyMap struct {
	Edit       key.Binding
	Transition key.Binding
	Comment    key.Binding
	Grab       key.Binding
	Open       key.Binding
}

var detailKeys = detailKeyMap{
	Edit: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "edit"),
	),
	Transition: key.NewBinding(
		key.WithKeys("t"),
		key.WithHelp("t", "transition"),
	),
	Comment: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "comment"),
	),
	Grab: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("g", "grab"),
	),
	Open: key.NewBinding(
		key.WithKeys("o"),
		key.WithHelp("o", "open in browser"),
	),
}

// Form key bindings.
type formKeyMap struct {
	NextField key.Binding
	PrevField key.Binding
	Submit    key.Binding
}

var formKeys = formKeyMap{
	NextField: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "next field"),
	),
	PrevField: key.NewBinding(
		key.WithKeys("shift+tab"),
		key.WithHelp("shift+tab", "prev field"),
	),
	Submit: key.NewBinding(
		key.WithKeys("ctrl+s"),
		key.WithHelp("ctrl+s", "submit"),
	),
}
