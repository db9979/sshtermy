package llm

import (
	"sort"
	"sync"
	"time"
)

// Antwortzeiten je Aufgabe (Ghost-Text, Agent, …) für den KI-Status: so sieht
// man, welche Funktion bremst. Gezählt werden nur erfolgreiche Anfragen;
// bei Streams die Zeit bis zum ersten Zeichen.

// TaskStat: gleitender Mittelwert der Antwortzeit einer Aufgabe.
type TaskStat struct {
	Task  string
	Model string // Modell der letzten Antwort
	N     int
	Avg   time.Duration // gleitend (die letzten Anfragen zählen mehr)
	Last  time.Duration
	At    time.Time
}

var tasks struct {
	mu sync.Mutex
	m  map[string]*TaskStat
}

func recordTask(task, model string, d time.Duration) {
	if task == "" {
		return
	}
	tasks.mu.Lock()
	defer tasks.mu.Unlock()
	if tasks.m == nil {
		tasks.m = map[string]*TaskStat{}
	}
	t := tasks.m[task]
	if t == nil {
		t = &TaskStat{Task: task}
		tasks.m[task] = t
	}
	t.N++
	if t.N == 1 {
		t.Avg = d
	} else {
		t.Avg = (t.Avg*4 + d) / 5
	}
	t.Last, t.Model, t.At = d, model, time.Now()
}

// TaskStats: alle Aufgaben, die seit dem Start gelaufen sind, langsamste zuerst.
func TaskStats() []TaskStat {
	tasks.mu.Lock()
	defer tasks.mu.Unlock()
	out := make([]TaskStat, 0, len(tasks.m))
	for _, t := range tasks.m {
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Avg != out[j].Avg {
			return out[i].Avg > out[j].Avg
		}
		return out[i].Task < out[j].Task
	})
	return out
}

// ResetTaskStats leert die Antwortzeiten (für Tests und Bildschirmfotos).
func ResetTaskStats() {
	tasks.mu.Lock()
	tasks.m = nil
	tasks.mu.Unlock()
}
