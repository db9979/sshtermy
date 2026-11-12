package agent

import "strings"

// Incomplete prüft einen Befehl für eine POSIX-Shell (bash, zsh, sh): Bliebe
// die Shell danach am Fortsetzungs-Prompt „>“ stehen (offenes Anführungszeichen,
// Heredoc ohne Ende, „|“ oder „\“ am Zeilenende, offene Klammer, if ohne fi …),
// verschluckte sie den nächsten Befehl als Rest dieser Zeile. Rückgabe: der
// Grund für Mensch und Modell (leer = vollständig).
//
// Bewusst vorsichtig: Was der Prüfer nicht sicher erkennt, gilt als
// vollständig. Ein Fehlalarm hielte einen guten Befehl auf, ein übersehener
// Fall fängt sshTermy am „>“ ohnehin ab (Strg+C).
func Incomplete(cmd string) string {
	sc := shScan{src: cmd, cmdPos: true}
	return sc.run()
}

type shScan struct {
	src    string
	i      int
	stack  []byte // '"' Anführung, '`' Backtick, '(' Klammer/$(, '{' Gruppe, 'p' ${…}
	word   strings.Builder
	plain  bool // Wort ohne Anführungszeichen (nur dann ein Schlüsselwort)
	cmdPos bool // nächstes Wort steht an Befehlsstelle
	ifs    int  // if … fi
	cases  int  // case … esac
	dos    int  // do … done
	docs   []heredoc
	last   string // letzter Operator/letztes Zeichen außerhalb von Anführungen
}

type heredoc struct {
	delim string
	tabs  bool // <<- : führende Tabs vor dem Endwort erlaubt
}

func (s *shScan) top() byte {
	if len(s.stack) == 0 {
		return 0
	}
	return s.stack[len(s.stack)-1]
}

func (s *shScan) push(b byte) { s.stack = append(s.stack, b) }
func (s *shScan) pop()        { s.stack = s.stack[:len(s.stack)-1] }

func (s *shScan) at(k int) byte {
	if s.i+k < len(s.src) {
		return s.src[s.i+k]
	}
	return 0
}

// endWord schließt das laufende Wort ab und zählt Schlüsselwörter an
// Befehlsstelle.
func (s *shScan) endWord() {
	if s.word.Len() == 0 {
		return
	}
	w, plain, pos := s.word.String(), s.plain, s.cmdPos
	s.word.Reset()
	s.plain = true
	s.last = "w"
	s.cmdPos = false
	if !plain || !pos {
		return
	}
	switch w {
	case "if":
		s.ifs++
	case "fi":
		s.ifs = max(0, s.ifs-1)
	case "case":
		s.cases++
	case "esac":
		s.cases = max(0, s.cases-1)
	case "do":
		s.dos++
	case "done":
		s.dos = max(0, s.dos-1)
	}
	switch w {
	case "if", "then", "else", "elif", "do", "while", "until", "!", "time", "{":
		s.cmdPos = true // danach folgt wieder ein Befehl
	}
}

func (s *shScan) addChar(c byte, plain bool) {
	if s.word.Len() == 0 {
		s.plain = true
	}
	if !plain {
		s.plain = false
	}
	s.word.WriteByte(c)
}

func (s *shScan) run() string {
	s.plain = true
	for s.i < len(s.src) {
		c := s.src[s.i]
		if s.top() == '"' {
			if r := s.inDouble(c); r != "" {
				return r
			}
			continue
		}
		if r := s.inCode(c); r != "" {
			return r
		}
	}
	s.endWord()
	switch {
	case len(s.docs) > 0:
		return "Heredoc (<<) ohne Endzeile"
	case len(s.stack) > 0:
		switch s.top() {
		case '"':
			return `offenes Anführungszeichen "`
		case '`':
			return "offener Backtick `"
		case '(':
			return "offene Klammer ("
		case '{':
			return "offene Klammer {"
		case 'p':
			return "offenes ${"
		}
	case s.last == "|" || s.last == "||" || s.last == "&&":
		return "endet mit " + s.last
	case s.last == `\`:
		return `endet mit \`
	case s.ifs > 0:
		return "if ohne fi"
	case s.dos > 0:
		return "do ohne done"
	case s.cases > 0:
		return "case ohne esac"
	}
	return ""
}

func (s *shScan) inDouble(c byte) string {
	switch {
	case c == '\\':
		s.i += 2
	case c == '"':
		s.pop()
		s.i++
	case c == '$' && s.at(1) == '(':
		s.push('(')
		s.i += 2
	case c == '$' && s.at(1) == '{':
		s.push('p')
		s.i += 2
	case c == '`':
		s.push('`')
		s.i++
	default:
		s.i++
	}
	return ""
}

