package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/relay"
)

// Unterbefehle ohne Oberfläche: der eigene Vermittler und der Anker auf
// Servern (siehe docs/vermittler.md). Beide sind freiwillig; sshTermy
// verbindet ohne sie wie bisher direkt.

func defaultRelayDir() string {
	if os.Geteuid() == 0 {
		return "/var/lib/sshtermy-vermittler"
	}
	d, err := os.UserConfigDir()
	if err != nil {
		d = "."
	}
	return filepath.Join(d, "sshtermy", "vermittler")
}

func signalCtx() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func relayUsage() {
	fmt.Fprint(os.Stderr, i18n.T(`sshtermy vermittler: eigener Vermittler für Server hinter NAT/Firewall

  sshtermy vermittler start --url https://HOST:8022 [--listen :8022] [--name N]
                            [--cert C --key K | --hinter-proxy] [--offen] [--dir D]
  sshtermy vermittler code [--anker|--verwalten] [--dir D]
                                                 Kopplungszeile (10 min, einmal)
  sshtermy vermittler liste [--dir D]            gekoppelte Geräte und Server
  sshtermy vermittler entfernen ID [--dir D]     Gerät oder Server entfernen
  sshtermy vermittler ordner ID ORDNER [--dir D] Server einem Ordner zuordnen
`))
}

func runRelay(args []string) int {
	if len(args) == 0 {
		relayUsage()
		return 2
	}
	fs := flag.NewFlagSet("vermittler", flag.ContinueOnError)
	dir := fs.String("dir", defaultRelayDir(), i18n.T("Datenordner"))
	switch args[0] {
	case "start":
		listen := fs.String("listen", ":8022", i18n.T("Adresse und Port"))
		pub := fs.String("url", "", i18n.T("öffentliche Adresse (https://…), steht in den Kopplungszeilen"))
		nm := fs.String("name", "", i18n.T("Anzeigename in sshTermy"))
		cert := fs.String("cert", "", i18n.T("eigenes Zertifikat (PEM), sonst selbst signiert und gepinnt"))
		key := fs.String("key", "", i18n.T("Schlüssel zum Zertifikat (PEM)"))
		proxy := fs.Bool("hinter-proxy", false, i18n.T("nur http, TLS macht ein Reverse-Proxy davor"))
		openShare := fs.Bool("offen", false, i18n.T("Teilen auch ohne Kopplung erlauben (öffentlicher Vermittler, gebremst)"))
		if fs.Parse(args[1:]) != nil {
			return 2
		}
		o := relay.ServeOptions{Dir: *dir, Listen: *listen, URL: *pub, Name: *nm, Cert: *cert, Key: *key, BehindProxy: *proxy, Open: *openShare}
		st, _, err := relay.Prepare(o)
		if err != nil {
			fail(err)
		}
		fmt.Printf(i18n.T("Vermittler „%s“ auf %s, Adresse %s\n"), st.Name, *listen, st.URL)
		if st.FP != "" {
			fmt.Println(i18n.T("Zertifikat (selbst signiert, gepinnt): SHA256 ") + st.FP)
		}
		fmt.Println(i18n.T("Kopplungszeilen: sshtermy vermittler code (Gerät) · sshtermy vermittler code --anker (Server)"))
		ctx, stop := signalCtx()
		defer stop()
		if err := relay.Serve(ctx, o); err != nil {
			fail(err)
		}
	case "code":
		anchor := fs.Bool("anker", false, i18n.T("Zeile für einen Server (Anker) statt für sshTermy"))
		admin := fs.Bool("verwalten", false, i18n.T("sshTermy darf danach selbst weitere Codes holen"))
		if fs.Parse(args[1:]) != nil {
			return 2
		}
		kind := relay.CodeClient
		if *anchor {
			kind = relay.CodeAnchor
		} else if *admin {
			kind = relay.CodeAdmin
		}
		line, err := relay.Line(*dir, kind)
		if err != nil {
			fail(err)
		}
		if *anchor {
			fmt.Println(i18n.T("Auf dem Server (gilt 10 Minuten, einmal):"))
			fmt.Printf("  sshtermy anker koppeln '%s'\n", line)
		} else {
			fmt.Println(i18n.T("In sshTermy: Strg+Leertaste → „Vermittler koppeln“, diese Zeile einfügen (gilt 10 Minuten, einmal):"))
			fmt.Println("  " + line)
		}
	case "liste":
		if fs.Parse(args[1:]) != nil {
			return 2
		}
		rows, err := relay.ListPeers(*dir)
		if err != nil {
			fail(err)
		}
		tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		fmt.Fprintln(tw, i18n.T("ID\tART\tNAME\tSYSTEM\tSEIT"))
		for _, r := range rows {
			k := i18n.T("Gerät")
			if r.Kind == relay.CodeAnchor {
				k = "Server"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.ID, k, r.Name, r.OS, r.Since.Local().Format("02.01.2006"))
		}
		tw.Flush()
	case "entfernen", "ordner":
		if fs.Parse(args[1:]) != nil || fs.NArg() < 1 || (args[0] == "ordner" && fs.NArg() < 2) {
			relayUsage()
			return 2
		}
		var err error
		if args[0] == "entfernen" {
			err = relay.RemovePeer(*dir, fs.Arg(0))
		} else {
			err = relay.SetFolder(*dir, fs.Arg(0), fs.Arg(1))
		}
		if err != nil {
			fail(err)
		}
		fmt.Println(i18n.T("Erledigt."))
	default:
		relayUsage()
		return 2
	}
	return 0
}

