package theme

import "charm.land/lipgloss/v2"

// Weitere Themes (2026-11-09). Eigene Paletten; manche sind von offenen
// Farbschemata (MIT-Lizenz: Nord, Dracula, Gruvbox, Solarized, Rosé Pine,
// Everforest, Kanagawa, Catppuccin) angeregt, die Werte sind aber für
// sshTermy neu abgestimmt: jede Farbe hält auf dem Hintergrund 4,5:1
// (Gedämpftes 3,5:1), Akzent und KI bleiben klar unterscheidbar, Ampelfarben
// nur für Zustände. Orange und Cyan sind die Zusatzfarben für Verbindungen.

// Polarnacht: kühles Graublau mit Eisblau und Polarlicht-Tönen.
var Polar = Theme{
	Name: "polar", Label: "Polarnacht", Dark: true,
	Bg:       lipgloss.Color("#242933"),
	Surface:  lipgloss.Color("#2e3440"),
	Surface2: lipgloss.Color("#3b4252"),
	Border:   lipgloss.Color("#4c566a"),
	Text:     lipgloss.Color("#e5e9f0"),
	Muted:    lipgloss.Color("#8f9bb3"),
	Accent:   lipgloss.Color("#88c0d0"),
	AI:       lipgloss.Color("#c4a3cf"),
	OK:       lipgloss.Color("#a3be8c"),
	Warn:     lipgloss.Color("#ebcb8b"),
	Danger:   lipgloss.Color("#f08a93"),
	Orange:   lipgloss.Color("#e3a184"),
	Cyan:     lipgloss.Color("#8fd4c8"),
}

// Vampir: tiefes Violettgrau mit kräftigen Bonbonfarben.
var Vampir = Theme{
	Name: "vampir", Label: "Vampir (lila)", Dark: true,
	Bg:       lipgloss.Color("#1e1f29"),
	Surface:  lipgloss.Color("#282a36"),
	Surface2: lipgloss.Color("#383a4c"),
	Border:   lipgloss.Color("#55597a"),
	Text:     lipgloss.Color("#f8f8f2"),
	Muted:    lipgloss.Color("#8c92c0"),
	Accent:   lipgloss.Color("#bd93f9"),
	AI:       lipgloss.Color("#ff79c6"),
	OK:       lipgloss.Color("#50fa7b"),
	Warn:     lipgloss.Color("#f1fa8c"),
	Danger:   lipgloss.Color("#ff6e6e"),
	Orange:   lipgloss.Color("#ffb86c"),
	Cyan:     lipgloss.Color("#8be9fd"),
}

// Wald: gedecktes Moosgrün, warmes Pergament, ruhig für lange Nächte.
var Wald = Theme{
	Name: "wald", Label: "Wald", Dark: true,
	Bg:       lipgloss.Color("#232a27"),
	Surface:  lipgloss.Color("#2b3430"),
	Surface2: lipgloss.Color("#384540"),
	Border:   lipgloss.Color("#53645c"),
	Text:     lipgloss.Color("#e1dcc4"),
	Muted:    lipgloss.Color("#93a39a"),
	Accent:   lipgloss.Color("#7fbbb3"),
	AI:       lipgloss.Color("#d699b6"),
	OK:       lipgloss.Color("#a7c080"),
	Warn:     lipgloss.Color("#dbbc7f"),
	Danger:   lipgloss.Color("#ec8385"),
	Orange:   lipgloss.Color("#e69875"),
	Cyan:     lipgloss.Color("#83c092"),
}

// Lagerfeuer: dunkles Holz, warme Glut, Retro-Erdfarben.
var Lagerfeuer = Theme{
	Name: "lagerfeuer", Label: "Lagerfeuer", Dark: true,
	Bg:       lipgloss.Color("#1d2021"),
	Surface:  lipgloss.Color("#282828"),
	Surface2: lipgloss.Color("#3c3836"),
	Border:   lipgloss.Color("#665c54"),
	Text:     lipgloss.Color("#ebdbb2"),
	Muted:    lipgloss.Color("#a89984"),
	Accent:   lipgloss.Color("#83a598"),
	AI:       lipgloss.Color("#d3869b"),
	OK:       lipgloss.Color("#b8bb26"),
	Warn:     lipgloss.Color("#fabd2f"),
	Danger:   lipgloss.Color("#fb4934"),
	Orange:   lipgloss.Color("#fe8019"),
	Cyan:     lipgloss.Color("#8ec07c"),
}

