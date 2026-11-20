package ui

// Aufgaben je Verbindung oder Ordner (domain/tasks.go): offene Arbeiten
// mit Fälligkeit. u im Manager (oder Alt+A im Terminal) zeigt die Liste der
// gewählten Verbindung samt den Aufgaben ihrer Ordner. ⏎ startet eine
// Aufgabe als Ziel im offenen Tab dieser Verbindung (Ziel- oder
// Agent-Modus, wie zuletzt gewählt); danach steht sie „zur Abnahme“ mit
// dem Ergebnis und der Gegenprüfung, a hakt sie ab.

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

const mTasks modalKind = 100 // Aufgabenliste (eigene Nummer, kein iota-Konflikt)

// taskRef: eine Aufgabe in der Liste; folder != "" = Aufgabe des Ordners.
type taskRef struct {
	folder string
	t      domain.Task
}

type taskList struct {
	connID string // "" = nur ein Ordner
	folder string // Ordner der Liste (bei Verbindung: ihr Ordner)
	sel    int
}

// openTasks: Liste für eine Verbindung (mit ihren Ordnern) oder einen Ordner.
func (a *App) openTasks(connID, folder string) tea.Cmd {
	a.taskL = &taskList{connID: connID, folder: domain.NormPath(folder)}
	a.modal = &modal{kind: mTasks}
	return nil
}

// taskRefs: offene zuerst (zur Abnahme, läuft, offen; nach Fälligkeit),
// dann die zuletzt erledigten.
func (a *App) taskRefs() []taskRef {
	l := a.taskL
	if l == nil {
		return nil
	}
	var out []taskRef
	if l.connID != "" {
		if c, ok := a.st.Connection(l.connID); ok {
			for _, t := range c.Tasks {
				out = append(out, taskRef{t: t})
			}
		}
	}
	for p := l.folder; p != ""; p = domain.PathParent(p) {
		for _, t := range a.st.FolderTasks(p) {
			out = append(out, taskRef{folder: p, t: t})
		}
		if l.connID == "" {
			break // Ordnerliste: nur dieser Ordner
		}
	}
	rank := func(s string) int {
		switch s {
		case domain.TaskReview:
			return 0
		case domain.TaskRunning:
			return 1
		case domain.TaskOpen:
			return 2
		}
		return 3
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].t, out[j].t
		if rank(a.State) != rank(b.State) {
			return rank(a.State) < rank(b.State)
		}
		if a.State == domain.TaskDone {
			return a.Updated.After(b.Updated)
		}
		if a.Due.IsZero() != b.Due.IsZero() {
			return !a.Due.IsZero()
		}
		if !a.Due.Equal(b.Due) {
			return a.Due.Before(b.Due)
		}
		return a.Created.Before(b.Created)
	})
	// erledigte: nur die letzten fünf zeigen
	n := 0
	keep := out[:0]
	for _, r := range out {
		if r.t.State == domain.TaskDone {
			n++
			if n > 5 {
				continue
			}
		}
		keep = append(keep, r)
	}
	return keep
}

// updateTask ändert eine Aufgabe (Verbindung oder Ordner) per ID.
func (a *App) updateTask(connID, folder, id string, fn func(t *domain.Task) bool) error {
	edit := func(ts []domain.Task) ([]domain.Task, bool) {
		out := append([]domain.Task(nil), ts...)
		for i := range out {
			if out[i].ID == id {
				if !fn(&out[i]) { // false = löschen
					return append(out[:i], out[i+1:]...), true
				}
				out[i].Updated = time.Now().UTC()
				return out, true
			}
		}
		return out, false
	}
	if folder != "" {
		ts, ok := edit(a.st.FolderTasks(folder))
		if !ok {
			return nil
		}
		return a.st.SetFolderTasks(folder, ts)
	}
	c, ok := a.st.Connection(connID)
	if !ok {
		return fmt.Errorf("%s", i18n.T("Verbindung nicht gefunden"))
	}
	ts, found := edit(c.Tasks)
	if !found {
		return nil
	}
	c.Tasks = ts
	_, err := a.st.UpsertConnection(c)
	return err
}