func (s *shScan) inCode(c byte) string {
	if c != ' ' && c != '\t' {
		s.last = "w"
	}
	switch c {
	case '\\':
		if s.i+1 >= len(s.src) {
			s.endWord()
			s.last = `\`
			s.i++
			return ""
		}
		if s.at(1) != '\n' {
			s.addChar(s.at(1), false)
		}
		s.i += 2
		return ""
	case '\'':
		ansi := s.word.Len() > 0 && strings.HasSuffix(s.word.String(), "$")
		j := s.i + 1
		for ; j < len(s.src); j++ {
			if ansi && s.src[j] == '\\' {
				j++
				continue
			}
			if s.src[j] == '\'' {
				break
			}
		}
		if j >= len(s.src) {
			return "offenes Anführungszeichen '"
		}
		s.addChar('\'', false)
		s.i = j + 1
		return ""
	case '"':
		s.addChar('"', false)
		s.push('"')
		s.i++
		return ""
	case '`':
		s.addChar('`', false)
		if s.top() == '`' {
			s.pop()
		} else {
			s.push('`')
		}
		s.i++
		return ""
	case '$':
		switch s.at(1) {
		case '(':
			s.addChar('$', false)
			s.push('(')
			s.i += 2
			return ""
		case '{':
			s.addChar('$', false)
			s.push('p')
			s.i += 2
			return ""
		}
		s.addChar('$', false)
		s.i++
		return ""
	case '}':
		if t := s.top(); t == 'p' || (t == '{' && s.word.Len() == 0) {
			s.endWord()
			s.pop()
			s.i++
			return ""
		}
		s.addChar(c, true)
		s.i++
		return ""
	}
	if s.top() == 'p' { // innerhalb ${…}: alles andere ist Text
		s.addChar(c, false)
		s.i++
		return ""
	}
	switch c {
	case ' ', '\t':
		s.endWord()
		s.i++
	case '\n':
		s.endWord()
		s.cmdPos = true
		s.last = "\n"
		s.i++
		if r := s.readDocs(); r != "" {
			return r
		}
	case '#':
		if s.word.Len() > 0 {
			s.addChar(c, true)
			s.i++
			break
		}
		for s.i < len(s.src) && s.src[s.i] != '\n' {
			s.i++
		}
	case '(':
		s.endWord()
		s.push('(')
		s.cmdPos = true
		s.i++
	case ')':
		s.endWord()
		if s.top() == '(' {
			s.pop()
		}
		s.cmdPos = s.cases > 0 // Muster einer case-Zeile: danach ein Befehl
		s.i++
	case '{':
		// Gruppe nur als eigenes Wort an Befehlsstelle; {a,b} und {} bleiben Text
		if s.word.Len() == 0 && s.cmdPos && (s.at(1) == ' ' || s.at(1) == '\t' || s.at(1) == '\n') {
			s.push('{')
			s.i++
			break
		}
		s.addChar(c, true)
		s.i++
	case ';', '&', '|':
		s.endWord()
		op := string(c)
		if n := s.at(1); n == c || (c == ';' && n == '&') || (c == '|' && n == '&') {
			op += string(n)
			s.i++
		}
		s.i++
		s.last = op
		s.cmdPos = true
	case '<':
		s.endWord()
		if s.at(1) == '<' && s.at(2) != '<' {
			s.i += 2
			s.heredocStart()
			s.last = "w"
			break
		}
		for s.i < len(s.src) && s.src[s.i] == '<' {
			s.i++
		}
		s.last = "<"
	default:
		s.addChar(c, true)
		s.i++
		s.last = "w"
	}
	return ""
}

// heredocStart liest das Endwort nach << bzw. <<-.
func (s *shScan) heredocStart() {
	tabs := false
	if s.at(0) == '-' {
		tabs = true
		s.i++
	}
	for s.at(0) == ' ' || s.at(0) == '\t' {
		s.i++
	}
	var d strings.Builder
	for s.i < len(s.src) {
		c := s.src[s.i]
		if c == ' ' || c == '\t' || c == '\n' || c == ';' || c == '&' || c == '|' || c == '<' || c == '>' || c == ')' {
			break
		}
		if c != '\'' && c != '"' && c != '\\' {
			d.WriteByte(c)
		}
		s.i++
	}
	if d.Len() > 0 {
		s.docs = append(s.docs, heredoc{delim: d.String(), tabs: tabs})
	}
}

// readDocs: nach dem Zeilenumbruch stehen die Inhalte offener Heredocs.
func (s *shScan) readDocs() string {
	for len(s.docs) > 0 {
		d := s.docs[0]
		found := false
		for s.i < len(s.src) {
			end := strings.IndexByte(s.src[s.i:], '\n')
			line := s.src[s.i:]
			if end >= 0 {
				line = s.src[s.i : s.i+end]
				s.i += end + 1
			} else {
				s.i = len(s.src)
			}
			if d.tabs {
				line = strings.TrimLeft(line, "\t")
			}
			if strings.TrimRight(line, "\r") == d.delim {
				found = true
				break
			}
		}
		if !found {
			return "Heredoc (<<) ohne Endzeile"
		}
		s.docs = s.docs[1:]
	}
	return ""
}