// Tiefsee: Marineblau mit leuchtendem Türkis und Korallenrot.
var Tiefsee = Theme{
	Name: "tiefsee", Label: "Tiefsee", Dark: true,
	Bg:       lipgloss.Color("#0b1622"),
	Surface:  lipgloss.Color("#11202f"),
	Surface2: lipgloss.Color("#1a3044"),
	Border:   lipgloss.Color("#2d4b66"),
	Text:     lipgloss.Color("#d6e6f5"),
	Muted:    lipgloss.Color("#7d98b3"),
	Accent:   lipgloss.Color("#4fc3f7"),
	AI:       lipgloss.Color("#c792ea"),
	OK:       lipgloss.Color("#6fdc8c"),
	Warn:     lipgloss.Color("#ffcb6b"),
	Danger:   lipgloss.Color("#ff6b81"),
	Orange:   lipgloss.Color("#f78c6c"),
	Cyan:     lipgloss.Color("#89ddff"),
}

// Kirschblüte: Pflaumenschwarz mit Rosé, Gold und Kiefergrün.
var Kirschbluete = Theme{
	Name: "kirschbluete", Label: "Kirschblüte", Dark: true,
	Bg:       lipgloss.Color("#191724"),
	Surface:  lipgloss.Color("#1f1d2e"),
	Surface2: lipgloss.Color("#2a273f"),
	Border:   lipgloss.Color("#4a4566"),
	Text:     lipgloss.Color("#e0def4"),
	Muted:    lipgloss.Color("#908caa"),
	Accent:   lipgloss.Color("#9ccfd8"),
	AI:       lipgloss.Color("#c4a7e7"),
	OK:       lipgloss.Color("#8fd1a6"),
	Warn:     lipgloss.Color("#f6c177"),
	Danger:   lipgloss.Color("#eb6f92"),
	Orange:   lipgloss.Color("#f0a07a"),
	Cyan:     lipgloss.Color("#7fc6c9"),
}

// Graphit: neutrales Anthrazit ohne Farbstich, Farben nur als Signal.
var Graphit = Theme{
	Name: "graphit", Label: "Graphit", Dark: true,
	Bg:       lipgloss.Color("#1b1b1d"),
	Surface:  lipgloss.Color("#232326"),
	Surface2: lipgloss.Color("#323236"),
	Border:   lipgloss.Color("#4d4d53"),
	Text:     lipgloss.Color("#e6e6e6"),
	Muted:    lipgloss.Color("#929298"),
	Accent:   lipgloss.Color("#78b6ff"),
	AI:       lipgloss.Color("#c9a1ff"),
	OK:       lipgloss.Color("#7bd88f"),
	Warn:     lipgloss.Color("#fcd566"),
	Danger:   lipgloss.Color("#fc6a8d"),
	Orange:   lipgloss.Color("#fd9353"),
	Cyan:     lipgloss.Color("#5ad4e6"),
}

// Abendrot: Synthwave-Lila mit Sonnenuntergangs-Orange und Neonpink.
var Abendrot = Theme{
	Name: "abendrot", Label: "Abendrot (Synthwave)", Dark: true,
	Bg:       lipgloss.Color("#1f1630"),
	Surface:  lipgloss.Color("#2a1f40"),
	Surface2: lipgloss.Color("#3b2c58"),
	Border:   lipgloss.Color("#5f4690"),
	Text:     lipgloss.Color("#f4eeff"),
	Muted:    lipgloss.Color("#a495c8"),
	Accent:   lipgloss.Color("#ff9e5e"),
	AI:       lipgloss.Color("#ff7edb"),
	OK:       lipgloss.Color("#72f1b8"),
	Warn:     lipgloss.Color("#fede5d"),
	Danger:   lipgloss.Color("#ff5c6c"),
	Orange:   lipgloss.Color("#ffb86b"),
	Cyan:     lipgloss.Color("#36f9f6"),
}

