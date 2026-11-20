package agent

import "testing"

// Antworten der KI sind fremder Text: halbes JSON, Markdown drumherum,
// falsche Typen. Lesen darf scheitern, aber nie abstürzen.
func FuzzAnswers(f *testing.F) {
	f.Add(`{"cmd":"uptime","why":"Last","done":false}`)
	f.Add("```json\n{\"plan\":[{\"cmd\":\"ls\"}],\"files\":[{\"path\":\"/tmp/x\",\"content\":\"a\"}]}\n```")
	f.Add(`{"windows":[{"n":1,"cmd":"df -h"}],"options":["a",2,null]}`)
	f.Add(`{"pick":3}`)
	f.Add(`[`)
	f.Fuzz(func(t *testing.T, s string) {
		for _, kind := range []string{"unix", "windows", "network"} {
			_, _ = parseStep(s, kind)
			_, _ = parsePlan(s, kind, true)
			_, _ = parseAuto(s, kind, true)
			_, _ = parseReview(s, 3, kind)
			_ = ParseStepCommand(s, kind)
		}
		_, _ = ParseMulti(s, []Window{{N: 1, Name: "web"}, {N: 2, Name: "db", Prod: true}})
		_ = ParseMenuPick(s, 5)
	})
}
