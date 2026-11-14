package cmdset

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/db9979/sshtermy/internal/i18n"
)

//go:embed builtin/*.yaml
var builtinFS embed.FS

// LoadError: eine Datei, die nicht geladen wurde.
type LoadError struct {
	File string
	ID   string
	Err  string
}

// Status eines Satzes im Register.
type Status struct {
	Set      *Set
	Active   bool   // wird benutzt
	Pending  bool   // Team: wartet auf Freigabe
	Changed  bool   // Team: früher freigegeben, Inhalt seitdem geändert
	Problem  string // warum nicht aktiv / Selbsttest
	Shadowed bool   // Kennung schon vergeben
}

type registry struct {
	builtin []*Set
	user    []*Set // eigene + freigegebene Team-Sätze (aktiv)
	all     []Status
	errs    []LoadError
	rx      map[string]*regexp.Regexp
	danger  []compiledDanger
}

type compiledDanger struct {
	rx               *regexp.Regexp
	reason, reasonEN string
	from             string
}

var (
	mu  sync.RWMutex
	reg = &registry{rx: map[string]*regexp.Regexp{}}
)

func init() {
	sets, err := loadBuiltin()
	if err != nil {
		panic(err) // eingebaute Sätze sind Teil des Programms; Tests fangen das ab
	}
	r := &registry{builtin: sets, rx: map[string]*regexp.Regexp{}}
	for _, s := range sets {
		r.all = append(r.all, Status{Set: s, Active: true})
	}
	r.compile()
	reg = r
}