// Tusche: Japanpapier bei Nacht, Tinte, Wellenblau und Herbstlaub.
var Tusche = Theme{
	Name: "tusche", Label: "Tusche", Dark: true,
	Bg:       lipgloss.Color("#1f1f28"),
	Surface:  lipgloss.Color("#2a2a37"),
	Surface2: lipgloss.Color("#363646"),
	Border:   lipgloss.Color("#54546d"),
	Text:     lipgloss.Color("#dcd7ba"),
	Muted:    lipgloss.Color("#9a9686"),
	Accent:   lipgloss.Color("#7fb4ca"),
	AI:       lipgloss.Color("#a990d4"),
	OK:       lipgloss.Color("#98bb6c"),
	Warn:     lipgloss.Color("#e6c384"),
	Danger:   lipgloss.Color("#ff7a85"),
	Orange:   lipgloss.Color("#ffa066"),
	Cyan:     lipgloss.Color("#7aa89f"),
}

// Solar: Petrol-Nachtblau mit den klaren Solar-Signalfarben.
var Solar = Theme{
	Name: "solar", Label: "Solar (dunkel)", Dark: true,
	Bg:       lipgloss.Color("#002b36"),
	Surface:  lipgloss.Color("#073642"),
	Surface2: lipgloss.Color("#0e4553"),
	Border:   lipgloss.Color("#2f6370"),
	Text:     lipgloss.Color("#d3dcd8"),
	Muted:    lipgloss.Color("#8aa1a6"),
	Accent:   lipgloss.Color("#4aa8ec"),
	AI:       lipgloss.Color("#a3a6f0"),
	OK:       lipgloss.Color("#a8bd2e"),
	Warn:     lipgloss.Color("#e0b52b"),
	Danger:   lipgloss.Color("#ff6f69"),
	Orange:   lipgloss.Color("#f0884f"),
	Cyan:     lipgloss.Color("#3cc2b5"),
}

// Lagune: dunkles Petrolgrün, Türkis und Korallenrot.
var Lagune = Theme{
	Name: "lagune", Label: "Lagune", Dark: true,
	Bg:       lipgloss.Color("#0f1f1f"),
	Surface:  lipgloss.Color("#142a2a"),
	Surface2: lipgloss.Color("#1d3b3a"),
	Border:   lipgloss.Color("#2f5c59"),
	Text:     lipgloss.Color("#d4efe9"),
	Muted:    lipgloss.Color("#80a8a2"),
	Accent:   lipgloss.Color("#2ed3b7"),
	AI:       lipgloss.Color("#b9a3ff"),
	OK:       lipgloss.Color("#93e38a"),
	Warn:     lipgloss.Color("#f4d35e"),
	Danger:   lipgloss.Color("#ff7a6b"),
	Orange:   lipgloss.Color("#ff9f5a"),
	Cyan:     lipgloss.Color("#6fd8f0"),
}

// ---------------------------------------------------------------- hell

// Papier: warmes Naturpapier mit Tintenblau, angenehm bei Tageslicht.
var Papier = Theme{
	Name: "papier", Label: "Papier (hell)", Dark: false,
	Bg:       lipgloss.Color("#fbf7ee"),
	Surface:  lipgloss.Color("#f0e9d8"),
	Surface2: lipgloss.Color("#e3d9c2"),
	Border:   lipgloss.Color("#b9ab8c"),
	Text:     lipgloss.Color("#2b2620"),
	Muted:    lipgloss.Color("#6e6352"),
	Accent:   lipgloss.Color("#1f5fa8"),
	AI:       lipgloss.Color("#8a3fb0"),
	OK:       lipgloss.Color("#3b7516"),
	Warn:     lipgloss.Color("#8a5800"),
	Danger:   lipgloss.Color("#b3261e"),
	Orange:   lipgloss.Color("#a84a00"),
	Cyan:     lipgloss.Color("#00707a"),
}

// Frost: kühles Hellgrau mit Polarblau, ruhig und sachlich.
var Frost = Theme{
	Name: "frost", Label: "Frost (hell)", Dark: false,
	Bg:       lipgloss.Color("#eceff4"),
	Surface:  lipgloss.Color("#e1e5ed"),
	Surface2: lipgloss.Color("#d0d6e2"),
	Border:   lipgloss.Color("#9ea8bc"),
	Text:     lipgloss.Color("#2e3440"),
	Muted:    lipgloss.Color("#5b6477"),
	Accent:   lipgloss.Color("#2f6597"),
	AI:       lipgloss.Color("#86478a"),
	OK:       lipgloss.Color("#46702a"),
	Warn:     lipgloss.Color("#835a00"),
	Danger:   lipgloss.Color("#b02a37"),
	Orange:   lipgloss.Color("#a5481b"),
	Cyan:     lipgloss.Color("#18707a"),
}

