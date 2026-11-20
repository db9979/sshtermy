package ui

// „Im Web suchen“: markierten Text (meist eine Fehlermeldung) mit der
// eingestellten Suchmaschine im Browser suchen. Rechtsklick auf eine
// Markierung zeigt dafür ein kleines Menü (Kopieren bleibt der erste
// Eintrag, ⏎ macht also dasselbe wie früher). Geschickt wird nur der
// markierte Text, ohne Passwörter/Token (agent.Redact), gekürzt auf 300
// Zeichen; Hostnamen und IP-Adressen bleiben drin, weil man sie sieht.

import (
	"net/url"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

type searchEngine struct{ id, label, url string }

var searchEngines = []searchEngine{
	{"", "DuckDuckGo", "https://duckduckgo.com/?q="},
	{"google", "Google", "https://www.google.com/search?q="},
	{"bing", "Bing", "https://www.bing.com/search?q="},
	{"startpage", "Startpage", "https://www.startpage.com/do/search?q="},
	{"ecosia", "Ecosia", "https://www.ecosia.org/search?q="},
}

func (a *App) searchEngine() (int, searchEngine) {
	id := ""
	if a.st != nil {
		id = a.st.Settings().WebSearch
	}
	for i, e := range searchEngines {
		if e.id == id {
			return i, e
		}
	}
	return 0, searchEngines[0]
}

func (a *App) cycleSearchEngine(dir int) tea.Cmd {
	i, _ := a.searchEngine()
	e := searchEngines[cycleIndex(i, len(searchEngines), dir)]
	if err := a.st.UpdateSettings(func(s *domain.Settings) { s.WebSearch = e.id }); err != nil {
		a.flash(i18n.T("Fehler: ") + err.Error())
	}
	return nil
}

// webQuery: Suchtext aus einer Markierung (eine Zeile, ohne Geheimnisse).
func webQuery(t string) string {
	t = agent.Redact(t)
	t = strings.Join(strings.Fields(t), " ")
	if r := []rune(t); len(r) > 300 {
		t = string(r[:300])
	}
	return t
}

// webSearch öffnet die Suche im Browser.
func (a *App) webSearch(t string) tea.Cmd {
	q := webQuery(t)
	if q == "" {
		a.flash(i18n.T("Nichts zu suchen: erst Text markieren"))
		return nil
	}
	_, e := a.searchEngine()
	return a.openURL(e.url + url.QueryEscape(q))
}

// selMenu: Rechtsklick auf eine Markierung im Terminal.
func (a *App) selMenu(s *session, text string, x, y int) *modal {
	items := []menuItem{
		{label: i18n.T("Kopieren"), hint: i18n.T("wie bisher"), run: func() tea.Cmd {
			_ = clipWrite(text)
			a.flash(i18n.T("Kopiert: ") + clipLabel(text))
			return tea.SetClipboard(text)
		}},
		{label: i18n.T("Im Web suchen"), hint: func() string { _, e := a.searchEngine(); return e.label }(), run: func() tea.Cmd { return a.webSearch(text) }},
	}
	if s != nil && s.term != nil && s.state == stConnected {
		items = append(items, menuItem{label: i18n.T("Einfügen"), hint: i18n.T("Markierung am Prompt"), sep: true, run: func() tea.Cmd { return a.guardedPaste(s, text) }})
	}
	if len(clipEntries()) > 1 {
		items = append(items, menuItem{label: i18n.T("Zwischenablage-Verlauf"), hint: a.keys.label(actClipHist), run: func() tea.Cmd { return a.openClipHist() }})
	}
	return &modal{kind: mMenu, title: clipLabel(text), items: items, at: [2]int{x, y}, anchored: true}
}
