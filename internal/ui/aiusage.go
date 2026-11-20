package ui

// KI-Verbrauch im KI-Status: Tokens heute, in 7 und in 30 Tagen, je Modell,
// dazu die teuersten Aufgaben von heute. Kosten nur, wenn in den
// KI-Einstellungen ein Preis je 1 Mio. Tokens steht (eigene Server kosten
// nichts). Die Zahlen kommen aus den Antworten des Servers; wo er nichts
// meldet (Streams, manche lokalen Server), ist geschätzt und mit ~ markiert.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/llm"
	"github.com/db9979/sshtermy/internal/store"
)

// hookAIUsage verbindet den Zähler der KI-Anfragen mit dem Speicher.
func (a *App) hookAIUsage() {
	st := a.st
	llm.OnUsage = func(u llm.Usage) {
		st.AddAIUsage(time.Now(), u.Model, u.Task, u.In, u.Out, u.Estimate, u.Cost())
	}
}

// aiUsageTick speichert höchstens einmal pro Minute.
func (a *App) aiUsageTick(now time.Time) {
	if now.Sub(a.usageSaved) < time.Minute {
		return
	}
	a.usageSaved = now
	_ = a.st.SaveAIUsage(now)
}

type usageSum struct {
	n, in, out, est int
	cost            float64
}

func (u *usageSum) add(r store.AIUsageRow) {
	u.n += r.N
	u.in += r.In
	u.out += r.Out
	u.est += r.Est
	u.cost += r.Cost
}

// fmtTokens: 950, 12,4k, 3,1 Mio.
func fmtTokens(n int) string {
	switch {
	case n < 1000:
		return strconv.Itoa(n)
	case n < 1_000_000:
		return localNum(fmt.Sprintf("%.1fk", float64(n)/1000))
	default:
		return localNum(fmt.Sprintf(i18n.T("%.1f Mio."), float64(n)/1e6))
	}
}

// localNum: Dezimalkomma auf Deutsch.
func localNum(s string) string {
	if i18n.Lang() == "en" {
		return s
	}
	return strings.Replace(s, ".", ",", 1)
}

func fmtCost(c float64) string {
	if c < 0.005 {
		return ""
	}
	return localNum(fmt.Sprintf("%.2f", c)) + " $"
}

// aiUsageView: Abschnitt „Verbrauch“ für den KI-Status ("" = noch nichts).
func (a *App) aiUsageView(now time.Time) string {
	s := a.s
	rows := a.st.AIUsageSince(now.AddDate(0, 0, -29))
	if len(rows) == 0 {
		return ""
	}
	today := now.Format("2006-01-02")
	week := now.AddDate(0, 0, -6).Format("2006-01-02")
	var d, w, m usageSum
	tasks := map[string]*usageSum{}
	for _, r := range rows {
		m.add(r)
		if r.Day >= week {
			w.add(r)
		}
		if r.Day == today {
			d.add(r)
			t := tasks[r.Task]
			if t == nil {
				t = &usageSum{}
				tasks[r.Task] = t
			}
			t.add(r)
		}
	}
	line := func(label string, u usageSum) string {
		tl := fmtTokens(u.in) + " ↑  " + fmtTokens(u.out) + " ↓"
		if u.est > 0 && u.est*2 >= u.n {
			tl = "~" + tl
		}
		out := fmt.Sprintf("  %-12s", label) + s.Text.Render(fmt.Sprintf("%-20s", tl)) +
			s.Muted.Render(fmt.Sprintf(i18n.T("%d Anfragen"), u.n))
		if c := fmtCost(u.cost); c != "" {
			out += s.Muted.Render(" · ") + s.Warn.Render(c)
		}
		return out + "\n"
	}
	var b strings.Builder
	b.WriteString("\n" + s.Accent.Render(i18n.T("Verbrauch")) + s.Muted.Render(i18n.T(" · Tokens hin ↑ und zurück ↓, ~ = geschätzt")) + "\n")
	b.WriteString(line(i18n.T("heute"), d))
	b.WriteString(line(i18n.T("7 Tage"), w))
	b.WriteString(line(i18n.T("30 Tage"), m))
	if len(tasks) > 0 {
		type kv struct {
			k string
			v *usageSum
		}
		var ts []kv
		for k, v := range tasks {
			ts = append(ts, kv{k, v})
		}
		sort.Slice(ts, func(i, j int) bool {
			ti, tj := ts[i].v.in+ts[i].v.out, ts[j].v.in+ts[j].v.out
			if ti != tj {
				return ti > tj
			}
			return ts[i].k < ts[j].k
		})
		var parts []string
		for i, t := range ts {
			if i == 4 {
				break
			}
			parts = append(parts, taskLabel(t.k)+" "+fmtTokens(t.v.in+t.v.out))
		}
		b.WriteString(s.Muted.Render("  "+i18n.T("heute meist: ")+strings.Join(parts, " · ")) + "\n")
	}
	if a.st.LLM().PriceIn == 0 && a.st.LLM().PriceOut == 0 && m.n > 0 {
		b.WriteString(s.Muted.Render("  "+i18n.T("Kosten: Preis je 1 Mio. Tokens in den KI-Einstellungen eintragen (s)")) + "\n")
	}
	return b.String()
}

// parsePrices: „3 / 15“ oder „3;15“ → Eingabe, Ausgabe (je 1 Mio. Tokens).
func parsePrices(v string) (in, out float64, ok bool) {
	v = strings.TrimSpace(strings.ReplaceAll(v, ",", "."))
	if v == "" {
		return 0, 0, true
	}
	f := strings.FieldsFunc(v, func(r rune) bool { return r == '/' || r == ';' || r == ' ' || r == '$' })
	if len(f) == 0 || len(f) > 2 {
		return 0, 0, false
	}
	p := make([]float64, len(f))
	for i, x := range f {
		n, err := strconv.ParseFloat(x, 64)
		if err != nil || n < 0 {
			return 0, 0, false
		}
		p[i] = n
	}
	if len(p) == 1 {
		return p[0], p[0], true
	}
	return p[0], p[1], true
}

func fmtPrices(in, out float64) string {
	if in == 0 && out == 0 {
		return ""
	}
	return strconv.FormatFloat(in, 'f', -1, 64) + " / " + strconv.FormatFloat(out, 'f', -1, 64)
}