func (a *App) addTask(connID, folder, text string) error {
	text, due := domain.ParseTaskText(text, time.Now())
	if strings.TrimSpace(text) == "" {
		return nil
	}
	t := domain.Task{ID: "t" + strconvBase36(time.Now().UnixNano()), Text: text, Due: due, Created: time.Now().UTC()}
	if folder != "" && connID == "" {
		return a.st.SetFolderTasks(folder, append(a.st.FolderTasks(folder), t))
	}
	c, ok := a.st.Connection(connID)
	if !ok {
		return fmt.Errorf("%s", i18n.T("Verbindung nicht gefunden"))
	}
	c.Tasks = append(append([]domain.Task(nil), c.Tasks...), t)
	_, err := a.st.UpsertConnection(c)
	return err
}

func strconvBase36(n int64) string {
	const digits = "0123456789abcdefghijklmnopqrstuvwxyz"
	if n < 0 {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{digits[n%36]}, b...)
		n /= 36
	}
	return string(b)
}

// taskSession: offener, verbundener Tab für die Aufgabe.
func (a *App) taskSession(r taskRef) *session {
	l := a.taskL
	ok := func(s *session) bool {
		if s == nil || s.state != stConnected || s.term == nil {
			return false
		}
		if l.connID != "" {
			return s.conn.ID == l.connID
		}
		g := domain.NormPath(s.conn.Group)
		return g == r.folder || strings.HasPrefix(g, r.folder+domain.PathSep)
	}
	if s := a.current(); ok(s) {
		return s
	}
	for _, s := range a.sessions {
		if ok(s) {
			return s
		}
	}
	return nil
}

// startTask: Aufgabe als Ziel im passenden Tab.
func (a *App) startTask(r taskRef) tea.Cmd {
	if r.t.State == domain.TaskDone {
		a.flash(i18n.T("Schon erledigt · a öffnet sie wieder"))
		return nil
	}
	s := a.taskSession(r)
	if s == nil {
		if a.taskL.connID != "" {
			if c, ok := a.st.Connection(a.taskL.connID); ok {
				a.flash(fmt.Sprintf(i18n.T("Erst mit %s verbinden, dann ⏎ in den Aufgaben (Alt+A)"), c.Alias))
				return nil
			}
		}
		a.flash(i18n.T("Kein offener Tab zu dieser Aufgabe: erst verbinden"))
		return nil
	}
	connID, folder, id := a.taskL.connID, r.folder, r.t.ID
	if folder == "" {
		connID = s.conn.ID
	}
	_ = a.updateTask(connID, folder, id, func(t *domain.Task) bool { t.State, t.Note = domain.TaskRunning, ""; return true })
	a.modal, a.taskL = nil, nil
	for i, x := range a.sessions {
		if x == s {
			a.focusSession(i)
		}
	}
	a.scr = scrTerminal
	cmd := a.startGoal(s, r.t.Text)
	s.goal.task = &goalTask{connID: connID, folder: folder, id: id}
	return cmd
}

// goalTask: das laufende Ziel gehört zu dieser Aufgabe.
type goalTask struct{ connID, folder, id string }

// taskGoalEnded (aus recordGoal): Aufgabe zur Abnahme bzw. wieder offen.
func (a *App) taskGoalEnded(s *session, state string) {
	gt := s.goal.task
	if gt == nil {
		return
	}
	if state != "done" {
		s.goal.task = nil
	}
	note := s.goal.doneReason
	_ = a.updateTask(gt.connID, gt.folder, gt.id, func(t *domain.Task) bool {
		if state == "done" {
			t.State, t.Note = domain.TaskReview, note
		} else {
			t.State, t.Note = domain.TaskOpen, i18n.T("Ziel abgebrochen")
		}
		return true
	})
}

// taskVerified (aus verifyDone): Befund der Gegenprüfung an die Aufgabe.
func (a *App) taskVerified(s *session, ok bool, finding string) {
	gt := s.goal.task
	if gt == nil {
		return
	}
	s.goal.task = nil
	mark := "✓ "
	if !ok {
		mark = "▲ "
	}
	_ = a.updateTask(gt.connID, gt.folder, gt.id, func(t *domain.Task) bool {
		t.Note = strings.TrimSpace(t.Note + "\n" + mark + i18n.T("Gegenprüfung: ") + finding)
		return true
	})
}

