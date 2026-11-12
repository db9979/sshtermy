// sshTermy · Zusehen im Browser. Die Anmeldung (Code, PAKE, Prüfwörter,
// Entschlüsseln) macht zusehen.wasm – derselbe Go-Code wie in sshTermy. Hier
// nur die Oberfläche: Formular, Warten, Terminal (xterm.js).
"use strict";
(() => {
  const DE = (navigator.language || "de").toLowerCase().startsWith("de");
  const EN = {
    "nur zusehen": "view only",
    "Hand heben": "Raise hand",
    "Trennen": "Leave",
    "Live zusehen": "Watch live",
    "Jemand teilt ein Terminal mit dir. Du brauchst dafür kein sshTermy.": "Someone is sharing a terminal with you. You don't need sshTermy for this.",
    "Code": "Code",
    "Dein Name (sieht der Gastgeber)": "Your name (shown to the host)",
    "Anklopfen": "Knock",
    "Der Code bleibt in deinem Browser. Daraus entsteht ein Schlüssel, den nur du und der Gastgeber kennen; alles ist Ende-zu-Ende verschlüsselt.": "The code stays in your browser. It turns into a key that only you and the host know; everything is end-to-end encrypted.",
    "Prüfe den Code …": "Checking the code …",
    "✓ Code stimmt.": "✓ Code is correct.",
    "Prüfwörter – vergleiche sie mit dem Gastgeber:": "Check words – compare them with the host:",
    "Stimmen sie nicht, sitzt jemand dazwischen: Seite schließen.": "If they don't match, someone is in between: close this page.",
    "Warte, bis der Gastgeber dich zulässt …": "Waiting for the host to let you in …",
    "Neu anklopfen": "Knock again",
    "Lade …": "Loading …",
    "Bitte einen Namen eingeben": "Please enter a name",
    "Zusehen beendet": "Watching ended",
    "Der Gastgeber hat das Teilen beendet.": "The host stopped sharing.",
    "Nicht geklappt": "Did not work",
    "%s bei %s": "%s at %s",
    "noch %d min": "%d min left",
    "darf mitschreiben bis %s": "may type until %s",
    "%s gibt gerade ein Passwort ein – ausgeblendet": "%s is typing a password – hidden",
    "Der Gastgeber": "The host",
    "Hand gehoben – der Gastgeber sieht die Bitte ums Mitschreiben": "Hand raised – the host sees your request to type",
    "Mitschreiben beendet – nur zusehen": "Typing ended – view only",
    "Du darfst mitschreiben bis %s – Tasten gehen jetzt an den Server": "You may type until %s – keys now go to the server",
    "Dieser Browser kann kein WebAssembly.": "This browser cannot run WebAssembly.",
    "Seite nur über https": "Page only over https",
  };
  const T = (s) => (DE ? s : EN[s] || s);
  const F = (s, ...a) => { let i = 0; return T(s).replace(/%[sd]/g, () => String(a[i++])); };
  const $ = (id) => document.getElementById(id);
  document.documentElement.lang = DE ? "de" : "en";
  document.querySelectorAll("[data-t]").forEach((el) => { el.textContent = T(el.dataset.t); });

  const show = (id) => { for (const s of ["start", "wait", "end", "screen"]) $(s).hidden = s !== id; };

  // Code aus dem Link (#7-taube-kompass-ofen) übernehmen und aus der Adresszeile entfernen.
  let code = decodeURIComponent((location.hash || "").replace(/^#/, "")).trim();
  if (code) history.replaceState(null, "", location.pathname + location.search);
  $("code").value = code;
  $("name").value = localStorage.getItem("sshtermy-name") || "";

  function rechner() {
    const ua = navigator.userAgent;
    const b = /Edg\//.test(ua) ? "Edge" : /Firefox\//.test(ua) ? "Firefox" : /Chrome\//.test(ua) ? "Chrome" : /Safari\//.test(ua) ? "Safari" : "Browser";
    const os = /Android/.test(ua) ? "Android" : /iPhone|iPad/.test(ua) ? "iOS" : /Windows/.test(ua) ? "Windows" : /Mac OS X/.test(ua) ? "macOS" : /Linux/.test(ua) ? "Linux" : "";
    return os ? b + " · " + os : b;
  }

  // ---------------------------------------------------------------- Terminal
  let term = null, cols = 80, rows = 24, fontSize = 0, info = null, writeUntil = 0, wantFont = Number(localStorage.getItem("sshtermy-font")) || 0;
  function fit() {
    if (!term) return;
    const box = $("screen");
    const w = box.clientWidth - 16, h = box.clientHeight - 16;
    // Zeichenbreite ≈ 0,6 × Schriftgröße, Zeilenhöhe ≈ 1,2 ×
    let fs = Math.floor(Math.min(w / (cols * 0.6), h / (rows * 1.2)));
    fs = Math.max(7, Math.min(20, fs));
    if (wantFont) fs = wantFont;
    if (fs !== fontSize) { fontSize = fs; term.options.fontSize = fs; }
  }
  function ensureTerm() {
    if (term) return;
    term = new Terminal({
      cols, rows, fontSize: 14, scrollback: 2000, cursorBlink: false, disableStdin: true, convertEol: false,
      fontFamily: 'ui-monospace, "Cascadia Mono", Menlo, Consolas, "DejaVu Sans Mono", monospace',
      theme: { background: "#0f1117", foreground: "#d8dee9" },
    });
    term.open($("term"));
    term.onData((d) => {
      if (Date.now() / 1000 < writeUntil) sshtermy.keys(new TextEncoder().encode(d));
    });
    fit();
  }
  addEventListener("resize", fit);
  $("smaller").onclick = () => { wantFont = Math.max(6, (fontSize || 14) - 1); localStorage.setItem("sshtermy-font", wantFont); fit(); };
  $("bigger").onclick = () => { wantFont = Math.min(32, (fontSize || 14) + 1); localStorage.setItem("sshtermy-font", wantFont); fit(); };

  function status() {
    const parts = [];
    if (info && info.tab) parts.push(F("%s bei %s", info.tab, info.von || "?"));
    if (info && info.bis) parts.push(F("noch %d min", Math.max(0, Math.floor((info.bis * 1000 - Date.now()) / 60000))));
    $("status").textContent = parts.join(" · ");
    const b = $("badge");
    if (Date.now() / 1000 < writeUntil) {
      b.textContent = "» " + F("darf mitschreiben bis %s", new Date(writeUntil * 1000).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }));
      b.className = "badge write";
    } else {
      b.textContent = "◉ " + T("nur zusehen");
      b.className = term ? "badge live" : "badge";
    }
    if (term) term.options.disableStdin = !(Date.now() / 1000 < writeUntil);
  }
  setInterval(status, 15000);

  let toastTimer = 0;
  function toast(msg) {
    $("paused").textContent = msg;
    $("paused").hidden = false;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => { if (!pausedNow) $("paused").hidden = true; }, 4000);
  }
  let pausedNow = false;

  // ---------------------------------------------------------------- Anmelden
  const handlers = {
    check(words) {
      $("waitmsg").hidden = true;
      $("checkbox").hidden = false;
      $("check").textContent = words;
    },
    admitted(js) {
      info = JSON.parse(js);
      show("screen");
      ensureTerm();
      for (const id of ["hand", "leave", "smaller", "bigger"]) $(id).hidden = false;
      status();
      term.focus();
    },
    data(b) { ensureTerm(); term.write(b); },
    size(c, r) { cols = c; rows = r; ensureTerm(); term.resize(c, r); fontSize = 0; fit(); },
    pause() {
      pausedNow = true;
      $("paused").textContent = "··· " + F("%s gibt gerade ein Passwort ein – ausgeblendet", (info && info.von) || T("Der Gastgeber")) + " ···";
      $("paused").hidden = false;
    },
    resume() { pausedNow = false; $("paused").hidden = true; },
    info(js) { info = JSON.parse(js); status(); },
    write(bis) {
      writeUntil = bis;
      status();
      if (bis > Date.now() / 1000) toast(F("Du darfst mitschreiben bis %s – Tasten gehen jetzt an den Server", new Date(bis * 1000).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })));
      else toast(T("Mitschreiben beendet – nur zusehen"));
    },
    end(msg) {
      for (const id of ["hand", "leave", "smaller", "bigger"]) $(id).hidden = true;
      writeUntil = 0;
      status();
      $("endtitle").textContent = msg ? T("Nicht geklappt") : T("Zusehen beendet");
      $("endmsg").textContent = msg || T("Der Gastgeber hat das Teilen beendet.");
      if (term && !msg) {
        // Bild stehen lassen, Hinweis darüber
        toast(T("Der Gastgeber hat das Teilen beendet."));
        return;
      }
      show("end");
    },
  };

  function wsURL() {
    const u = new URL("../v1/teilen/ansehen", location.href);
    u.protocol = "wss:";
    u.hash = ""; u.search = "";
    return u.toString();
  }

  function knock(ev) {
    if (ev) ev.preventDefault();
    const err = $("err");
    err.hidden = true;
    const c = $("code").value.trim(), n = $("name").value.trim();
    const bad = sshtermy.codeOK(c);
    if (bad) { err.textContent = bad; err.hidden = false; return; }
    if (!n) { err.textContent = T("Bitte einen Namen eingeben"); err.hidden = false; $("name").focus(); return; }
    localStorage.setItem("sshtermy-name", n);
    $("waitmsg").textContent = T("Prüfe den Code …");
    $("waitmsg").hidden = false;
    $("checkbox").hidden = true;
    show("wait");
    const r = sshtermy.join({ code: c, ws: wsURL(), name: n, rechner: rechner(), lang: DE ? "de" : "en" }, handlers);
    if (r) handlers.end(r);
  }
  $("form").addEventListener("submit", knock);
  $("again").onclick = () => { sshtermy.leave(); show("start"); };
  $("leave").onclick = () => { sshtermy.leave(); };
  $("hand").onclick = () => { const r = sshtermy.hand(); toast(r || T("Hand gehoben – der Gastgeber sieht die Bitte ums Mitschreiben")); };

  // ---------------------------------------------------------------- Start
  if (location.protocol !== "https:") { $("endtitle").textContent = T("Nicht geklappt"); $("endmsg").textContent = T("Seite nur über https"); show("end"); return; }
  if (typeof WebAssembly !== "object") { $("endtitle").textContent = T("Nicht geklappt"); $("endmsg").textContent = T("Dieser Browser kann kein WebAssembly."); show("end"); return; }
  $("waitmsg").textContent = T("Lade …");
  show("wait");
  window.sshtermyReady = () => {
    show("start");
    (code ? $("name") : $("code")).focus();
  };
  const go = new Go();
  WebAssembly.instantiateStreaming(fetch("zusehen.wasm"), go.importObject)
    .then((r) => go.run(r.instance))
    .catch((e) => { $("endtitle").textContent = T("Nicht geklappt"); $("endmsg").textContent = String(e); show("end"); });
})();
