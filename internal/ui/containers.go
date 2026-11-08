package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/containers"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Container-Ansicht (F6 c, Palette „docker“)
//
// Listet Docker-, Podman-Container und Kubernetes-Pods des Servers über eine
// eigene Exec-Sitzung. ⏎ öffnet eine Shell im Container, Strg+L zeigt die
// Logs live, Strg+K lässt die KI die letzten Log-Zeilen auswerten. Neustart
// und Start/Stopp werden nur eingefügt: erst ⏎ im Terminal führt sie aus.

type containersMsg struct {
	sid int
	res containers.Result
	err error
}

type containerLogsMsg struct {
	sid  int
	name string
	text string
	err  error
}

func (a *App) containersOK(s *session) bool {
	return s != nil && s.state == stConnected && a.hostServerOK(s)
}

func (a *App) openContainers(s *session) tea.Cmd {
	if !a.containersOK(s) {
		a.flash(i18n.T("Container gibt es nur auf verbundenen Linux-Servern (SSH)"))
		return nil
	}
	a.flash(i18n.T("Suche Container (docker, podman, kubectl) …"))
	sid, cl := s.id, s.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		out, err := cl.RunScript(ctx, containers.Script)
		return containersMsg{sid: sid, res: containers.Parse(out), err: err}
	}
}

func containerItems(r containers.Result) []pickItem {
	var out []pickItem
	for i, c := range r.List {
		dot := "○ "
		if c.Running() {
			dot = "● "
		}
		label := dot + c.Name
		detail := c.Image
		if c.Runtime == "k8s" {
			detail = "ns " + c.Namespace
		}
		hint := c.Status
		if c.Runtime != "docker" {
			hint += " · " + c.Runtime
		}
		out = append(out, pickItem{value: fmt.Sprint(i), label: label, detail: detail, hint: trunc(hint, 34),
			search: c.Runtime + " " + c.Namespace + " " + c.State})
	}
	return out
}

func (a *App) containerMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case containersMsg:
		s := a.byID(m.sid)
		if s == nil {
			return nil, true
		}
		if m.err != nil && len(m.res.List) == 0 {
			a.flashErr(i18n.T("Container: ") + m.err.Error())
			return nil, true
		}
		return a.showContainers(s, m.res), true
	case containerLogsMsg:
		s := a.byID(m.sid)
		if s == nil {
			return nil, true
		}
		if m.err != nil && strings.TrimSpace(m.text) == "" {
			a.flashErr(i18n.T("Logs: ") + m.err.Error())
			return nil, true
		}
		if strings.TrimSpace(m.text) == "" {
			a.flash(i18n.T("Der Container hat keine Logs"))
			return nil, true
		}
		return a.diagnoseWith(s, m.text, fmt.Sprintf(i18n.T("Logs von %s (%s)"), m.name, plural(countLines(m.text), i18n.T("Zeile"), i18n.T("Zeilen")))), true
	}
	return nil, false
}

func (a *App) showContainers(s *session, r containers.Result) tea.Cmd {
	if len(r.List) == 0 {
		switch {
		case len(r.Errors) > 0:
			a.flashErr(i18n.T("Keine Container lesbar: ") + strings.Join(r.Errors, " · ") + containerHint(r.Errors))
		case len(r.Runtimes) == 0:
			a.flash(i18n.T("Kein docker, podman oder kubectl auf ") + s.tabName())
		default:
			a.flash(i18n.T("Keine Container auf ") + s.tabName())
		}
		return nil
	}
	list := r.List
	p := newPicker(fmt.Sprintf(i18n.T("Container auf %s"), s.tabName()), i18n.T("Name, Image, Namespace"), containerItems(r), "", func(it pickItem) tea.Cmd {
		c, ok := pickContainer(list, it.value)
		if !ok {
			return nil
		}
		s.panelFocus = false
		return a.sendCommand(s, c.ExecCmd())
	})
	help := i18n.T("● läuft · ○ gestoppt. Neustart und Start/Stopp werden nur eingefügt, ⏎ im Terminal führt sie aus.")
	if len(r.Errors) > 0 {
		help += " " + strings.Join(r.Errors, " · ") + containerHint(r.Errors)
	}
	p.help = help
	p.keys = [][2]string{{"⏎", i18n.T("Shell im Container")}, {"Strg+L", i18n.T("Logs live")}, {"Strg+K", i18n.T("KI wertet Logs aus")},
		{"Strg+R", i18n.T("Neustart einfügen")}, {"Strg+O", i18n.T("Start/Stopp einfügen")}, {"Strg+U", i18n.T("neu laden")}, {"Esc", i18n.T("zurück")}}
	p.onKey = func(key string, it *pickItem) (tea.Cmd, bool) {
		if key == "ctrl+u" {
			a.pick = nil
			return a.openContainers(s), true
		}
		if it == nil {
			return nil, false
		}
		c, ok := pickContainer(list, it.value)
		if !ok {
			return nil, false
		}
		switch key {
		case "ctrl+l":
			a.pick = nil
			s.panelFocus = false
			a.flash(i18n.T("Logs laufen · Strg+C beendet"))
			return a.sendCommand(s, c.LogsCmd()), true
		case "ctrl+k":
			a.pick = nil
			return a.containerLogs(s, c), true
		case "ctrl+r":
			a.pick = nil
			return a.insertCommand(s, c.RestartCmd()), true
		case "ctrl+o":
			cmd := c.StartStopCmd()
			if cmd == "" {
				a.flash(i18n.T("Pods startet und stoppt Kubernetes selbst · Strg+R löscht den Pod, der Controller legt ihn neu an"))
				return nil, true
			}
			a.pick = nil
			return a.insertCommand(s, cmd), true
		}
		return nil, false
	}
	return a.openPicker(p)
}

func pickContainer(list []containers.Container, v string) (containers.Container, bool) {
	var i int
	if _, err := fmt.Sscan(v, &i); err != nil || i < 0 || i >= len(list) {
		return containers.Container{}, false
	}
	return list[i], true
}

// containerHint: Tipp zu typischen Fehlern.
func containerHint(errs []string) string {
	all := strings.ToLower(strings.Join(errs, " "))
	if strings.Contains(all, "permission denied") {
		return i18n.T(" · Tipp: Benutzer in die Gruppe docker aufnehmen oder sudo ohne Passwort erlauben")
	}
	return ""
}

// insertCommand: Befehl an den Prompt schreiben, ohne ihn auszuführen.
func (a *App) insertCommand(s *session, cmd string) tea.Cmd {
	if s.state != stConnected || s.term == nil {
		return nil
	}
	s.panelFocus = false
	s.term.SendText(cmd)
	a.flash(i18n.T("Eingefügt · ⏎ führt aus, Strg+C verwirft"))
	return nil
}

// containerLogs holt die letzten Log-Zeilen über Exec und gibt sie der KI.
func (a *App) containerLogs(s *session, c containers.Container) tea.Cmd {
	if !a.st.LLM().SendScreen {
		a.flash(i18n.T("Terminal-Inhalt an die KI ist in den KI-Einstellungen ausgeschaltet"))
		return nil
	}
	a.flash(fmt.Sprintf(i18n.T("Hole Logs von %s …"), c.Name))
	sid, cl := s.id, s.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		out, err := cl.RunScript(ctx, c.FetchLogs(diagnoseLines))
		return containerLogsMsg{sid: sid, name: c.Name, text: out, err: err}
	}
}
