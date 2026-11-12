package store

import (
	"fmt"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
)

func TestGoalRuns(t *testing.T) {
	t.Setenv("SSHTERMY_HOME", t.TempDir())
	st, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Init([]byte("correct horse"), false); err != nil {
		t.Fatal(err)
	}
	base := time.Now()
	for i := 0; i < goalRunsPer+5; i++ {
		if err := st.AddGoalRun(domain.GoalRun{ConnID: "a", Goal: fmt.Sprint("ziel ", i), State: "done", At: base.Add(time.Duration(i) * time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.AddGoalRun(domain.GoalRun{ConnID: "b", Goal: "anderes", State: "stopped", At: base}); err != nil {
		t.Fatal(err)
	}
	runs := st.GoalRuns("a")
	if len(runs) != goalRunsPer || runs[0].Goal != fmt.Sprint("ziel ", goalRunsPer+4) || runs[len(runs)-1].Goal != "ziel 5" {
		t.Fatalf("Verlauf a: %d, erster %q, letzter %q", len(runs), runs[0].Goal, runs[len(runs)-1].Goal)
	}
	ids := map[string]bool{}
	for _, r := range runs {
		if ids[r.ID] {
			t.Fatalf("doppelte ID %s", r.ID)
		}
		ids[r.ID] = true
	}
	if err := st.DeleteGoalRun(runs[0].ID); err != nil {
		t.Fatal(err)
	}
	if len(st.GoalRuns("a")) != goalRunsPer-1 || len(st.GoalRuns("b")) != 1 {
		t.Error("Löschen trifft den falschen Eintrag")
	}
	// bleibt nach dem Neuladen (lokal gespeichert)
	st2, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := st2.V.UnlockPassword([]byte("correct horse")); err != nil {
		t.Fatal(err)
	}
	if err := st2.AfterUnlock(); err != nil {
		t.Fatal(err)
	}
	if len(st2.GoalRuns("b")) != 1 {
		t.Error("Zielverlauf nicht gespeichert")
	}
}
