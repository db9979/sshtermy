package importer

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/db9979/sshtermy/internal/domain"
)

// MobaXterm legt Sitzungen in MobaXterm.ini (bzw. einem .mxtsessions-Export)
// in Abschnitten [Bookmarks], [Bookmarks_1] … ab. SubRep ist der Ordner,
// jede weitere Zeile eine Sitzung:
//
//	Name=#109#0%host%22%user%%-1%-1%%gateway%22%gwuser%0%0%0%keyfile%…#MobaFont%…
//
// 109 steht für SSH; die Felder nach dem zweiten # trennt „%“.

const mobaSSH = "109"

// ParseMoba liest den Inhalt einer MobaXterm.ini oder .mxtsessions-Datei.
func ParseMoba(txt string) ([]Entry, error) {
	var out []Entry
	inBook := false
	group := ""
	sc := bufio.NewScanner(strings.NewReader(txt))
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			sec := strings.ToLower(line[1 : len(line)-1])
			inBook = sec == "bookmarks" || strings.HasPrefix(sec, "bookmarks_")
			group = ""
			continue
		}
		if !inBook {
			continue
		}
		eq := strings.Index(line, "=")
		if eq <= 0 {
			continue
		}
		k, v := strings.TrimSpace(line[:eq]), strings.TrimSpace(line[eq+1:])
		switch strings.ToLower(k) {
		case "subrep":
			group = strings.ReplaceAll(v, `\`, " / ")
			continue
		case "imgnum":
			continue
		}
		parts := strings.Split(v, "#")
		if len(parts) < 3 || parts[1] != mobaSSH {
			continue
		}
		f := strings.Split(parts[2], "%")
		get := func(i int) string {
			if i < len(f) {
				return strings.TrimSpace(f[i])
			}
			return ""
		}
		if get(1) == "" {
			continue
		}
		e := Entry{Source: SrcMoba, Conn: domain.Connection{
			Alias: k, Host: get(1), Username: get(3), Port: 22, Group: group,
			Auth: domain.AuthPassword, Tags: []string{"mobaxterm"},
		}}
		if p, err := strconv.Atoi(get(2)); err == nil && p > 0 {
			e.Conn.Port = p
		}
		fillUser(&e)
		keyAuth(&e, mobaPath(get(14)))
		// Felder 8–10: SSH-Gateway (Host, Port, Benutzer)
		if gw := get(8); gw != "" && gw != "-1" && gw != "0" {
			e.Jump = gw
			if u := get(10); u != "" && u != "-1" {
				e.Jump = u + "@" + gw
			}
			if gp := get(9); gp != "" && gp != "22" && gp != "-1" && gp != "0" {
				e.Jump += ":" + gp
			}
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

// mobaPath: MobaXterm-Platzhalter in Pfaden auflösen, soweit möglich.
func mobaPath(p string) string {
	if p == "" || p == "-1" || p == "0" {
		return ""
	}
	home, _ := os.UserHomeDir()
	r := strings.NewReplacer(
		"_ProfileDir_", home,
		"_CurrentDrive_", strings.TrimSuffix(filepath.VolumeName(home), `\`),
		"_MyDocuments_", filepath.Join(home, "Documents"),
	)
	return r.Replace(p)
}
