package containers

import (
	"os/exec"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	out := `@sudo docker
docker|a1b2c3|web|nginx:1.27|running|Up 3 hours
docker|d4e5f6|db|postgres:16|exited|Exited (1) 2 minutes ago
@err podman|cannot connect to Podman socket
k8s|shop|api-7f9c|  |Running|true,false 0,3
@done
`
	r := Parse(out)
	if !r.Done || len(r.List) != 3 || len(r.Errors) != 1 {
		t.Fatalf("%+v", r)
	}
	web := r.List[0]
	if web.Name != "web" || !web.Running() || !web.Sudo || web.Image != "nginx:1.27" {
		t.Errorf("%+v", web)
	}
	if r.List[1].Running() {
		t.Error("db läuft nicht")
	}
	pod := r.List[2]
	if pod.Namespace != "shop" || pod.Name != "api-7f9c" || pod.Status != "1/2 ↻3" {
		t.Errorf("%+v", pod)
	}
	if strings.Join(r.Runtimes, ",") != "docker,podman,k8s" {
		t.Errorf("%v", r.Runtimes)
	}
}

func TestCommands(t *testing.T) {
	d := Container{Runtime: "docker", Name: "web", State: "running", Sudo: true}
	if d.LogsCmd() != "sudo docker logs -f --tail 200 web" {
		t.Error(d.LogsCmd())
	}
	if d.StartStopCmd() != "sudo docker stop web" || !strings.HasPrefix(d.ExecCmd(), "sudo docker exec -it web sh -c") {
		t.Error(d.StartStopCmd(), d.ExecCmd())
	}
	k := Container{Runtime: "k8s", Namespace: "shop", Name: "api-7f9c"}
	if k.RestartCmd() != "kubectl delete pod -n shop api-7f9c" || k.StartStopCmd() != "" {
		t.Error(k.RestartCmd())
	}
	if k.FetchLogs(300) != "kubectl logs --tail 300 -n shop api-7f9c 2>&1" {
		t.Error(k.FetchLogs(300))
	}
	odd := Container{Runtime: "podman", Name: "it's"}
	if !strings.Contains(odd.LogsCmd(), `'it'\''s'`) {
		t.Error(odd.LogsCmd())
	}
}

// Das Skript muss in einer schlichten sh laufen, auch ohne docker/kubectl.
func TestScriptRuns(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("kein sh")
	}
	out, err := exec.Command(sh, "-c", Script).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if !Parse(string(out)).Done {
		t.Fatalf("kein @done: %s", out)
	}
}
