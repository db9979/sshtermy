// Package containers listet Docker-, Podman- und Kubernetes-Container auf
// einem Server und baut die Befehle, mit denen man in sie hineinkommt.
//
// Abgefragt wird mit einem kleinen, nur lesenden sh-Skript über eine eigene
// Exec-Sitzung. Braucht docker root und darf der Benutzer sudo ohne
// Passwort, versucht es das Skript mit sudo -n; sonst kommt ein Hinweis.
package containers

import (
	"regexp"
	"strconv"
	"strings"
)

// Script: listet Container als „laufzeit|id|name|image|zustand|status“,
// Pods als „k8s|namespace|name||phase|bereit neustarts“, Fehler als
// „@err laufzeit|meldung“ und sudo als „@sudo laufzeit“.
const Script = `
for rt in docker podman; do
  command -v $rt >/dev/null 2>&1 || continue
  f='{{.ID}}|{{.Names}}|{{.Image}}|{{.State}}|{{.Status}}'
  out=$($rt ps -a --format "$f" 2>&1); rc=$?
  if [ $rc -ne 0 ] && command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then
    out2=$(sudo -n $rt ps -a --format "$f" 2>&1) && { out=$out2; rc=0; echo "@sudo $rt"; }
  fi
  if [ $rc -eq 0 ]; then
    printf '%s\n' "$out" | awk -v rt=$rt 'NF { print rt "|" $0 }'
  else
    echo "@err $rt|$(printf '%s' "$out" | head -n 1)"
  fi
done
if command -v kubectl >/dev/null 2>&1; then
  out=$(kubectl get pods -A --no-headers --request-timeout=8s -o 'custom-columns=NS:.metadata.namespace,NAME:.metadata.name,PHASE:.status.phase,READY:.status.containerStatuses[*].ready,RESTARTS:.status.containerStatuses[*].restartCount' 2>&1); rc=$?
  if [ $rc -eq 0 ]; then
    printf '%s\n' "$out" | awk 'NF { print "k8s|" $1 "|" $2 "||" $3 "|" $4 " " $5 }'
  else
    echo "@err kubectl|$(printf '%s' "$out" | head -n 1)"
  fi
fi
echo "@done"
`

// Container: ein Container oder Pod.
type Container struct {
	Runtime   string // docker | podman | k8s
	ID        string
	Name      string
	Image     string
	State     string // running, exited … (k8s: Running, Pending …)
	Status    string // „Up 3 hours“ (k8s: bereit + Neustarts)
	Namespace string // nur k8s
	Sudo      bool
}

// Running: läuft der Container?
func (c Container) Running() bool {
	s := strings.ToLower(c.State)
	return s == "running" || s == "up"
}

// Result: alles, was das Skript gemeldet hat.
type Result struct {
	List     []Container
	Errors   []string // „docker: permission denied …“
	Runtimes []string // gefundene Laufzeiten
	Done     bool     // Skript lief bis zum Ende
}

// Parse liest die Ausgabe von Script.
func Parse(out string) Result {
	var r Result
	sudo := map[string]bool{}
	seen := map[string]bool{}
	note := func(rt string) {
		if !seen[rt] {
			seen[rt] = true
			r.Runtimes = append(r.Runtimes, rt)
		}
	}
	for _, l := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		l = strings.TrimSpace(l)
		switch {
		case l == "":
			continue
		case l == "@done":
			r.Done = true
			continue
		case strings.HasPrefix(l, "@sudo "):
			sudo[strings.TrimSpace(l[6:])] = true
			continue
		case strings.HasPrefix(l, "@err "):
			rt, msg, _ := strings.Cut(l[5:], "|")
			note(rt)
			r.Errors = append(r.Errors, rt+": "+strings.TrimSpace(msg))
			continue
		}
		f := strings.Split(l, "|")
		if len(f) < 6 {
			continue
		}
		c := Container{Runtime: f[0]}
		if c.Runtime == "k8s" {
			c.Namespace, c.Name, c.State, c.Status = f[1], f[2], f[4], strings.TrimSpace(f[5])
			c.Status = k8sStatus(c.Status)
		} else if c.Runtime == "docker" || c.Runtime == "podman" {
			c.ID, c.Name, c.Image, c.State, c.Status = f[1], f[2], f[3], f[4], strings.Join(f[5:], "|")
			c.Sudo = sudo[c.Runtime]
		} else {
			continue
		}
		note(c.Runtime)
		r.List = append(r.List, c)
	}
	return r
}

// k8sStatus: „true,true 0,2“ → „2/2 bereit · 2 Neustarts“ (ohne i18n, die
// Zahlen sprechen für sich).
func k8sStatus(s string) string {
	ready, restarts, _ := strings.Cut(s, " ")
	n, ok := 0, 0
	for _, x := range strings.Split(ready, ",") {
		if x == "" || x == "<none>" {
			continue
		}
		n++
		if x == "true" {
			ok++
		}
	}
	sum := 0
	for _, x := range strings.Split(restarts, ",") {
		v, _ := strconv.Atoi(x)
		sum += v
	}
	return itoa(ok) + "/" + itoa(n) + " ↻" + itoa(sum)
}

func itoa(n int) string { return strconv.Itoa(n) }

var safeName = regexp.MustCompile(`^[A-Za-z0-9_.:/-]+$`)

func q(s string) string {
	if safeName.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (c Container) bin() string {
	b := c.Runtime
	if c.Runtime == "k8s" {
		return "kubectl"
	}
	if c.Sudo {
		b = "sudo " + b
	}
	return b
}

func (c Container) ns() string { return "-n " + q(c.Namespace) + " " }

// ExecCmd: Shell im Container (bash, sonst sh).
func (c Container) ExecCmd() string {
	if c.Runtime == "k8s" {
		return "kubectl exec -it " + c.ns() + q(c.Name) + " -- sh -c 'command -v bash >/dev/null && exec bash || exec sh'"
	}
	return c.bin() + " exec -it " + q(c.Name) + " sh -c 'command -v bash >/dev/null && exec bash || exec sh'"
}

// LogsCmd: Logs live verfolgen (Strg+C beendet).
func (c Container) LogsCmd() string {
	if c.Runtime == "k8s" {
		return "kubectl logs -f --tail 200 " + c.ns() + q(c.Name)
	}
	return c.bin() + " logs -f --tail 200 " + q(c.Name)
}

// FetchLogs: letzte Zeilen für die KI (über Exec, nicht im Terminal).
func (c Container) FetchLogs(lines int) string {
	n := itoa(lines)
	if c.Runtime == "k8s" {
		return "kubectl logs --tail " + n + " " + c.ns() + q(c.Name) + " 2>&1"
	}
	return c.bin() + " logs --tail " + n + " " + q(c.Name) + " 2>&1"
}

// RestartCmd: Neustart (bei k8s: Pod löschen, der Controller legt ihn neu an).
func (c Container) RestartCmd() string {
	if c.Runtime == "k8s" {
		return "kubectl delete pod " + c.ns() + q(c.Name)
	}
	return c.bin() + " restart " + q(c.Name)
}

// StartStopCmd: läuft er, wird er gestoppt, sonst gestartet ("" bei k8s).
func (c Container) StartStopCmd() string {
	if c.Runtime == "k8s" {
		return ""
	}
	if c.Running() {
		return c.bin() + " stop " + q(c.Name)
	}
	return c.bin() + " start " + q(c.Name)
}
