package agent

import (
	"regexp"
	"strings"
)

// Befehl wartet auf eine Eingabe: Rückfrage (Y/n), Pager (less, --More--)
// oder „beliebige Taste“. Der Ziel-Modus würde sonst ewig auf den Prompt
// warten. Passwortabfragen erkennt LooksLikeSecretPrompt.

// InputKind: welche Art Eingabe erwartet wird.
type InputKind string

const (
	InputNone    InputKind = ""
	InputYesNo   InputKind = "yesno"   // [Y/n], (yes/no), Continue?
	InputPager   InputKind = "pager"   // less, more, --More-- am Switch
	InputAnyKey  InputKind = "key"     // Press any key / ENTER
	InputConfirm InputKind = "confirm" // [confirm] (Cisco): ⏎ = ja
)

var (
	ynRx      = regexp.MustCompile(`(?i)(\[\s*y(es)?\s*/\s*n(o)?\s*\]|\(\s*y(es)?\s*/\s*n(o)?\s*\)|\[\s*j\s*/\s*n\s*\]|\(\s*j(a)?\s*/\s*n(ein)?\s*\)|\byes/no\b|\by/n\b)\s*[?:]?\s*$`)
	ynQRx     = regexp.MustCompile(`(?i)\b(do you want to continue|continue|proceed|are you sure|overwrite|möchten sie fortfahren|fortfahren|wirklich)\b[^\n]{0,40}\?\s*$`)
	confirmRx = regexp.MustCompile(`(?i)\[confirm\]\s*$`)
	pagerRx   = regexp.MustCompile(`(?i)(^\s*:\s*$|\(END\)\s*$|^\s*lines \d+-\d+(/\d+)?( \d+%)?( \(END\))?\s*$|--\s*more\s*--|-- ?more ?--|--mehr--|press space|<--- more --->|^\s*more\s*$|\bmanual page\b.*\(press h for help or q to quit\))`)
	keyRx     = regexp.MustCompile(`(?i)(press any key|press (enter|return)|hit (enter|return)|beliebige taste|drücken sie (die )?(eingabe|enter)|press <?enter>? to continue)[^\n]{0,30}$`)
)

// InputWait prüft die Zeile mit dem Cursor (und bei Pagern die letzte
// Bildschirmzeile). alt = Alternativer Bildschirm (less, man).
func InputWait(cursorLine string, alt bool) InputKind {
	l := strings.TrimRight(cursorLine, " \t")
	if strings.TrimSpace(l) == "" {
		return InputNone
	}
	switch {
	case confirmRx.MatchString(l):
		return InputConfirm
	case ynRx.MatchString(l), ynQRx.MatchString(l):
		return InputYesNo
	case keyRx.MatchString(l):
		return InputAnyKey
	case pagerRx.MatchString(l):
		if strings.TrimSpace(l) == ":" && !alt {
			return InputNone // ein Doppelpunkt allein ist ohne Pager eher eine Eingabe
		}
		return InputPager
	}
	return InputNone
}

// InputKeys: was die Knöpfe senden. Reihenfolge = Anzeige.
type InputButton struct {
	Key, Label, Send string // Label ist deutsch, die Oberfläche übersetzt
}

func InputKeys(k InputKind) []InputButton {
	ctrlC := InputButton{"ctrl+c", "abbrechen", "\x03"}
	switch k {
	case InputYesNo:
		return []InputButton{{"j", "ja", "y\r"}, {"n", "nein", "n\r"}, ctrlC}
	case InputConfirm:
		return []InputButton{{"j", "ja", "\r"}, {"n", "nein", "n"}, ctrlC}
	case InputPager:
		return []InputButton{{"space", "weiter", " "}, {"q", "beenden", "q"}, ctrlC}
	case InputAnyKey:
		return []InputButton{{"enter", "weiter", "\r"}, ctrlC}
	}
	return nil
}