func (a *App) tasksKey(key string) tea.Cmd {
	l := a.taskL
	refs := a.taskRefs()
	if l == nil {
		a.modal = nil
		return nil
	}
	l.sel = clamp(l.sel, 0, max(0, len(refs)-1))
	var cur *taskRef
	if l.sel < len(refs) {
		cur = &refs[l.sel]
	}
	target := func(r *taskRef) string { // Verbindung der Aufgabe
		if r.folder != "" {
			return ""
		}
		return l.connID
	}
	switch key {
	case "esc", "q":
		a.modal, a.taskL = nil, nil
	case "up", "k":
		l.sel = max(0, l.sel-1)
	case "down", "j":
		l.sel = min(max(0, len(refs)-1), l.sel+1)
	case "n", "N":
		connID, folder := l.connID, ""
		where := i18n.T("dieser Verbindung")
		if key == "N" || l.connID == "" {
			if l.folder == "" {
				a.flash(i18n.T("Die Verbindung liegt in keinem Ordner"))
				return nil
			}
			connID, folder = "", l.folder
			where = i18n.T("dem Ordner ") + l.folder
		}
		back := *l
		a.modal = inputModal(i18n.T("Neue Aufgabe"), i18n.T("Für ")+where+i18n.T(". Fälligkeit am Ende, z. B. „bis 12.12.“"), "", func(x string) tea.Cmd {
			if err := a.addTask(connID, folder, x); err != nil {
				a.flashErr(err.Error())
			}
			a.taskL = &back
			a.modal = &modal{kind: mTasks}
			return nil
		})
		a.modal.in.Placeholder = i18n.T("z. B. Zertifikat erneuern bis 12.12.")
		a.modal.in.SetWidth(60)
	case "enter":
		if cur != nil {
			return a.startTask(*cur)
		}
	case "a", "space":
		if cur == nil {
			return nil
		}
		_ = a.updateTask(target(cur), cur.folder, cur.t.ID, func(t *domain.Task) bool {
			if t.State == domain.TaskDone {
				t.State = domain.TaskOpen
			} else {
				t.State = domain.TaskDone
			}
			return true
		})
	case "e":
		if cur == nil {
			return nil
		}
		r, back := *cur, *l
		val := r.t.Text
		if !r.t.Due.IsZero() {
			val += " " + r.t.Due.Format("02.01.2006")
		}
		a.modal = inputModal(i18n.T("Aufgabe ändern"), i18n.T("Fälligkeit am Ende, z. B. „bis 12.12.“"), val, func(x string) tea.Cmd {
			text, due := domain.ParseTaskText(x, time.Now())
			if text != "" {
				_ = a.updateTask(target(&r), r.folder, r.t.ID, func(t *domain.Task) bool { t.Text, t.Due = text, due; return true })
			}
			a.taskL = &back
			a.modal = &modal{kind: mTasks}
			return nil
		})
		a.modal.in.SetWidth(60)
	case "delete", "d", "backspace":
		if cur == nil {
			return nil
		}
		r := *cur
		_ = a.updateTask(target(&r), r.folder, r.t.ID, func(*domain.Task) bool { return false })
		a.flash(i18n.T("Aufgabe gelöscht: ") + trunc(r.t.Text, 40))
	}
	return nil
}

