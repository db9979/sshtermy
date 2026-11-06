// changelog erzeugt internal/changelog/CHANGELOG.md aus der Git-Historie von
// main – in der Release-Pipeline vor dem Bauen, damit jede Programmdatei ihr
// „Was ist neu“ mitbringt.
//
// Regeln (passen zu .github/workflows/release.yml):
//   - Jeder Commit auf main (first-parent) ist ein Release, außer er ändert
//     nur docs/**, site/**, Formula/** oder *.md. Dessen Neuerungen wandern ins nächste Release.
//   - Version = MAJOR.MINOR aus VERSION + Anzahl Commits bis dahin.
//   - Text: aus dem PR-Abschnitt „## Was ist neu“ (Stichpunkte für Nutzer).
//     Fehlt er, der PR-Titel. Steht dort nur „intern“ oder „-“, entfällt der
//     PR. Commits ohne PR liefern ihre Betreffzeile.
//
// Mit GH_TOKEN/GITHUB_TOKEN werden die PR-Beschreibungen über die GitHub-API
// geholt; ohne Token (lokal) reichen die PR-Titel aus den Merge-Commits.
//
//	go run ./tools/changelog -out internal/changelog/CHANGELOG.md -notes notes.md
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type commit struct {
	sha, subject, body string
	date               time.Time
}

type release struct {
	version, date string
	items         []string
}

var (
	mergeRe  = regexp.MustCompile(`^Merge pull request #(\d+) from `)
	headRe   = regexp.MustCompile(`^#{1,4}\s*(.+?)\s*:?\s*$`)
	newsHead = regexp.MustCompile(`(?i)^(was ist neu|neu für nutzer|für nutzer|changelog|neuerungen)\b`)
)

func main() {
	out := flag.String("out", "internal/changelog/CHANGELOG.md", "Ziel-Datei")
	notes := flag.String("notes", "", "optional: Release-Notizen nur für die neueste Version")
	limit := flag.Int("limit", 60, "höchstens so viele Versionen")
	flag.Parse()

	rels, err := build(*limit)
	if err != nil {
		fmt.Fprintln(os.Stderr, "changelog:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, []byte(render(rels)), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "changelog:", err)
		os.Exit(1)
	}
	if *notes != "" && len(rels) > 0 {
		var b strings.Builder
		b.WriteString("## Was ist neu\n\n")
		for _, it := range rels[0].items {
			b.WriteString("- " + it + "\n")
		}
		if err := os.WriteFile(*notes, []byte(b.String()), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "changelog:", err)
			os.Exit(1)
		}
	}
	fmt.Printf("changelog: %d Versionen → %s\n", len(rels), *out)
}

func git(args ...string) (string, error) {
	b, err := exec.Command("git", args...).Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(b), nil
}

func build(limit int) ([]release, error) {
	raw, err := git("log", "--first-parent", "--reverse", "--format=%H%x1f%cI%x1f%s%x1f%b%x1e", "HEAD")
	if err != nil {
		return nil, err
	}
	var cs []commit
	for _, rec := range strings.Split(raw, "\x1e") {
		f := strings.Split(strings.TrimLeft(rec, "\n"), "\x1f")
		if len(f) < 4 {
			continue
		}
		d, _ := time.Parse(time.RFC3339, f[1])
		cs = append(cs, commit{sha: f[0], date: d, subject: f[2], body: strings.TrimSpace(f[3])})
	}
	gh := newGitHub()
	var rels []release
	var pending []string
	for i, c := range cs {
		pending = append(pending, itemsFor(c, gh)...)
		if i < len(cs)-1 && !releases(c, i == 0) {
			continue // nur Doku: kein Release, Punkte gehen ins nächste
		}
		n, err := git("rev-list", "--count", c.sha)
		if err != nil {
			return nil, err
		}
		mm := "1.0"
		if v, err := git("show", c.sha+":VERSION"); err == nil && strings.TrimSpace(v) != "" {
			mm = strings.TrimSpace(v)
		}
		rels = append(rels, release{
			version: mm + "." + strings.TrimSpace(n),
			date:    c.date.Local().Format("02.01.2006"),
			items:   dedupe(pending),
		})
		pending = nil
	}
	// neueste zuerst
	for i, j := 0, len(rels)-1; i < j; i, j = i+1, j-1 {
		rels[i], rels[j] = rels[j], rels[i]
	}
	if len(rels) > limit {
		rels = rels[:limit]
	}
	return rels, nil
}

