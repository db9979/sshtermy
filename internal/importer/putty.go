package importer

import (
	"bufio"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/db9979/sshtermy/internal/domain"
)

// PuTTY speichert Sitzungen unter
// HKCU\Software\SimonTatham\PuTTY\Sessions\<Name, URL-kodiert>. Unter
// Windows liest sshTermy die Registry direkt, sonst einen .reg-Export
// (regedit → Exportieren, oder „reg export HKCU\Software\SimonTatham\PuTTY putty.reg“).

const puttyKey = `Software\SimonTatham\PuTTY\Sessions`

// puttySession: Werte einer Sitzung (REG_SZ als string, DWORD als int).
type puttySession map[string]any

func (s puttySession) str(k string) string {
	v, _ := s[k].(string)
	return strings.TrimSpace(v)
}

func (s puttySession) num(k string) int {
	switch v := s[k].(type) {
	case int:
		return v
	case string:
		n, _ := strconv.Atoi(v)
		return n
	}
	return 0
}

func puttyEntries(sessions map[string]puttySession) []Entry {
	names := make([]string, 0, len(sessions))
	for n := range sessions {
		names = append(names, n)
	}
	sort.Strings(names)
	var out []Entry
	for _, raw := range names {
		s := sessions[raw]
		name, err := url.PathUnescape(raw)
		if err != nil {
			name = raw
		}
		proto := strings.ToLower(s.str("Protocol"))
		host := s.str("HostName")
		if host == "" || (proto != "" && proto != "ssh") {
			continue // „Default Settings“, Telnet, seriell …
		}
		e := Entry{Source: SrcPuTTY, Conn: domain.Connection{
			Alias: name, Host: host, Port: s.num("PortNumber"), Username: s.str("UserName"),
			Auth: domain.AuthPassword, Tags: []string{"putty"},
		}}
		if i := strings.LastIndex(host, "@"); i > 0 && e.Conn.Username == "" {
			e.Conn.Username, e.Conn.Host = host[:i], host[i+1:]
		}
		if e.Conn.Port == 0 {
			e.Conn.Port = 22
		}
		fillUser(&e)
		keyAuth(&e, s.str("PublicKeyFile"))
		if ph := s.str("ProxyHost"); ph != "" && s.num("ProxyMethod") != 0 {
			e.Notes = append(e.Notes, "Proxy "+ph+" nicht übernommen")
		}
		if s.str("PortForwardings") != "" {
			e.Notes = append(e.Notes, "Port-Weiterleitungen bitte als Tunnel (t) anlegen")
		}
		out = append(out, e)
	}
	return out
}

// ParsePuTTYReg liest einen .reg-Export (bereits zu UTF-8 dekodiert).
func ParsePuTTYReg(txt string) ([]Entry, error) {
	sessions := map[string]puttySession{}
	var cur puttySession
	sc := bufio.NewScanner(strings.NewReader(txt))
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	marker := `\` + puttyKey + `\`
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			cur = nil
			sec := line[1 : len(line)-1]
			if i := strings.Index(sec, marker); i >= 0 {
				name := sec[i+len(marker):]
				if name != "" && !strings.Contains(name, `\`) {
					cur = puttySession{}
					sessions[name] = cur
				}
			}
			continue
		}
		if cur == nil || !strings.HasPrefix(line, `"`) {
			continue
		}
		end := strings.Index(line[1:], `"=`)
		if end < 0 {
			continue
		}
		k, v := line[1:end+1], line[end+3:]
		switch {
		case strings.HasPrefix(v, `"`):
			v = strings.TrimSuffix(strings.TrimPrefix(v, `"`), `"`)
			cur[k] = strings.NewReplacer(`\\`, `\`, `\"`, `"`).Replace(v)
		case strings.HasPrefix(v, "dword:"):
			n, _ := strconv.ParseUint(strings.TrimPrefix(v, "dword:"), 16, 32)
			cur[k] = int(n)
		}
	}
	return puttyEntries(sessions), sc.Err()
}