func loadBuiltin() ([]*Set, error) {
	ents, err := builtinFS.ReadDir("builtin")
	if err != nil {
		return nil, err
	}
	var out []*Set
	seen := map[string]bool{}
	for _, e := range ents {
		raw, err := builtinFS.ReadFile("builtin/" + e.Name())
		if err != nil {
			return nil, err
		}
		s, err := ParseFile(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		s.Source = Builtin
		if seen[s.ID] {
			return nil, fmt.Errorf("%s: Kennung %s doppelt", e.Name(), s.ID)
		}
		seen[s.ID] = true
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// ValidateBuiltin prüft alle eingebauten Sätze (für Tests; braucht CheckRead).
func ValidateBuiltin() error {
	var errs []string
	for _, s := range Builtins() {
		if err := s.Validate(); err != nil {
			errs = append(errs, s.ID+": "+err.Error())
		}
		if s.Base != "" {
			errs = append(errs, s.ID+": eingebaute Sätze bauen auf keinem anderen auf")
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "\n"))
	}
	return nil
}

func (r *registry) compile() {
	add := func(p string) {
		if p == "" {
			return
		}
		if _, ok := r.rx[p]; ok {
			return
		}
		if rx, err := regexp.Compile(p); err == nil {
			r.rx[p] = rx
		}
	}
	r.danger = nil
	for _, s := range r.active() {
		for _, l := range [][]string{s.Detect.Banner, s.Detect.Screen, s.Detect.Version, s.Read} {
			for _, p := range l {
				add(p)
			}
		}
		for _, d := range s.Danger {
			if rx, err := regexp.Compile(`(?im)` + d.Rx); err == nil {
				r.danger = append(r.danger, compiledDanger{rx: rx, reason: d.Reason, reasonEN: d.ReasonEN, from: s.ID})
			}
		}
	}
}

func (r *registry) active() []*Set { return append(append([]*Set(nil), r.builtin...), r.user...) }

func (r *registry) match(p, text string) bool {
	if rx := r.rx[p]; rx != nil {
		return rx.MatchString(text)
	}
	rx, err := regexp.Compile(p)
	return err == nil && rx.MatchString(text)
}

// ---------------------------------------------------------------- Abfragen

// Builtins: alle eingebauten Sätze in Erkennungsreihenfolge.
func Builtins() []*Set {
	mu.RLock()
	defer mu.RUnlock()
	return append([]*Set(nil), reg.builtin...)
}

// All: alle aktiven Sätze (eingebaute zuerst).
func All() []*Set {
	mu.RLock()
	defer mu.RUnlock()
	return reg.active()
}

// Statuses: alle bekannten Sätze mit Zustand (für die Liste im Editor).
func Statuses() []Status {
	mu.RLock()
	defer mu.RUnlock()
	return append([]Status(nil), reg.all...)
}

// Errors: Dateien, die beim Laden scheiterten.
func Errors() []LoadError {
	mu.RLock()
	defer mu.RUnlock()
	return append([]LoadError(nil), reg.errs...)
}

// Lookup sucht einen aktiven Satz.
func Lookup(id string) (*Set, bool) {
	if id == "" {
		return nil, false
	}
	mu.RLock()
	defer mu.RUnlock()
	return reg.lookup(id)
}

func (r *registry) lookup(id string) (*Set, bool) {
	for _, s := range r.builtin {
		if s.ID == id {
			return s, true
		}
	}
	for _, s := range r.user {
		if s.ID == id {
			return s, true
		}
	}
	return nil, false
}

// IsBuiltin: Kennung eines eingebauten Satzes?
func IsBuiltin(id string) bool {
	for _, s := range Builtins() {
		if s.ID == id {
			return true
		}
	}
	return false
}

// Resolve liefert den wirksamen Satz für ein Gerät: Grundlage(n), dann der
// Satz selbst, dann die erste passende Variante. nil = unbekannt.
func Resolve(id, model, firmware string) *Set {
	mu.RLock()
	defer mu.RUnlock()
	return reg.resolve(id, model, firmware)
}

func (r *registry) resolve(id, model, firmware string) *Set {
	s, ok := r.lookup(id)
	if !ok {
		return nil
	}
	chain := []*Set{s}
	for cur := s; cur.Base != "" && len(chain) <= maxBaseDepth; {
		b, ok := r.lookup(cur.Base)
		if !ok || b == s {
			break
		}
		chain = append(chain, b)
		cur = b
	}
	eff := &Set{}
	for i := len(chain) - 1; i >= 0; i-- {
		overlay(eff, chain[i], i == 0)
	}
	eff.Variants = nil
	for _, v := range s.Variants {
		if v.matches(model, firmware) {
			applyVariant(eff, v)
			break
		}
	}
	return eff
}

func (v Variant) matches(model, firmware string) bool {
	ok := func(p, val string) bool {
		if p == "" {
			return true
		}
		rx, err := regexp.Compile(p)
		return err == nil && val != "" && rx.MatchString(val)
	}
	return ok(v.Model, model) && ok(v.Firmware, firmware)
}

// VariantName: welche Variante für Modell/Firmware gilt ("" = keine).
func VariantName(id, model, firmware string) string {
	s, ok := Lookup(id)
	if !ok {
		return ""
	}
	for _, v := range s.Variants {
		if v.matches(model, firmware) {
			return v.Name
		}
	}
	return ""
}

// overlay legt src über dst: gesetzte Felder gewinnen. Kennung, Name und
// Herkunft kommen vom obersten Satz (top). Erkennung wird nicht vererbt.
func overlay(dst, src *Set, top bool) {
	str := func(d *string, v string) {
		if v != "" {
			*d = v
		}
	}
	if top {
		dst.ID, dst.Name, dst.NameEN, dst.Base = src.ID, src.Name, src.NameEN, src.Base
		dst.Source, dst.File, dst.Sum, dst.From, dst.Order = src.Source, src.File, src.Sum, src.From, src.Order
		dst.Detect = src.Detect
		dst.Samples = src.Samples
		dst.Read = src.Read
	}
	str(&dst.Family, src.Family)
	str(&dst.ReadKind, src.ReadKind)
	if src.Hint != "" {
		dst.Hint = strings.TrimSpace(dst.Hint + " " + src.Hint)
	}
	overCommands(&dst.Commands, src.Commands)
	overParse(&dst.Parse, src.Parse)
	overLogs(&dst.Logs, src.Logs)
	overSyslog(&dst.Syslog, src.Syslog)
	if len(src.Advisory.CPE) > 0 {
		dst.Advisory.CPE = src.Advisory.CPE
	}
	if src.Advisory.Version != "" {
		dst.Advisory.Version = src.Advisory.Version
	}
	if src.Advisory.URL != "" {
		dst.Advisory.URL = src.Advisory.URL
	}
	dst.Danger = append(dst.Danger, src.Danger...)
	dst.Variants = src.Variants
}

func applyVariant(dst *Set, v Variant) {
	if v.Hint != "" {
		dst.Hint = strings.TrimSpace(dst.Hint + " " + v.Hint)
	}
	overCommands(&dst.Commands, v.Commands)
	overParse(&dst.Parse, v.Parse)
	overLogs(&dst.Logs, v.Logs)
	overSyslog(&dst.Syslog, v.Syslog)
}

func overCommands(d *Commands, s Commands) {
	for _, x := range []struct {
		d *string
		v string
	}{{&d.PagingOff, s.PagingOff}, {&d.Version, s.Version}, {&d.System, s.System}, {&d.Memory, s.Memory},
		{&d.Ports, s.Ports}, {&d.VLANs, s.VLANs}, {&d.Config, s.Config}} {
		if x.v != "" {
			*x.d = x.v
		}
	}
	if len(s.Extra) > 0 {
		d.Extra = s.Extra
	}
}

func overParse(d *Parse, s Parse) {
	for _, x := range []struct {
		d *string
		v string
	}{{&d.Reader, s.Reader}, {&d.Model, s.Model}, {&d.Firmware, s.Firmware},
		{&d.Name, s.Name}, {&d.Uptime, s.Uptime}, {&d.CPU, s.CPU}, {&d.Mem, s.Mem},
		{&d.PortUp, s.PortUp}, {&d.PortDown, s.PortDown}, {&d.VLAN, s.VLAN}} {
		if x.v != "" {
			*x.d = x.v
		}
	}
}

func overLogs(d *Logs, s Logs) {
	if len(s.Commands) > 0 {
		d.Commands = s.Commands
	}
	if s.Ports != "" {
		d.Ports = s.Ports
	}
	if s.Reader != "" {
		d.Reader = s.Reader
	}
}

func overSyslog(d *Syslog, s Syslog) {
	if len(s.Steps) > 0 {
		d.Steps, d.Abort, d.Note = s.Steps, s.Abort, "" // Abbruch gehört zu den Schritten
	}
	if s.Note != "" {
		d.Note = s.Note
		if len(s.Steps) == 0 {
			d.Steps, d.Abort = nil, nil
		}
	}
	if s.Save != "" {
		d.Save = s.Save
	}
}

// ---------------------------------------------------------------- Erkennung

// detect: erster Satz, dessen Muster passt. Eingebaute Sätze zuerst, eigene
// und Team-Sätze nur, wenn kein eingebauter passt.
func detect(text string, pick func(*Set) []string) *Set {
	if text == "" {
		return nil
	}
	mu.RLock()
	defer mu.RUnlock()
	for _, group := range [][]*Set{reg.builtin, reg.user} {
		for _, s := range group {
			for _, p := range pick(s) {
				if reg.match(p, text) {
					return s
				}
			}
		}
	}
	return nil
}

// DetectBanner: Satz zur SSH-Kennung.
func DetectBanner(banner string) *Set {
	return detect(banner, func(s *Set) []string { return s.Detect.Banner })
}

// DetectScreen: Satz zum Terminaltext.
func DetectScreen(text string) *Set {
	return detect(text, func(s *Set) []string { return s.Detect.Screen })
}

// DetectVersion: Satz zur Ausgabe des Versionsbefehls (auch Bildschirmmuster).
func DetectVersion(out string) *Set {
	if s := detect(out, func(s *Set) []string { return s.Detect.Version }); s != nil {
		return s
	}
	return detect(out, func(s *Set) []string {
		if s.EffFamily() != FamilyNetwork {
			return nil
		}
		return s.Detect.Screen
	})
}

// ProbePaths: Dateien, an denen man Server-Sätze erkennt (Pfad → Kennung),
// in Erkennungsreihenfolge.
func ProbePaths() [][2]string {
	var out [][2]string
	seen := map[string]bool{}
	for _, s := range All() {
		for _, p := range s.Detect.Probe {
			if reProbePath.MatchString(p) && !seen[p] {
				seen[p] = true
				out = append(out, [2]string{p, s.ID})
			}
		}
	}
	return out
}

// ---------------------------------------------------------------- Sicherheit

// BuiltinRead: steht der ganze Befehl auf der Leseliste eines eingebauten
// Satzes? Eigene Sätze können keine Lesebefehle freigeben.
func BuiltinRead(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" || strings.ContainsAny(cmd, "\r\n") {
		return false
	}
	mu.RLock()
	defer mu.RUnlock()
	for _, s := range reg.builtin {
		for _, p := range s.Read {
			if reg.match(p, cmd) {
				return true
			}
		}
	}
	return false
}

// DangerFinding: Treffer einer zusätzlichen Gefahrenregel.
type DangerFinding struct {
	Reason string
	From   string // Kennung des Satzes
}

// CheckDanger prüft einen Befehl gegen die zusätzlichen Gefahrenregeln
// aller aktiven Sätze (egal, welcher Satz zum Gerät gehört: nur ergänzen).
func CheckDanger(cmd string) []DangerFinding {
	mu.RLock()
	defer mu.RUnlock()
	var out []DangerFinding
	for _, d := range reg.danger {
		if d.rx.MatchString(cmd) {
			r := d.reason
			if i18n.English() {
				r = i18n.T(r)
				if d.reasonEN != "" {
					r = d.reasonEN
				}
			}
			out = append(out, DangerFinding{Reason: r, From: d.from})
		}
	}
	return out
}

// ---------------------------------------------------------------- eigene Sätze

// DirName: Unterordner im Konfigordner.
const DirName = "befehlssaetze"

// TeamDir: Unterordner für Sätze aus dem Team.
const TeamDir = "team"

// SyncDir: Unterordner für Sätze, die der Sync von anderen Rechnern bringt.
// Sie wirken erst, wenn man sie übernimmt (dann als eigener Satz).
const SyncDir = "sync"

// Ext: Dateiendung.
const Ext = ".yaml"

// Loaded: Ergebnis von LoadUser (eigene und Team-Dateien).
type Loaded struct {
	Own, Team, Sync []*Set
	Errs            []LoadError
}

// ReadDir liest eigene (dir) und Team-Sätze (dir/team).
func ReadDir(dir string) Loaded {
	var l Loaded
	read := func(d string, src Source) []*Set {
		ents, err := os.ReadDir(d)
		if err != nil {
			return nil
		}
		var out []*Set
		for _, e := range ents {
			if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), Ext) {
				continue
			}
			p := filepath.Join(d, e.Name())
			info, err := e.Info()
			if err != nil || info.Size() > MaxFileSize {
				l.Errs = append(l.Errs, LoadError{File: p, Err: fmt.Sprintf(i18n.T("Datei zu groß (höchstens %d KB)"), MaxFileSize>>10)})
				continue
			}
			raw, err := os.ReadFile(p)
			if err != nil {
				l.Errs = append(l.Errs, LoadError{File: p, Err: err.Error()})
				continue
			}
			s, err := ParseFile(raw)
			if err != nil {
				l.Errs = append(l.Errs, LoadError{File: p, Err: err.Error()})
				continue
			}
			s.Source, s.File = src, p
			if src == Team || src == Synced {
				s.From = readFrom(raw)
			}
			out = append(out, s)
		}
		return out
	}
	l.Own = read(dir, Own)
	l.Team = read(filepath.Join(dir, TeamDir), Team)
	l.Sync = read(filepath.Join(dir, SyncDir), Synced)
	return l
}