// releases: würde dieser Commit den Release-Workflow auslösen?
func releases(c commit, root bool) bool {
	if root {
		return true
	}
	files, err := git("diff", "--name-only", c.sha+"^1", c.sha)
	if err != nil {
		return true
	}
	for _, f := range strings.Fields(files) {
		if !noRelease(f) {
			return true
		}
	}
	return false
}

// noRelease: Pfade aus paths-ignore in release.yml (Doku, Webseite, die
// vom Release selbst geschriebene Homebrew-Formel).
func noRelease(f string) bool {
	for _, p := range []string{"docs/", "site/", "Formula/"} {
		if strings.HasPrefix(f, p) {
			return true
		}
	}
	return strings.HasSuffix(f, ".md")
}

func itemsFor(c commit, gh *github) []string {
	m := mergeRe.FindStringSubmatch(c.subject)
	if m == nil {
		if strings.HasPrefix(c.subject, "Merge ") {
			// Merge ohne PR: die Betreffzeilen der hereingeholten Commits
			out, err := git("log", "--no-merges", "--format=%s", c.sha+"^1.."+c.sha)
			if err != nil {
				return nil
			}
			var items []string
			for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
				if l = strings.TrimSpace(l); l != "" {
					items = append([]string{l}, items...) // älteste zuerst
				}
			}
			return items
		}
		return []string{c.subject}
	}
	title := strings.TrimSpace(strings.SplitN(c.body, "\n", 2)[0])
	n, _ := strconv.Atoi(m[1])
	if body, ok := gh.prBody(n); ok {
		if items, found := newsSection(body); found {
			return items
		}
	}
	if title == "" {
		return nil
	}
	return []string{title}
}

// newsSection sucht in einer PR-Beschreibung den Abschnitt „Was ist neu“ und
// liefert dessen Stichpunkte. found = Abschnitt vorhanden (auch wenn er
// bewusst leer/„intern“ ist).
func newsSection(body string) (items []string, found bool) {
	in := false
	for _, l := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(l)
		if h := headRe.FindStringSubmatch(t); h != nil {
			if in {
				break
			}
			in = newsHead.MatchString(h[1])
			found = found || in
			continue
		}
		if !in || t == "" || strings.HasPrefix(t, "<!--") {
			continue
		}
		t = strings.TrimSpace(strings.TrimLeft(t, "-*•"))
		if t == "" || isNothing(t) {
			continue
		}
		items = append(items, t)
	}
	return items, found
}

func isNothing(t string) bool {
	switch strings.ToLower(strings.Trim(t, " .–-_*()")) {
	case "", "intern", "nichts", "keine", "keine änderung für nutzer", "n/a":
		return true
	}
	return false
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func render(rels []release) string {
	var b strings.Builder
	b.WriteString("# Was ist neu in sshTermy\n\n")
	b.WriteString("<!-- Automatisch erzeugt von tools/changelog (Release-Pipeline). Nicht von Hand bearbeiten:\n")
	b.WriteString("     Neuerungen gehören in den Abschnitt „## Was ist neu“ der PR-Beschreibung. -->\n")
	for _, r := range rels {
		if len(r.items) == 0 {
			r.items = []string{"Kleine Verbesserungen und Fehlerbehebungen"}
		}
		fmt.Fprintf(&b, "\n## v%s · %s\n", r.version, r.date)
		for _, it := range r.items {
			b.WriteString("- " + it + "\n")
		}
	}
	return b.String()
}

// ---------------------------------------------------------------- GitHub

type github struct {
	token, repo string
	hc          *http.Client
}

func newGitHub() *github {
	tok := os.Getenv("GH_TOKEN")
	if tok == "" {
		tok = os.Getenv("GITHUB_TOKEN")
	}
	repo := os.Getenv("GITHUB_REPOSITORY")
	if repo == "" {
		repo = "db9979/sshtermy"
	}
	return &github{token: tok, repo: repo, hc: &http.Client{Timeout: 15 * time.Second}}
}

func (g *github) prBody(n int) (string, bool) {
	if g.token == "" || n == 0 {
		return "", false
	}
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("https://api.github.com/repos/%s/pulls/%d", g.repo, n), nil)
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := g.hc.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", false
	}
	var pr struct {
		Body string `json:"body"`
	}
	if json.NewDecoder(resp.Body).Decode(&pr) != nil {
		return "", false
	}
	return pr.Body, true
}
