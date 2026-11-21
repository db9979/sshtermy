package cmdcheck

import (
	"reflect"
	"strings"
	"testing"
)

const manTop = `TOP(1)                     User Commands                    TOP(1)

NAME
       top - display Linux processes

SYNOPSIS
       top -hv|-bcEeHiOSs1 -d secs -n max -u|U user -p pids -o field -w [cols]

OPTIONS
       -b  :Batch-mode operation
            Starts top in Batch mode, which could be useful for sending output from top to other programs or to a file.

       -n  :Number-of-iterations limit as:  -n number
            Specifies the maximum number of iterations, or frames, top should produce before ending.

       -o  :Override-sort-field as:  -o fieldname
            Specifies the name of the field on which tasks will be sorted.
`

const manFind = `FIND(1)

NAME
       find - search for files in a directory hierarchy

EXPRESSION
       -name pattern
              Base of file name (the path with the leading directories removed) matches shell pattern pattern.

       -mtime n
              File's data was last modified less than, more than or exactly n*24 hours ago.

       -type c
              File is of type c.

       -delete
              Delete files or directories; true if removal succeeded.

       -exec command ;
              Execute command; true if 0 status is returned.
`

const helpGrep = `Usage: grep [OPTION]... PATTERNS [FILE]...
Search for PATTERNS in each FILE.

Pattern selection and interpretation:
  -E, --extended-regexp     PATTERNS are extended regular expressions
  -i, --ignore-case         ignore case distinctions in patterns and data
  -r, --recursive           like --directories=recurse
  -n, --line-number         print line number with output lines
  -e, --regexp=PATTERNS     use PATTERNS for matching
`

const manTar = `NAME
       tar - an archiving utility

OPTIONS
       -c, --create
              Create a new archive.
       -z, --gzip, --gunzip, --ungzip
              Filter the archive through gzip(1).
       -f, --file=ARCHIVE
              Use archive file or device ARCHIVE.
       -v, --verbose
              Verbosely list files processed.
       -x, --extract, --get
              Extract files from an archive.
`

func docs() Docs {
	return Docs{
		"top":  {Source: "man", Text: manTop, Path: "/usr/bin/top"},
		"find": {Source: "man", Text: manFind, Path: "/usr/bin/find"},
		"grep": {Source: "help", Text: helpGrep, Path: "/usr/bin/grep"},
		"tar":  {Source: "man", Text: manTar, Path: "/usr/bin/tar"},
		"rm":   {Source: "man", Text: "NAME\n       rm - remove files or directories\n\nOPTIONS\n       -f, --force\n              ignore nonexistent files and arguments, never prompt\n       -r, -R, --recursive\n              remove directories and their contents recursively\n       -i     prompt before every removal\n", Path: "/usr/bin/rm"},
		"lsof": {Missing: true},
		"cd":   {Builtin: true},
	}
}

func TestUsesWrappers(t *testing.T) {
	got := Programs(`sudo -u bob timeout 5 xargs -I{} find . -name '*.log' -exec rm -f {} \; | LANG=C sort | head -n 2`)
	want := []string{"sudo", "timeout", "xargs", "find", "rm", "sort", "head"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Programs = %v, want %v", got, want)
	}
	got = Programs(`echo "$(lsof -i :80)" 2>&1 > /tmp/x; sh -c 'netstat -tlpn'`)
	want = []string{"echo", "sh", "netstat", "lsof"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Programs = %v, want %v", got, want)
	}
}