// Sand: Retro-Beige mit Erdtönen, das helle Gegenstück zu Lagerfeuer.
var Sand = Theme{
	Name: "sand", Label: "Sand (hell)", Dark: false,
	Bg:       lipgloss.Color("#f9f1dc"),
	Surface:  lipgloss.Color("#efe3c4"),
	Surface2: lipgloss.Color("#e2d3ae"),
	Border:   lipgloss.Color("#b8a47c"),
	Text:     lipgloss.Color("#3c3836"),
	Muted:    lipgloss.Color("#6f6359"),
	Accent:   lipgloss.Color("#076678"),
	AI:       lipgloss.Color("#8f3f71"),
	OK:       lipgloss.Color("#5f6a0b"),
	Warn:     lipgloss.Color("#8a5400"),
	Danger:   lipgloss.Color("#9d0006"),
	Orange:   lipgloss.Color("#a83f00"),
	Cyan:     lipgloss.Color("#3f6b3d"),
}

// Morgenrot: Rosé-Papier mit Pflaume und Petrol, das helle Gegenstück zu
// Kirschblüte.
var Morgenrot = Theme{
	Name: "morgenrot", Label: "Morgenrot (hell)", Dark: false,
	Bg:       lipgloss.Color("#faf4ed"),
	Surface:  lipgloss.Color("#f2e9e1"),
	Surface2: lipgloss.Color("#e6d9d2"),
	Border:   lipgloss.Color("#b9a7a4"),
	Text:     lipgloss.Color("#3a3552"),
	Muted:    lipgloss.Color("#6e6a86"),
	Accent:   lipgloss.Color("#286983"),
	AI:       lipgloss.Color("#7a4fa8"),
	OK:       lipgloss.Color("#3a7550"),
	Warn:     lipgloss.Color("#93560a"),
	Danger:   lipgloss.Color("#a8415f"),
	Orange:   lipgloss.Color("#a14b14"),
	Cyan:     lipgloss.Color("#2f6f73"),
}

// Minze: frisches Weißgrün mit Petrol, klar und hell.
var Minze = Theme{
	Name: "minze", Label: "Minze (hell)", Dark: false,
	Bg:       lipgloss.Color("#f2faf6"),
	Surface:  lipgloss.Color("#e2f1ea"),
	Surface2: lipgloss.Color("#cfe6db"),
	Border:   lipgloss.Color("#93b8a8"),
	Text:     lipgloss.Color("#1d2b26"),
	Muted:    lipgloss.Color("#52665e"),
	Accent:   lipgloss.Color("#00756a"),
	AI:       lipgloss.Color("#7344b5"),
	OK:       lipgloss.Color("#2f7a1f"),
	Warn:     lipgloss.Color("#866000"),
	Danger:   lipgloss.Color("#c0262d"),
	Orange:   lipgloss.Color("#ad4a00"),
	Cyan:     lipgloss.Color("#00687f"),
}

// Kreide: Schiefertafel-Grün mit Kreidefarben.
var Kreide = Theme{
	Name: "kreide", Label: "Kreide (Tafel)", Dark: true,
	Bg:       lipgloss.Color("#1e2b26"),
	Surface:  lipgloss.Color("#24342e"),
	Surface2: lipgloss.Color("#30443c"),
	Border:   lipgloss.Color("#4f6a5f"),
	Text:     lipgloss.Color("#eef2e8"),
	Muted:    lipgloss.Color("#9db1a6"),
	Accent:   lipgloss.Color("#9fd3ff"),
	AI:       lipgloss.Color("#f2b8e6"),
	OK:       lipgloss.Color("#c4ec8f"),
	Warn:     lipgloss.Color("#ffe98a"),
	Danger:   lipgloss.Color("#ff9a8a"),
	Orange:   lipgloss.Color("#ffc08a"),
	Cyan:     lipgloss.Color("#9ef0e0"),
}