// readFrom: Absender aus der Kommentarzeile „# von: …“ am Dateianfang.
func readFrom(raw []byte) string {
	first, _, _ := strings.Cut(string(raw), "\n")
	if v, ok := strings.CutPrefix(strings.TrimSpace(first), "# von:"); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// Install übernimmt eigene und Team-Sätze. approved: Kennung → Prüfsumme
// des freigegebenen Inhalts. Team-Sätze ohne passende Freigabe bleiben
// inaktiv. selfTest (optional) prüft die Beispiele eines Satzes.
func Install(l Loaded, approved map[string]string, selfTest func(*Set) []string) {
	r := &registry{rx: map[string]*regexp.Regexp{}}
	mu.RLock()
	r.builtin = reg.builtin
	mu.RUnlock()
	for _, s := range r.builtin {
		r.all = append(r.all, Status{Set: s, Active: true})
	}
	r.errs = append(r.errs, l.Errs...)
	taken := map[string]bool{}
	for _, s := range r.builtin {
		taken[s.ID] = true
	}
	// Zuerst prüfen, dann Grundlagen auflösen (eigene Sätze können auf
	// anderen eigenen aufbauen).
	try := func(s *Set) (st Status) {
		st.Set = s
		switch {
		case taken[s.ID]:
			st.Shadowed = true
			if IsBuiltinID(r.builtin, s.ID) {
				st.Problem = i18n.T("Kennung gehört zu einem eingebauten Satz; bitte umbenennen")
			} else {
				st.Problem = i18n.T("Kennung schon vergeben")
			}
			return st
		}
		if err := s.Validate(); err != nil {
			st.Problem = err.Error()
			return st
		}
		return st
	}
	var cands, syncSts []Status
	for _, s := range l.Own {
		st := try(s)
		if st.Problem == "" {
			taken[s.ID] = true
		}
		cands = append(cands, st)
	}
	for _, s := range l.Team {
		st := try(s)
		if st.Problem == "" {
			sum, ok := approved[s.ID]
			switch {
			case !ok:
				st.Pending = true
				st.Problem = i18n.T("wartet auf Freigabe")
			case sum != s.Sum:
				st.Pending, st.Changed = true, true
				st.Problem = i18n.T("vom Team geändert, wartet auf Freigabe")
			default:
				taken[s.ID] = true
			}
		}
		cands = append(cands, st)
	}
	// Vom Sync: nie aktiv, nur zur Übernahme angeboten.
	own := map[string]bool{}
	for _, s := range l.Own {
		own[s.ID] = true
	}
	for _, s := range l.Sync {
		st := Status{Set: s, Pending: true}
		switch {
		case IsBuiltinID(r.builtin, s.ID):
			st.Pending, st.Shadowed = false, true
			st.Problem = i18n.T("Kennung gehört zu einem eingebauten Satz; bitte umbenennen")
		default:
			if err := s.Validate(); err != nil {
				st.Pending, st.Problem = false, err.Error()
			} else if own[s.ID] {
				st.Changed = true
				st.Problem = i18n.T("auf einem anderen Rechner geändert, wartet auf Übernahme")
			} else {
				st.Problem = i18n.T("vom Sync, wartet auf Übernahme")
			}
		}
		syncSts = append(syncSts, st)
	}
	for i := range cands {
		if cands[i].Problem == "" {
			r.user = append(r.user, cands[i].Set)
		}
	}
	// Grundlage muss existieren und darf keinen Kreis bilden.
	for changed := true; changed; {
		changed = false
		for i := range cands {
			st := &cands[i]
			if st.Problem != "" || st.Set.Base == "" {
				continue
			}
			if msg := r.baseProblem(st.Set); msg != "" {
				st.Problem = msg
				r.user = without(r.user, st.Set)
				changed = true
			}
		}
	}
	// Familie erbt von der Grundlage (für Erkennung, Anzeige, Nur-lesen-Regeln).
	for _, u := range r.user {
		if u.Family == "" && u.Base != "" {
			if res := r.resolve(u.ID, "", ""); res != nil {
				u.famEff = res.Family
			}
		}
	}
	if selfTest != nil {
		for i := range cands {
			st := &cands[i]
			if st.Problem != "" || len(st.Set.Samples) == 0 {
				continue
			}
			if fails := selfTest(st.Set); len(fails) > 0 {
				// aktiv bleiben (Erkennung kann trotzdem stimmen), aber zeigen
				st.Problem = i18n.T("Selbsttest: ") + strings.Join(fails, "; ")
			}
		}
	}
	for _, st := range cands {
		st.Active = !st.Pending && !st.Shadowed && containsSet(r.user, st.Set)
		r.all = append(r.all, st)
	}
	r.all = append(r.all, syncSts...)
	r.compile()
	mu.Lock()
	reg = r
	mu.Unlock()
}

// IsBuiltinID: gehört id zu einem der Sätze?
func IsBuiltinID(list []*Set, id string) bool {
	for _, s := range list {
		if s.ID == id {
			return true
		}
	}
	return false
}

func (r *registry) baseProblem(s *Set) string {
	seen := map[string]bool{s.ID: true}
	cur := s
	for depth := 0; cur.Base != ""; depth++ {
		if depth >= maxBaseDepth {
			return i18n.T("Grundlage zu tief verschachtelt")
		}
		if seen[cur.Base] {
			return i18n.T("Grundlagen bilden einen Kreis")
		}
		b, ok := r.lookup(cur.Base)
		if !ok {
			return fmt.Sprintf(i18n.T("Grundlage „%s“ fehlt"), cur.Base)
		}
		seen[b.ID] = true
		cur = b
	}
	return ""
}

func without(list []*Set, s *Set) []*Set {
	out := list[:0:0]
	for _, x := range list {
		if x != s {
			out = append(out, x)
		}
	}
	return out
}

func containsSet(list []*Set, s *Set) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// RunSelfTests prüft die Beispiele aller aktiven eigenen und Team-Sätze und
// merkt Abweichungen in der Liste. Die Sätze bleiben aktiv: die Erkennung
// kann trotzdem stimmen, der Nutzer sieht aber, was nicht passt.
func RunSelfTests(test func(*Set) []string) {
	mu.RLock()
	var todo []*Set
	for _, st := range reg.all {
		if st.Active && st.Set.Source != Builtin && len(st.Set.Samples) > 0 {
			todo = append(todo, st.Set)
		}
	}
	mu.RUnlock()
	res := map[*Set]string{}
	for _, s := range todo {
		if fails := test(s); len(fails) > 0 {
			res[s] = i18n.T("Selbsttest: ") + strings.Join(fails, "; ")
		}
	}
	mu.Lock()
	for i := range reg.all {
		if msg, ok := res[reg.all[i].Set]; ok {
			reg.all[i].Problem = msg
		}
	}
	mu.Unlock()
}

// Marshal schreibt einen Satz als YAML (für den Editor und den Export).
func Marshal(s *Set) ([]byte, error) {
	var b strings.Builder
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(s); err != nil {
		return nil, err
	}
	_ = enc.Close()
	return []byte(b.String()), nil
}

// ParseOwn liest eine Datei als eigenen Satz und prüft sie (Import, Editor,
// KI-Entwurf). Eine Kennung, die schon ein eingebauter Satz hat, gilt nicht.
func ParseOwn(raw []byte) (*Set, error) {
	s, err := ParseFile(raw)
	if err != nil {
		return nil, err
	}
	s.Source = Own
	if IsBuiltin(s.ID) {
		return s, fmt.Errorf(i18n.T("Kennung „%s“ gehört zu einem eingebauten Satz; bitte eine eigene wählen (z. B. %s-eigen)"), s.ID, s.ID)
	}
	if err := s.Validate(); err != nil {
		return s, err
	}
	if s.Base != "" {
		if _, ok := Lookup(s.Base); !ok {
			return s, fmt.Errorf(i18n.T("Grundlage „%s“ fehlt"), s.Base)
		}
	}
	return s, nil
}
