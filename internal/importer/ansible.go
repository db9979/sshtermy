package importer

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Ansible-Inventar, als INI oder YAML:
//
//	[web]                              all:
//	web1 ansible_host=10.0.0.1           children:
//	web[01:03].example.com                 web:
//	                                         hosts:
//	[core:vars]                                web1:
//	ansible_network_os=arubaoss                  ansible_host: 10.0.0.1
//
// Übernommen werden Name, ansible_host, ansible_port, ansible_user,
// ansible_ssh_private_key_file und die Gruppe als Ordner (weitere Gruppen
// als Tags). ansible_network_os oder ansible_connection=network_cli macht
// daraus ein Netzwerkgerät. Gruppen-Variablen (:vars, vars:) gelten für
// alle Hosts der Gruppe. Passwörter (ansible_password) werden nie übernommen.

// LooksLikeAnsible: Ist txt ein Ansible-Inventar?
func LooksLikeAnsible(txt, path string) bool {
	l := strings.ToLower(txt)
	p := strings.ToLower(path)
	if strings.Contains(l, "ansible_host") || strings.Contains(l, "ansible_user") || strings.Contains(l, "ansible_network_os") {
		return true
	}
	if strings.Contains(p, "inventory") || strings.HasSuffix(p, "/hosts") || strings.HasSuffix(p, `\hosts`) {
		return true
	}
	return isYAMLInventory(txt)
}

func isYAMLInventory(txt string) bool {
	for _, line := range strings.Split(txt, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") || t == "---" {
			continue
		}
		return t == "all:" || strings.HasSuffix(t, ":") && strings.Contains(txt, "hosts:")
	}
	return false
}

// invHost: ein Host mit seinen Variablen und Gruppen.
type invHost struct {
	name   string
	vars   map[string]string
	groups []string
}

type inventory struct {
	order     []string
	hosts     map[string]*invHost
	groupVars map[string]map[string]string
	parents   map[string][]string // Gruppe → Eltern (children)
}

func newInventory() *inventory {
	return &inventory{hosts: map[string]*invHost{}, groupVars: map[string]map[string]string{}, parents: map[string][]string{}}
}

func (inv *inventory) host(name, group string) *invHost {
	h := inv.hosts[name]
	if h == nil {
		h = &invHost{name: name, vars: map[string]string{}}
		inv.hosts[name] = h
		inv.order = append(inv.order, name)
	}
	if group != "" && group != "all" && group != "ungrouped" {
		for _, g := range h.groups {
			if g == group {
				return h
			}
		}
		h.groups = append(h.groups, group)
	}
	return h
}

func (inv *inventory) setGroupVar(group, k, v string) {
	if inv.groupVars[group] == nil {
		inv.groupVars[group] = map[string]string{}
	}
	inv.groupVars[group][k] = v
}

// ParseAnsible liest ein Inventar (INI oder YAML).
func ParseAnsible(txt string) ([]Entry, error) {
	inv := newInventory()
	if isYAMLInventory(txt) {
		parseAnsibleYAML(txt, inv)
	} else {
		parseAnsibleINI(txt, inv)
	}
	return inv.entries(), nil
}

func parseAnsibleINI(txt string, inv *inventory) {
	section, kind := "ungrouped", ""
	sc := bufio.NewScanner(strings.NewReader(txt))
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section, kind = line[1:len(line)-1], ""
			if i := strings.Index(section, ":"); i >= 0 {
				section, kind = section[:i], section[i+1:]
			}
			continue
		}
		switch kind {
		case "vars":
			if k, v, ok := strings.Cut(line, "="); ok {
				inv.setGroupVar(section, strings.TrimSpace(k), unquoteVal(strings.TrimSpace(v)))
			}
			continue
		case "children":
			child := strings.Fields(line)[0]
			inv.parents[child] = append(inv.parents[child], section)
			continue
		}
		fields := splitArgs(line)
		if len(fields) == 0 {
			continue
		}
		for _, name := range expandRange(fields[0]) {
			h := inv.host(name, section)
			for _, f := range fields[1:] {
				if k, v, ok := strings.Cut(f, "="); ok {
					h.vars[k] = unquoteVal(v)
				}
			}
		}
	}
}