func TestCheckDocs(t *testing.T) {
	cases := []struct {
		cmd  string
		want []string
	}{
		{"top -b -n 1 -l 1", []string{"top: -l steht nicht in der Doku"}},
		{"top -b -n 1 -o %MEM", nil},
		{"find . -name '*.tmp' -mtime +30 -delete", nil},
		{"find . -iname x", []string{"find: -iname steht nicht in der Doku"}},
		{"grep -rn --ignore-case foo /etc", nil},
		{"grep --color=auto foo x", []string{"grep: --color steht nicht in der Doku"}},
		{"tar -czvf a.tgz dir", nil},
		{"tar czf a.tgz dir", nil},
		{"lsof -i :80", []string{"lsof: auf diesem System nicht installiert"}},
		{"cd /tmp && rm -rf x", nil},
		{"grep -e '-foo' file", nil},
	}
	for _, c := range cases {
		var got []string
		for _, is := range CheckDocs(c.cmd, docs()) {
			got = append(got, is.Text)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %v, want %v", c.cmd, got, c.want)
		}
	}
}

func TestForbidden(t *testing.T) {
	cases := map[string][]string{
		"which process listens on port 8080, without lsof":  {"lsof"},
		"welcher Prozess lauscht auf Port 8080, ohne lsof":  {"lsof"},
		"without lsof, netstat or ss":                       {"lsof", "netstat", "ss"},
		"zeige offene Ports, netstat ist nicht installiert": {"netstat"},
		"don't use sudo please":                             {"sudo"},
		"ohne sudo oder su":                                 {"sudo", "su"},
		"liste Dateien ohne Kopfzeile":                      nil,
		"delete logs without deleting the folder":           nil,
		"statt curl bitte wget":                             {"curl"},
		"bitte kein `awk` verwenden":                        {"awk"},
		"benutze grep nicht":                                nil,
		"grep bitte nicht verwenden":                        {"grep"},
	}
	for q, want := range cases {
		if got := Forbidden(q); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: %v, want %v", q, got, want)
		}
	}
}

func texts(r Result) string { return strings.Join(r.Texts(), " | ") }

func TestStaticIssues(t *testing.T) {
	cases := []struct{ q, cmd, want string }{
		{"welcher Prozess lauscht auf Port 8080, ohne lsof", "sudo lsof -i :8080", "die Frage sagt ohne lsof, der Befehl nutzt es"},
		{"welcher Prozess lauscht auf Port 8080", "ss -ltnp 'sport = :8080'", ""},
		{"welcher Prozess lauscht auf Port 8080", "ss -ltnp", "die Frage nennt Port 8080, der Befehl nicht"},
		{"lösche .log-Dateien älter als 30 Tage in /var/log", "find /var/log -name '*.log' -mtime +30 -delete", ""},
		{"delete .log files older than 30 days", "find . -name '*.log' -mmin +43200 -delete", ""},
		{"delete .log files older than 30 days", "find . -name '*.txt' -mtime +30 -delete", "die Frage nennt .log, der Befehl nicht"},
		{"zeige Dateien größer als 1 GB", "find / -size +1G", ""},
		{"zeige Dateien größer als 1 GB", "find / -size +1024M", ""},
		{"zeige welche tmp-Dateien weg können", "find /tmp -type f -delete", "die Frage will nur ansehen, der Befehl löscht"},
		{"finde und lösche leere Ordner", "find . -type d -empty -delete", ""},
		{"fahr den Server herunter", "sudo systemctl reboot", "die Frage sagt herunterfahren, der Befehl startet neu"},
		{"", "top -l 1", "top -l gibt es nur auf macOS; unter Linux top -b -n 1"},
		{"", "echo hi | pbcopy", "pbcopy gibt es nur auf macOS"},
		{"", "echo 'offen", "Anführungszeichen oder Klammer nicht geschlossen"},
		{"kill the process PID on port 8080", "kill -TERM PID", ""},
	}
	for _, c := range cases {
		if got := texts(Static(c.q, c.cmd, Sys{})); got != c.want {
			t.Errorf("%q / %q:\n got  %q\n want %q", c.q, c.cmd, got, c.want)
		}
	}
}