func (a *App) tasksView() string {
	st := a.s
	l := a.taskL
	refs := a.taskRefs()
	w := dialogInner(a.w)
	title, sub := i18n.T("Aufgaben"), l.folder
	if l.connID != "" {
		if c, ok := a.st.Connection(l.connID); ok {
			sub = c.Alias
		}
	}
	now := time.Now()
	var b strings.Builder
	if len(refs) == 0 {
		b.WriteString(st.Muted.Render(i18n.T("Noch keine Aufgaben. n legt eine an, z. B. „Zertifikat erneuern bis 12.12.“.")) + "\n")
	}
	for i, r := range refs {
		t := r.t
		mark, ms := "○", st.Text
		switch t.State {
		case domain.TaskRunning:
			mark, ms = "◌", st.AI
		case domain.TaskReview:
			mark, ms = "◆", st.Warn
		case domain.TaskDone:
			mark, ms = "✓", st.Muted
		}
		meta := ""
		switch {
		case t.State == domain.TaskReview:
			meta = i18n.T("zur Abnahme")
		case t.State == domain.TaskRunning:
			meta = i18n.T("läuft")
		case t.Overdue(now):
			meta = i18n.T("fällig ") + t.Due.Format("02.01.")
		case !t.Due.IsZero():
			meta = i18n.T("bis ") + t.Due.Format("02.01.")
		}
		if r.folder != "" && l.connID != "" {
			meta = strings.TrimSpace(meta + " · " + trunc(r.folder, 18))
		}
		text := trunc(t.Text, max(10, w-lipgloss.Width(meta)-6))
		line := ms.Render(mark) + " " + st.Text.Render(text)
		if t.State == domain.TaskDone {
			line = ms.Render(mark + " " + text)
		}
		gap := max(1, w-2-lipgloss.Width(mark+" "+text)-lipgloss.Width(meta))
		metaSt := st.Muted
		if t.Overdue(now) || t.State == domain.TaskReview {
			metaSt = st.Warn
		}
		line += strings.Repeat(" ", gap) + metaSt.Render(meta)
		if i == l.sel {
			line = st.Selected.Render(ansi.Strip(line))
		}
		b.WriteString(line + "\n")
		if i == l.sel && t.Note != "" {
			for _, nl := range strings.Split(t.Note, "\n") {
				for _, wl := range wrapText(nl, max(20, w-6)) {
					b.WriteString("    " + st.Muted.Render(wl) + "\n")
				}
			}
		}
	}
	keys := [][2]string{{"⏎", i18n.T("als Ziel starten")}, {"n", i18n.T("neu")}}
	if l.connID != "" && l.folder != "" {
		keys = append(keys, [2]string{"⇧N", i18n.T("neu für den Ordner")})
	}
	keys = append(keys, [2]string{"a", i18n.T("abhaken")}, [2]string{"e", i18n.T("ändern")}, [2]string{"Entf", i18n.T("löschen")}, [2]string{"Esc", i18n.T("schließen")})
	return st.dialog(dialog{icon: "◇", title: title, sub: sub, body: b.String(), keys: keys, width: w}, a.w)
}

// tasksBadge: „2 Aufgaben · 1 fällig“ für die Detailspalte ("" = keine).
func tasksBadge(ts []domain.Task, now time.Time) string {
	n, due, review := 0, 0, 0
	for _, t := range ts {
		if t.State == domain.TaskDone {
			continue
		}
		n++
		if t.Overdue(now) {
			due++
		}
		if t.State == domain.TaskReview {
			review++
		}
	}
	if n == 0 {
		return ""
	}
	out := plural(n, i18n.T("Aufgabe"), i18n.T("Aufgaben"))
	if due > 0 {
		out += fmt.Sprintf(i18n.T(" · %d fällig"), due)
	}
	if review > 0 {
		out += fmt.Sprintf(i18n.T(" · %d zur Abnahme"), review)
	}
	return out + i18n.T(" · u zeigt")
}

// folderTasksUp: Aufgaben aller Ordner über einer Verbindung.
func (a *App) folderTasksUp(group string) []domain.Task {
	var out []domain.Task
	for p := domain.NormPath(group); p != ""; p = domain.PathParent(p) {
		out = append(out, a.st.FolderTasks(p)...)
	}
	return out
}

// tasksReminder: einmal nach dem Entsperren an fällige Aufgaben erinnern.
func (a *App) tasksReminder(now time.Time) {
	if a.tasksReminded || a.st == nil || a.scr == scrUnlock || a.st.V == nil || !a.st.V.Unlocked() {
		return
	}
	a.tasksReminded = true
	due, review := 0, 0
	count := func(ts []domain.Task) {
		for _, t := range ts {
			if t.Overdue(now) {
				due++
			}
			if t.State == domain.TaskReview {
				review++
			}
		}
	}
	for _, c := range a.st.Connections() {
		count(c.Tasks)
	}
	for _, f := range a.st.Folders() {
		count(f.Tasks)
	}
	switch {
	case due > 0:
		a.pushToast(toastWarn, fmt.Sprintf(i18n.T("%s fällig · u im Manager zeigt die Aufgaben"), plural(due, i18n.T("Aufgabe"), i18n.T("Aufgaben"))))
	case review > 0:
		a.flash(fmt.Sprintf(i18n.T("%s zur Abnahme · u im Manager"), plural(review, i18n.T("Aufgabe"), i18n.T("Aufgaben"))))
	}
}
