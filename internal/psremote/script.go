package psremote

import (
	"encoding/base64"
	"strings"
	"unicode/utf16"
)

// Die Gegenstelle ist keine Konsole: WinRS reicht nur stdin/stdout/stderr
// durch. Damit sich das trotzdem wie Enter-PSSession anfühlt, läuft drüben
// eine kleine Schleife (bootstrap), die Zeilen aus stdin liest:
//
//	\x1eC<base64>   Befehl ausführen (unvollständig → Marke M, mehr Zeilen)
//	\x1eX<base64>   Befehl ausführen, auch wenn er unvollständig ist
//	\x1eT<n>:<b64>  Tab-Ergänzung (TabExpansion2) für Cursor-Spalte n
//	\x1eW<n>        Ausgabebreite in Zeichen
//
// Zeilen ohne \x1e sind Eingaben, während ein Befehl lief: Read-Host & Co.
// lesen sie selbst; bleibt eine liegen, führt die Schleife sie danach aus
// (wie Vorweg-Tippen in einer Konsole) und meldet das mit Marke R.
// Zurück kommen Marken \x01<Art><Text>\x02 mitten in der Ausgabe:
// P = Prompt, D = Ordner, I = System, T = Ergänzungen, M = weiter, R = s. o.

// stub: das, was als -EncodedCommand mitgeht. Liest die eigentliche
// Schleife als erste Zeile von stdin (die Kommandozeile hat bei WinRS/cmd
// nur gut 8000 Zeichen Platz).
const stub = `$__b=[Console]::In.ReadLine();. ([ScriptBlock]::Create([Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($__b))))`

const bootstrap = `$ErrorActionPreference='Continue'
$ProgressPreference='SilentlyContinue'
try{[Console]::OutputEncoding=New-Object Text.UTF8Encoding $false}catch{}
try{$OutputEncoding=New-Object Text.UTF8Encoding $false}catch{}
$__o=[Console]::Out
$__w=120
$__esc=[string][char]27
function __m([string]$k,[string]$v){$__o.Write([string][char]1+$k+$v+[string][char]2);$__o.Flush()}
function __p{
 $p=''
 try{$p=[string](prompt)}catch{}
 if(-not $p){$p='PS '+$PWD.Path+'> '}
 __m 'D' $PWD.Path
 __m 'P' $p
}
function __c($x){
 if($x -is [Management.Automation.ErrorRecord]){return $__esc+'[91m'+($x|Out-String -Width $__w).TrimEnd()+$__esc+'[0m'}
 if($x -is [Management.Automation.WarningRecord]){return $__esc+'[93mWARNING: '+$x.Message+$__esc+'[0m'}
 if($x -is [Management.Automation.VerboseRecord]){return $__esc+'[96mVERBOSE: '+$x.Message+$__esc+'[0m'}
 if($x -is [Management.Automation.DebugRecord]){return $__esc+'[96mDEBUG: '+$x.Message+$__esc+'[0m'}
 return $x
}
if($__d){try{Set-Location -LiteralPath $__d -ErrorAction Stop}catch{}}
$__os=''
try{$__os=(Get-CimInstance Win32_OperatingSystem -ErrorAction Stop).Caption}catch{try{$__os=(Get-WmiObject Win32_OperatingSystem -ErrorAction Stop).Caption}catch{}}
if(-not $__os){$__os=[Environment]::OSVersion.VersionString}
__m 'I' ([string]$__os+'|'+[string]$PSVersionTable.PSVersion+'|'+[Environment]::MachineName)
__p
while($true){
 $__l=[Console]::In.ReadLine()
 if($null -eq $__l){break}
 if($__l.Length -eq 0){continue}
 if($__l[0] -ne [char]30){__m 'R' $__l;$__l=[string][char]30+'X'+[Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($__l))}
 if($__l.Length -lt 2){continue}
 $__k=[string]$__l[1];$__a=$__l.Substring(2)
 if($__k -eq 'W'){try{$__w=[int]$__a}catch{};continue}
 if($__k -eq 'T'){
  $__r=''
  try{
   $__i=$__a.IndexOf(':')
   $__s=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($__a.Substring($__i+1)))
   $__x=TabExpansion2 -inputScript $__s -cursorColumn ([int]$__a.Substring(0,$__i))
   $__r=[string]$__x.ReplacementIndex+','+[string]$__x.ReplacementLength
   foreach($__y in @($__x.CompletionMatches|Select-Object -First 300)){$__r+=[string][char]31+$__y.CompletionText}
  }catch{}
  __m 'T' $__r
  continue
 }
 if($__k -ne 'C' -and $__k -ne 'X'){continue}
 $__s=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($__a))
 if($__k -eq 'C'){
  $__t=$null;$__pe=$null
  [void][Management.Automation.Language.Parser]::ParseInput($__s,[ref]$__t,[ref]$__pe)
  if(@($__pe|Where-Object{$_.IncompleteInput -or ($_.ErrorId -match '^(MissingEnd|MissingClosing|TerminatorExpected|MissingEndCurlyBrace)' -and $_.Extent.EndOffset -ge $__s.TrimEnd().Length)}).Count -gt 0){__m 'M' '';continue}
 }
 try{. ([ScriptBlock]::Create($__s)) *>&1 | ForEach-Object{__c $_} | Out-String -Stream -Width $__w | ForEach-Object{$__o.WriteLine($_)}}catch{$__o.WriteLine((__c $_))}
 $__o.Flush()
 __p
}
`

// psQuote: Text als PowerShell-Zeichenkette in einfachen Anführungszeichen.
func psQuote(s string) string {
	r := strings.NewReplacer("'", "''", "‘", "‘‘", "’", "’’", "‚", "‚‚", "‛", "‛‛")
	return "'" + r.Replace(s) + "'"
}

// bootLine: erste Zeile für stdin (Schleife samt Startordner).
func bootLine(dir string) []byte {
	src := "$__d=" + psQuote(dir) + "\n" + bootstrap
	return []byte(base64.StdEncoding.EncodeToString([]byte(src)) + "\n")
}

// encodedCommand: PowerShell erwartet bei -EncodedCommand UTF-16LE in Base64.
func encodedCommand(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, 2*len(u))
	for i, c := range u {
		b[2*i], b[2*i+1] = byte(c), byte(c>>8)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// psArgs: Aufruf von powershell.exe/pwsh mit der Schleife.
func psArgs() []string {
	return []string{"-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encodedCommand(stub)}
}

func cmdLine(kind byte, payload string) []byte {
	return []byte("\x1e" + string(kind) + payload + "\n")
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
