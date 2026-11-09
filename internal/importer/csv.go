package importer

import (
	"bytes"
	"encoding/csv"
	"errors"
	"strconv"
	"strings"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// CSV: eine Zeile je Verbindung, erste Zeile mit Spaltennamen. Erkannt
// werden übliche Namen (deutsch und englisch) und der CSV-Export von NetBox
// (Name, Site, Role, Platform, Primary IPv4 „10.0.0.1/24“). Trennzeichen
// Komma oder Semikolon (Excel mit deutscher Einstellung). Das Format von
// ExportCSV liest sshTermy unverändert wieder ein.

// csvColumns: Spaltenname (klein, ohne Leer-/Sonderzeichen) → Feld.
var csvColumns = map[string]string{
	"alias": "alias", "name": "alias", "anzeigename": "alias", "displayname": "alias", "label": "alias", "device": "alias", "gerät": "alias",
	"host": "host", "hostname": "host", "ip": "host", "ipaddress": "host", "ipadresse": "host", "address": "host", "adresse": "host",
	"primaryip": "host", "primaryipv4": "host", "primaryip4": "host", "primaryipv6": "host6", "primaryip6": "host6", "mgmtip": "host", "managementip": "host", "fqdn": "host", "dnsname": "host",
	"port": "port", "sshport": "port",
	"user": "user", "username": "user", "benutzer": "user", "login": "user",
	"group": "group", "folder": "group", "ordner": "group", "gruppe": "group", "path": "group",
	"site": "site", "standort": "site", "location": "site", "tenant": "site",
	"tags": "tags", "tag": "tags",
	"role": "role", "devicerole": "role", "rolle": "role", "type": "role", "devicetype": "role", "platform": "role", "plattform": "role", "manufacturer": "role", "os": "role",
	"protocol": "protocol", "protokoll": "protocol",
	"jump": "jump", "jumphost": "jump", "proxyjump": "jump",
	"keyfile": "key", "key": "key", "identityfile": "key",
	"devicekind": "kind", "gerätetyp": "kind",
}

// netWords: Hinweise in Rolle/Plattform/Typ auf ein Netzwerkgerät.
var netWords = []string{"switch", "router", "firewall", "aruba", "procurve", "cisco", "ios", "nxos", "junos", "juniper", "mikrotik", "routeros", "comware", "fortigate", "fortinet", "unifi", "ubiquiti", "hpe", "access point", "accesspoint", "core", "distribution"}

func normCol(s string) string {
	s = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(s, "\ufeff")))
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '_' || r == '-' || r == '.' || r == '/' {
			return -1
		}
		return r
	}, s)
}

// csvDelim: Semikolon, wenn die Kopfzeile mehr davon hat.
func csvDelim(txt string) rune {
	head, _, _ := strings.Cut(txt, "\n")
	if strings.Count(head, ";") > strings.Count(head, ",") {
		return ';'
	}
	if strings.Count(head, "\t") > strings.Count(head, ",") {
		return '\t'
	}
	return ','
}

// LooksLikeCSV: .csv-Datei oder Kopfzeile mit bekannten Spalten.
func LooksLikeCSV(txt, path string) bool {
	if strings.HasSuffix(strings.ToLower(path), ".csv") {
		return true
	}
	head, _, _ := strings.Cut(txt, "\n")
	d := csvDelim(txt)
	cols := strings.Split(head, string(d))
	if len(cols) < 2 {
		return false
	}
	hit := map[string]bool{}
	for _, c := range cols {
		if f, ok := csvColumns[normCol(strings.Trim(c, `"`))]; ok {
			hit[f] = true
		}
	}
	return hit["host"] && (hit["alias"] || hit["user"] || hit["port"])
}