// parseAnsibleYAML: genug YAML für Inventare (Einrückung, „key: value“,
// keine Listen, keine Anker).
func parseAnsibleYAML(txt string, inv *inventory) {
	type frame struct {
		indent int
		key    string
	}
	var stack []frame
	for _, raw := range strings.Split(strings.ReplaceAll(txt, "\t", "  "), "\n") {
		line := raw
		if i := strings.Index(line, " #"); i >= 0 {
			line = line[:i]
		}
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") || t == "---" || strings.HasPrefix(t, "- ") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		k, v, ok := strings.Cut(t, ":")
		if !ok {
			continue
		}
		k, v = unquoteVal(strings.TrimSpace(k)), unquoteVal(strings.TrimSpace(v))
		for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		path := make([]string, len(stack))
		for i, f := range stack {
			path[i] = f.key
		}
		n := len(path)
		switch {
		case n >= 1 && path[n-1] == "hosts":
			// Host direkt unter „hosts:“, Gruppe ist der Schlüssel davor
			inv.host(k, groupAt(path, n-2))
		case n >= 2 && path[n-2] == "hosts" && v != "":
			inv.host(path[n-1], groupAt(path, n-3)).vars[k] = v
		case n >= 1 && path[n-1] == "vars" && v != "":
			inv.setGroupVar(groupAt(path, n-2), k, v)
		case n >= 1 && path[n-1] == "children":
			if g := groupAt(path, n-2); g != "" {
				inv.parents[k] = append(inv.parents[k], g)
			}
		}
		stack = append(stack, frame{indent, k})
	}
}

func groupAt(path []string, i int) string {
	if i < 0 || i >= len(path) {
		return "all"
	}
	return path[i]
}

// varOf: Variable des Hosts, sonst aus seinen Gruppen (auch geerbt).
func (inv *inventory) varOf(h *invHost, k string) string {
	if v, ok := h.vars[k]; ok {
		return v
	}
	seen := map[string]bool{}
	var walk func(g string) (string, bool)
	walk = func(g string) (string, bool) {
		if seen[g] {
			return "", false
		}
		seen[g] = true
		if v, ok := inv.groupVars[g][k]; ok {
			return v, true
		}
		for _, p := range inv.parents[g] {
			if v, ok := walk(p); ok {
				return v, true
			}
		}
		return "", false
	}
	for _, g := range h.groups {
		if v, ok := walk(g); ok {
			return v
		}
	}
	return inv.groupVars["all"][k]
}

func (inv *inventory) entries() []Entry {
	var out []Entry
	for _, name := range inv.order {
		h := inv.hosts[name]
		host := orStr(inv.varOf(h, "ansible_host"), inv.varOf(h, "ansible_ssh_host"), name)
		c := domain.Connection{Alias: name, Host: host, Port: 22, Username: orStr(inv.varOf(h, "ansible_user"), inv.varOf(h, "ansible_ssh_user")),
			Auth: domain.AuthAgent, Tags: []string{"ansible"}}
		if p, err := strconv.Atoi(orStr(inv.varOf(h, "ansible_port"), inv.varOf(h, "ansible_ssh_port"))); err == nil && p > 0 {
			c.Port = p
		}
		if len(h.groups) > 0 {
			c.Group = h.groups[0]
			c.Tags = append(c.Tags, h.groups[1:]...)
		}
		e := Entry{Conn: c, Source: SrcAnsible}
		if os := inv.varOf(h, "ansible_network_os"); os != "" || inv.varOf(h, "ansible_connection") == "network_cli" {
			e.Conn.Device = domain.DeviceNetwork
			if os != "" {
				e.Notes = append(e.Notes, fmt.Sprintf(i18n.T("Netzwerkgerät (%s)"), os))
			}
		}
		keyAuth(&e, inv.varOf(h, "ansible_ssh_private_key_file"))
		if inv.varOf(h, "ansible_password") != "" || inv.varOf(h, "ansible_ssh_pass") != "" {
			e.Notes = append(e.Notes, i18n.T("Passwort aus dem Inventar nicht übernommen, wird beim Verbinden gefragt"))
		}
		fillUser(&e)
		out = append(out, e)
	}
	return out
}

func orStr(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

func unquoteVal(s string) string {
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"' || s[0] == '\'' && s[len(s)-1] == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}

// splitArgs trennt an Leerzeichen, außer in Anführungszeichen.
func splitArgs(s string) []string {
	var out []string
	var b strings.Builder
	var q rune
	for _, r := range s {
		switch {
		case q != 0:
			if r == q {
				q = 0
			}
			b.WriteRune(r)
		case r == '"' || r == '\'':
			q = r
			b.WriteRune(r)
		case r == ' ' || r == '\t':
			if b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

// expandRange: web[01:03].lan → web01.lan, web02.lan, web03.lan (höchstens 256).
func expandRange(name string) []string {
	i := strings.Index(name, "[")
	j := strings.Index(name, "]")
	if i < 0 || j < i {
		return []string{name}
	}
	lo, hi, ok := strings.Cut(name[i+1:j], ":")
	if !ok {
		return []string{name}
	}
	a, err1 := strconv.Atoi(lo)
	b, err2 := strconv.Atoi(hi)
	if err1 != nil || err2 != nil || b < a || b-a > 255 {
		return []string{name}
	}
	var out []string
	for n := a; n <= b; n++ {
		out = append(out, name[:i]+fmt.Sprintf("%0*d", len(lo), n)+name[j+1:])
	}
	return out
}
