package ui

import (
	"regexp"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/version"
)

var germanish = regexp.MustCompile(`[äöüÄÖÜß]|\b(und|oder|nicht|mit|für|von|der|die|das|ein|eine|neu|alle|nur|zum|zur|im|auf|tippen|suchen|wählen|löschen)\b`)

// sameInEnglish: Tabellentexte, die auf Englisch gleich bleiben.
var sameInEnglish = map[string]bool{
	"ArubaOS-CX": true, "ArubaOS-Switch (ProCurve)": true, "BSD": true, "Cisco IOS / IOS-XE": true, "FortiOS": true,
	"HPE Comware / H3C": true, "Huawei VRP": true, "Juniper Junos": true, "Linux/Unix-Shell": true, "MikroTik RouterOS": true,
	"Windows PowerShell": true, "Windows cmd.exe": true, "macOS": true, "user@host": true,
}

var keyWord = regexp.MustCompile(`^(Strg|Alt|Shift|Space|Tab|Esc|F\d+)$`)

func keyLabelOnly(s string) bool {
	for _, w := range regexp.MustCompile(`[A-Za-zÄÖÜäöüß]{2,}`).FindAllString(s, -1) {
		if !keyWord.MatchString(w) {
			return false
		}
	}
	return true
}

// TestTablesTranslated: Texte aus Tabellen auf Paketebene (Hilfe, Tour,
// Tastenkürzel …) werden erst beim Anzeigen übersetzt; jeder braucht einen
// Eintrag in en.go.
func TestTablesTranslated(t *testing.T) {
	var texts []string
	for _, s := range helpSections {
		texts = append(texts, s.title)
		for _, r := range s.rows {
			texts = append(texts, r[0], r[1])
		}
	}
	for _, s := range tourSteps {
		texts = append(texts, s.title, s.text)
		for _, k := range s.keys {
			texts = append(texts, k[0], k[1])
		}
	}
	for _, d := range actionDefs {
		texts = append(texts, d.label, d.area)
	}
	for _, th := range Themes {
		texts = append(texts, th.Label)
	}
	for _, g := range palGroupName {
		texts = append(texts, g)
	}
	texts = append(texts, deviceLabels...)
	texts = append(texts, dialectAuto, modeFolder, modeOff, version.Idea, version.Slogan)
	for _, d := range domain.Dialects {
		texts = append(texts, d.Name)
	}
	word := regexp.MustCompile(`[A-Za-zÄÖÜäöüß]{2,}`)
	for _, s := range texts {
		if word.MatchString(s) && !sameInEnglish[s] && !keyLabelOnly(s) && !i18n.Has(s) {
			t.Errorf("keine englische Fassung für %q", s)
		}
	}
}

func TestEnglishHelpFits(t *testing.T) {
	defer i18n.Set(i18n.DE)
	i18n.Set(i18n.EN)
	s := NewStyles(Nacht)
	for _, sec := range helpSections {
		for _, l := range s.helpSection(sec, hcNone) {
			if l := ansi.Strip(l); germanish.MatchString(l) {
				t.Errorf("deutsch in der englischen Hilfe: %q", l)
			}
		}
	}
}
