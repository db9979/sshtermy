package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sessionlog"
)

// Änderungsbericht (Sitzungsprotokolle, Taste b): Die KI schreibt aus den
// Sitzungen der gewählten Zeile (ein Tag, ein Host, eine Sitzung) einen
// Bericht „was wurde auf welchem Host geändert“, fertig für Ticket oder
// Doku. Er landet in der Zwischenablage und als Markdown-Datei in Downloads.

type logReportMsg struct {
	title, text string
	err         error
}

func (a *App) reportLogs(list []*sessionlog.Log, title string) tea.Cmd {
	if len(list) == 0 {
		return nil
	}
	ag, err := a.agentClient()
	if err != nil {
		a.flashErr("KI: " + err.Error())
		return nil
	}
	var b strings.Builder
	budget := 30000
	// älteste zuerst, damit der Bericht in der Reihenfolge der Arbeit steht
	for i := len(list) - 1; i >= 0; i-- {
		if b.Len() > 0 {
			b.WriteString("\n---\n")
		}
		b.WriteString(list[i].Transcript(max(2000, budget/len(list))))
		if b.Len() > budget {
			break
		}
	}
	a.logs.busy = i18n.T("Änderungsbericht für ") + title
	tr := b.String()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()
		text, err := ag.Report(ctx, tr)
		return logReportMsg{title, text, err}
	}
}

func (a *App) reportMsg(msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(logReportMsg)
	if !ok {
		return nil, false
	}
	a.logs.busy = ""
	if m.err != nil {
		a.flashErr(i18n.T("Änderungsbericht: ") + m.err.Error())
		return nil, true
	}
	now := time.Now()
	doc := fmt.Sprintf("# %s: %s\n\n%s\n\n_%s %s_\n", i18n.T("Änderungsbericht"), m.title, m.text,
		i18n.T("Erstellt mit sshTermy am"), now.Format("02.01.2006 15:04"))
	name := filepath.Join(exportDir(), "sshtermy-"+i18n.T("bericht")+"-"+unsafeName.ReplaceAllString(m.title, "_")+"-"+now.Format("20060102-1504")+".md")
	if err := os.WriteFile(name, []byte(doc), 0o600); err != nil {
		a.flashErr(err.Error())
		name = ""
	}
	a.logs.report, a.logs.reportFile, a.logs.group, a.logs.scroll = m.text, name, "", 0
	go func() { _ = clipboard.WriteAll(doc) }()
	a.flashOK(i18n.T("Änderungsbericht kopiert"))
	return tea.SetClipboard(doc), true
}