// ParseCSV liest Verbindungen aus einer CSV-Datei.
func ParseCSV(txt string) ([]Entry, error) {
	r := csv.NewReader(strings.NewReader(txt))
	r.Comma = csvDelim(txt)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.TrimLeadingSpace = true
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 1 {
		return nil, nil
	}
	idx := map[string]int{}
	for i, h := range rows[0] {
		if f, ok := csvColumns[normCol(h)]; ok {
			if _, dup := idx[f]; !dup {
				idx[f] = i
			}
		}
	}
	_, hasHost := idx["host"]
	_, hasAlias := idx["alias"]
	if !hasHost && !hasAlias {
		return nil, errors.New(i18n.T("CSV ohne Spalte für Host oder Name (z. B. „host“, „ip“, „name“)"))
	}
	var out []Entry
	for _, row := range rows[1:] {
		get := func(f string) string {
			if i, ok := idx[f]; ok && i < len(row) {
				return strings.TrimSpace(row[i])
			}
			return ""
		}
		host := orStr(get("host"), get("host6"))
		if i := strings.Index(host, "/"); i > 0 {
			host = host[:i] // NetBox: 10.0.0.1/24
		}
		alias := get("alias")
		if host == "" {
			host = alias
		}
		if host == "" {
			continue
		}
		if alias == "" {
			alias = host
		}
		c := domain.Connection{Alias: alias, Host: host, Port: 22, Username: get("user"), Auth: domain.AuthAgent, Tags: []string{"csv"}}
		if p, err := strconv.Atoi(get("port")); err == nil && p > 0 && p < 65536 {
			c.Port = p
		}
		switch strings.ToLower(get("protocol")) {
		case "telnet":
			c.Protocol, c.Auth = domain.ProtoTelnet, domain.AuthPassword
			if get("port") == "" {
				c.Port = 23
			}
		}
		c.Group = get("group")
		if site := get("site"); site != "" {
			c.Group = strings.Trim(c.Group+" / "+site, " /")
		}
		c.Group = domain.NormPath(strings.ReplaceAll(c.Group, `\`, "/"))
		for _, t := range strings.FieldsFunc(get("tags"), func(r rune) bool { return r == ',' || r == ';' || r == '|' }) {
			if t = strings.TrimSpace(t); t != "" {
				c.Tags = append(c.Tags, t)
			}
		}
		role := strings.ToLower(get("role"))
		switch strings.ToLower(get("kind")) {
		case string(domain.DeviceNetwork), "netzwerkgerät", "network device":
			c.Device = domain.DeviceNetwork
		case string(domain.DeviceServer):
			c.Device = domain.DeviceServer
		default:
			for _, w := range netWords {
				if role != "" && strings.Contains(role, w) {
					c.Device = domain.DeviceNetwork
					break
				}
			}
		}
		e := Entry{Conn: c, Source: SrcCSV, Jump: get("jump")}
		keyAuth(&e, get("key"))
		if c.Protocol != domain.ProtoTelnet {
			fillUser(&e)
		}
		out = append(out, e)
	}
	return out, nil
}

// ExportCSV schreibt Verbindungen als CSV (ohne Passwörter, Keys oder
// Passphrasen). jump liefert den Alias eines Jump-Hosts zur ID.
func ExportCSV(conns []domain.Connection, jump func(id string) string) []byte {
	var b bytes.Buffer
	b.WriteString("\ufeff") // Excel erkennt UTF-8 nur mit BOM
	w := csv.NewWriter(&b)
	_ = w.Write([]string{"alias", "host", "port", "user", "protocol", "folder", "tags", "device_kind", "jump_host", "keyfile"})
	for _, c := range conns {
		proto := string(c.Protocol)
		if proto == "" {
			proto = "ssh"
		}
		port := ""
		if c.Port != 0 {
			port = strconv.Itoa(c.Port)
		}
		j := ""
		if c.JumpHostID != "" && jump != nil {
			j = jump(c.JumpHostID)
		}
		host := c.Host
		if c.Protocol == domain.ProtoSerial {
			host = c.SerialPort
		}
		_ = w.Write([]string{c.Alias, host, port, c.Username, proto, c.Group, strings.Join(c.Tags, ";"), string(c.Device), j, c.KeyPath})
	}
	w.Flush()
	return b.Bytes()
}