func anchorUsage() {
	fmt.Fprint(os.Stderr, i18n.T(`sshtermy anker: diesen Server über einen Vermittler erreichbar machen

  sshtermy anker koppeln 'https://…#termy=CODE' [--name N] [--ziel 127.0.0.1:22] [--datei F]
  sshtermy anker start [--datei F]      Verbindung halten (Dienst)
  sshtermy anker dienst [--datei F]     systemd-Unit ausgeben
  sshtermy anker zustand [--datei F]    gekoppelt? mit welchem Vermittler?
`))
}

func runAnchor(args []string) int {
	if len(args) == 0 {
		anchorUsage()
		return 2
	}
	fs := flag.NewFlagSet("anker", flag.ContinueOnError)
	file := fs.String("datei", relay.DefaultAnchorPath(), i18n.T("Einstellungsdatei des Ankers"))
	switch args[0] {
	case "koppeln":
		nm := fs.String("name", "", i18n.T("Name in sshTermy (Standard: Rechnername)"))
		target := fs.String("ziel", relay.DefaultTarget, i18n.T("eigenes sshd (nur dorthin öffnet der Anker Kanäle)"))
		line := ""
		if len(args) > 1 && args[1] != "" && args[1][0] != '-' {
			line, args = args[1], append([]string{args[0]}, args[2:]...)
		}
		if fs.Parse(args[1:]) != nil {
			return 2
		}
		if line == "" && fs.NArg() > 0 {
			line = fs.Arg(0)
		}
		pl, err := relay.ParseLine(line)
		if err != nil {
			fail(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cfg, err := relay.PairAnchor(ctx, pl, *nm, *target)
		if err != nil {
			fail(err)
		}
		if err := relay.SaveAnchor(*file, cfg); err != nil {
			fail(err)
		}
		fmt.Printf(i18n.T("Gekoppelt als „%s“ (%s). Gespeichert in %s\n"), cfg.Name, cfg.ID, *file)
		fmt.Println(i18n.T("Starten: sshtermy anker start   ·   als Dienst: sshtermy anker dienst"))
	case "start":
		if fs.Parse(args[1:]) != nil {
			return 2
		}
		cfg, err := relay.LoadAnchor(*file)
		if err != nil {
			fail(fmt.Errorf(i18n.T("nicht gekoppelt (%w): erst „sshtermy anker koppeln …“"), err))
		}
		ctx, stop := signalCtx()
		defer stop()
		a := &relay.Anchor{Cfg: cfg}
		if err := a.Run(ctx); err != nil {
			fail(err)
		}
	case "dienst":
		if fs.Parse(args[1:]) != nil {
			return 2
		}
		exe, err := os.Executable()
		if err != nil {
			exe = "/usr/local/bin/sshtermy"
		}
		fmt.Print(relay.SystemdUnit(exe, *file))
		fmt.Fprintln(os.Stderr, i18n.T("# speichern als /etc/systemd/system/sshtermy-anker.service, dann: systemctl enable --now sshtermy-anker"))
	case "zustand":
		if fs.Parse(args[1:]) != nil {
			return 2
		}
		cfg, err := relay.LoadAnchor(*file)
		if err != nil {
			fmt.Println(i18n.T("nicht gekoppelt"))
			return 1
		}
		fmt.Printf(i18n.T("gekoppelt als „%s“ (%s) mit %s, Ziel %s\n"), cfg.Name, cfg.ID, cfg.Base, cfg.Target)
	default:
		anchorUsage()
		return 2
	}
	return 0
}
