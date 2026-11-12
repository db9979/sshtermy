package i18n

// Texte zum Fortsetzungs-Prompt „>“ (contprompt.go, agent/incomplete.go).
func init() {
	for de, e := range map[string]string{
		"Shell wartete am „>“ auf eine Fortsetzung: mit Strg+C abgebrochen, dann den Befehl gesendet": "Shell was waiting at “>” for more input: cancelled with Ctrl+C, then sent the command",
		"Befehl unvollständig": "command incomplete",
		"Befehl blieb am „>“ hängen (%s): mit Strg+C abgebrochen":                                       "Command got stuck at “>” (%s): cancelled with Ctrl+C",
		"unvollständig, Shell wartete am „>“ auf Fortsetzung (%s), sshTermy hat mit Strg+C abgebrochen": "incomplete, the shell was waiting at “>” for more input (%s), sshTermy cancelled with Ctrl+C",
		"Befehl unvollständig (%s): nur eingefügt, bitte ergänzen":                                      "Command incomplete (%s): inserted only, please complete it",
		"Heredoc (<<) ohne Endzeile":  "heredoc (<<) without end line",
		`offenes Anführungszeichen "`: `unclosed quote "`,
		"offenes Anführungszeichen '": "unclosed quote '",
		"offener Backtick `":          "unclosed backtick `",
		"offene Klammer (":            "unclosed (",
		"offene Klammer {":            "unclosed {",
		"offenes ${":                  "unclosed ${",
		"endet mit |":                 "ends with |",
		"endet mit ||":                "ends with ||",
		"endet mit &&":                "ends with &&",
		`endet mit \`:                 `ends with \`,
		"if ohne fi":                  "if without fi",
		"do ohne done":                "do without done",
		"case ohne esac":              "case without esac",
	} {
		en[de] = e
	}
}