func TestRepair(t *testing.T) {
	cases := []struct {
		q, cmd string
		sys    Sys
		want   string
	}{
		{"", "ps -eo pid,comm,%cpu --sort=-%cpu | head -n 1", Sys{}, "ps -eo pid,comm,%cpu --sort=-%cpu | head -n 2"},
		{"", "ps -eo pid=,comm= | head -n 1", Sys{}, "ps -eo pid=,comm= | head -n 1"},
		{"", "grep ERROR /var/log", Sys{}, "grep -r ERROR /var/log"},
		{"", "grep ERROR app.log", Sys{}, "grep ERROR app.log"},
		{"", "grep -rn ERROR /etc/", Sys{}, "grep -rn ERROR /etc/"},
		{"restart nginx without sudo", "sudo systemctl restart nginx", Sys{}, "systemctl restart nginx"},
		{"", "sed -i '' 's/a/b/' f", Sys{}, "sed -i 's/a/b/' f"},
		{"liste installierte Pakete", "rpm -qa", Sys{PkgManager: "apt"}, "dpkg -l"},
		{"liste Pakete mit rpm", "rpm -qa", Sys{PkgManager: "apt"}, "rpm -qa"},
		{"suche nach htop", "apt search htop", Sys{PkgManager: "pacman"}, "pacman -Ss htop"},
	}
	for _, c := range cases {
		got, _ := Repair(c.q, c.cmd, c.sys)
		if got != c.want {
			t.Errorf("%q: got %q, want %q", c.cmd, got, c.want)
		}
	}
	if got := texts(Static("installiere htop", "yum install htop", Sys{PkgManager: "apt"})); got != "yum gibt es hier nicht, dieses System nutzt apt" {
		t.Errorf("foreign manager: %q", got)
	}
}

func TestExplain(t *testing.T) {
	parts := Explain("find . -name '*.tmp' -exec rm -rf {} \\;", docs())
	var got []string
	for _, p := range parts {
		got = append(got, p.Name+"="+p.Text)
	}
	want := []string{
		"find=search for files in a directory hierarchy",
		"-name=Base of file name (the path with the leading directories removed) matches shell pattern pattern",
		"-exec=Execute command; true if 0 status is returned",
		"rm=remove files or directories",
		"-r=remove directories and their contents recursively",
		"-f=ignore nonexistent files and arguments, never prompt",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Explain:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	parts = Explain("grep -in foo x", docs())
	got = nil
	for _, p := range parts {
		got = append(got, p.Name+"="+p.Text)
	}
	want = []string{"grep=Search for PATTERNS in each FILE", "-i=ignore case distinctions in patterns and data", "-n=print line number with output lines"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Explain help:\n%s", strings.Join(got, "\n"))
	}
}

func TestProbeRoundTrip(t *testing.T) {
	s := ProbeScript([]string{"top", "bad;name", "lsof"})
	if strings.Contains(s, "bad;name") || !strings.Contains(s, " top lsof;") {
		t.Fatalf("script: %s", s)
	}
	out := "junk\n" + marker + "top\n@@path /usr/bin/top\n@@man\nNAME\n       top - display Linux processes\n" +
		"\n" + marker + "lsof\n@@missing\n" + "\n" + marker + "cd\n@@builtin\n"
	d := ParseProbe(out)
	if d["top"] == nil || d["top"].Source != "man" || !strings.Contains(d["top"].Text, "display Linux") || d["top"].Path != "/usr/bin/top" {
		t.Fatalf("top: %+v", d["top"])
	}
	if !d["lsof"].Missing || !d["cd"].Builtin {
		t.Fatalf("lsof/cd: %+v %+v", d["lsof"], d["cd"])
	}
}

func TestForbiddenGermanVerbLast(t *testing.T) {
	cases := map[string][]string{
		"nginx ohne sudo neu starten":          {"sudo"},
		"welcher Port, ohne lsof herausfinden": {"lsof"},
		"Dateien ohne Kopfzeile zählen":        nil,
		"ohne Rückfrage löschen":               nil,
		"liste Prozesse, kein ps verwenden":    {"ps"},
		"zeige Ports ohne netstat oder ss":     {"netstat", "ss"},
	}
	for q, want := range cases {
		if got := Forbidden(q); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: %v, want %v", q, got, want)
		}
	}
}
