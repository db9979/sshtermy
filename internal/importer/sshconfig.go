package importer

import (
	"bufio"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/db9979/sshtermy/internal/domain"
)

// OpenSSH-Konfiguration: Host-Blöcke mit konkreten Namen werden zu
// Verbindungen. Wie bei OpenSSH gilt pro Option der erste passende Wert,
// auch aus späteren Mustern wie „Host *“ oder „Host *.lan“. Include wird
// aufgelöst (relativ zu ~/.ssh), Match-Blöcke werden übersprungen.

type sshBlock struct {
	patterns []string // nil = globaler Teil vor dem ersten Host
	match    bool
	opts     [][2]string
}

// ParseSSHConfig liest den Inhalt einer ssh_config. file dient nur zum
// Auflösen relativer Include-Pfade.
func ParseSSHConfig(txt, file string) ([]Entry, error) {
	var blocks []*sshBlock
	cur := &sshBlock{}
	blocks = append(blocks, cur)
	seen := map[string]bool{}
	var parse func(txt, file string, depth int)
	parse = func(txt, file string, depth int) {
		sc := bufio.NewScanner(strings.NewReader(txt))
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			k, args := splitOpt(sc.Text())
			if k == "" {
				continue
			}
			switch k {
			case "host":
				cur = &sshBlock{patterns: args}
				blocks = append(blocks, cur)
			case "match":
				cur = &sshBlock{match: true}
				blocks = append(blocks, cur)
			case "include":
				if depth > 8 {
					continue
				}
				for _, pat := range args {
					for _, f := range includeFiles(pat, file) {
						if seen[f] {
							continue
						}
						seen[f] = true
						if b, err := os.ReadFile(f); err == nil {
							parse(decodeText(b), f, depth+1)
						}
					}
				}
			default:
				cur.opts = append(cur.opts, [2]string{k, strings.Join(args, " ")})
			}
		}
	}
	parse(txt, file, 0)

	var out []Entry
	done := map[string]bool{}
	for _, b := range blocks {
		for _, alias := range b.patterns {
			if strings.ContainsAny(alias, "*?!") || done[strings.ToLower(alias)] {
				continue
			}
			done[strings.ToLower(alias)] = true
			out = append(out, resolveSSH(alias, blocks))
		}
	}
	return out, nil
}

func resolveSSH(alias string, blocks []*sshBlock) Entry {
	opt := map[string]string{}
	var ids []string
	for _, b := range blocks {
		if b.match || (b.patterns != nil && !hostMatches(alias, b.patterns)) {
			continue
		}
		for _, o := range b.opts {
			if o[0] == "identityfile" {
				ids = append(ids, o[1])
				continue
			}
			if _, ok := opt[o[0]]; !ok {
				opt[o[0]] = o[1]
			}
		}
	}
	e := Entry{Source: SrcSSHConfig, Conn: domain.Connection{
		Alias: alias, Host: alias, Port: 22, Auth: domain.AuthAgent, Tags: []string{"ssh-config"},
	}}
	if h := opt["hostname"]; h != "" {
		e.Conn.Host = strings.ReplaceAll(h, "%h", alias)
	}
	if p, err := strconv.Atoi(opt["port"]); err == nil && p > 0 {
		e.Conn.Port = p
	}
	e.Conn.Username = opt["user"]
	fillUser(&e)
	if len(ids) > 0 && strings.ToLower(ids[0]) != "none" {
		keyAuth(&e, expandTokens(unquote(ids[0]), e.Conn))
	}
	if j := opt["proxyjump"]; j != "" && strings.ToLower(j) != "none" {
		first := strings.TrimSpace(strings.Split(j, ",")[0])
		e.Jump = first
		if strings.Contains(j, ",") {
			e.Notes = append(e.Notes, "ProxyJump mit mehreren Stationen: nur „"+first+"“ übernommen")
		}
	} else if pc := opt["proxycommand"]; pc != "" && strings.ToLower(pc) != "none" {
		e.Notes = append(e.Notes, "ProxyCommand wird nicht unterstützt (nur ProxyJump)")
	}
	for _, k := range []string{"localforward", "remoteforward", "dynamicforward"} {
		if opt[k] != "" {
			e.Notes = append(e.Notes, "Port-Weiterleitungen bitte als Tunnel (t) anlegen")
			break
		}
	}
	return e
}

// splitOpt: „Key Wert“, „Key=Wert“, Kommentare und Anführungszeichen.
func splitOpt(line string) (string, []string) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", nil
	}
	i := strings.IndexAny(line, " \t=")
	if i < 0 {
		return strings.ToLower(line), nil
	}
	k := strings.ToLower(line[:i])
	rest := strings.TrimLeft(line[i:], " \t")
	rest = strings.TrimLeft(strings.TrimPrefix(rest, "="), " \t")
	return k, fields(rest)
}

// fields trennt an Leerzeichen, respektiert "…" und schneidet Kommentare ab.
func fields(s string) []string {
	var out []string
	var b strings.Builder
	inQ, has := false, false
	for _, r := range s {
		switch {
		case r == '"':
			inQ, has = !inQ, true
		case !inQ && (r == ' ' || r == '\t'):
			if has {
				out = append(out, b.String())
				b.Reset()
				has = false
			}
		case !inQ && r == '#' && !has:
			return out
		default:
			b.WriteRune(r)
			has = true
		}
	}
	if has {
		out = append(out, b.String())
	}
	return out
}

func unquote(s string) string { return strings.Trim(s, `"`) }

// hostMatches: OpenSSH-Muster mit * und ?, „!“ schließt aus.
func hostMatches(host string, pats []string) bool {
	host = strings.ToLower(host)
	ok := false
	for _, p := range pats {
		neg := strings.HasPrefix(p, "!")
		p = strings.ToLower(strings.TrimPrefix(p, "!"))
		if m, _ := path.Match(p, host); m {
			if neg {
				return false
			}
			ok = true
		}
	}
	return ok
}

func includeFiles(pat, from string) []string {
	pat = expandHome(unquote(pat))
	if !filepath.IsAbs(pat) {
		home, _ := os.UserHomeDir()
		base := filepath.Join(home, ".ssh")
		if from != "" && !strings.HasPrefix(filepath.Clean(from), base) {
			base = filepath.Dir(from)
		}
		pat = filepath.Join(base, pat)
	}
	m, _ := filepath.Glob(pat)
	return m
}

// expandTokens: die üblichen %-Platzhalter in IdentityFile.
func expandTokens(p string, c domain.Connection) string {
	home, _ := os.UserHomeDir()
	r := strings.NewReplacer("%d", home, "%h", c.Host, "%r", c.Username, "%u", localUser(), "%n", c.Alias, "%%", "%")
	p = r.Replace(p)
	if strings.HasPrefix(p, "~") {
		p = expandHome(p)
	}
	return p
}
