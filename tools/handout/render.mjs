// Druckbare Kurzreferenz (A4, 2 Seiten) als PDF für die Webseite.
//
//   node tools/handout/render.mjs            # schreibt site/handout/*.pdf (--html: auch die HTML-Vorschau)
//
// Braucht Playwright mit Chromium und die Schriften JetBrains Mono + Inter
// (fehlen sie, nimmt Chromium Ersatzschriften). Inhalt: content.mjs.
import { createRequire } from 'node:module';
import { readFileSync, writeFileSync, mkdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { de, en } from './content.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..', '..');
const out = join(root, 'site', 'handout');
const version = readFileSync(join(root, 'VERSION'), 'utf8').trim();
mkdirSync(out, { recursive: true });
const { chromium } = createRequire(import.meta.url)('playwright');

const esc = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

// Taste → Kästchen. "Strg+Space / F1" → zwei Alternativen; _Wort_ = Mausaktion.
function keyHTML(k) {
  return k.split(' / ').map((alt) => alt.split(' · ').map((part) => {
    if (/^_[^_]+_$/.test(part)) return `<span class="mouse">${esc(part.slice(1, -1))}</span>`;
    if (/\s/.test(part.trim()) && !/^(⏎ ⏎|F12 \w)$/.test(part) && !part.includes('…') && !part.startsWith('sshtermy')) {
      return `<span class="txt">${esc(part)}</span>`;
    }
    if (part.startsWith('sshtermy')) return `<code>${esc(part)}</code>`;
    if (part === '⏎ ⏎') return '<kbd>⏎</kbd><kbd>⏎</kbd>';
    if (/^F12 \w$/.test(part)) return `<kbd>F12</kbd><span class="then">›</span><kbd>${part.slice(4)}</kbd>`;
    const pieces = part === '+' || part === '-' ? [part] : part.split(/(?<=.)\+(?=.)/);
    return pieces.map((p) => {
      const m = p.match(/^_(.+)_$/);
      if (m) return `<span class="mouse">${esc(m[1])}</span>`;
      if (p.includes('…')) {
        const [a, b] = p.split('…').map((x) => x.trim());
        return `<kbd>${esc(a)}</kbd><span class="then">…</span><kbd>${esc(b)}</kbd>`;
      }
      return `<kbd>${esc(p)}</kbd>`;
    }).join('<span class="plus">+</span>');
  }).join('<span class="then">·</span>')).join('<span class="or">/</span>');
}

function box(b) {
  const cls = ['box', b.ai ? 'ai' : '', b.warn ? 'warn' : ''].join(' ');
  const rows = b.rows.map(([k, v, tag]) =>
    b.legend
      ? `<div class="k"><span class="lg ${tag || ''}">${esc(k)}</span></div><div class="v">${esc(v)}</div>`
      : `<div class="k">${keyHTML(k)}</div><div class="v ${tag || ''}">${esc(v)}</div>`).join('');
  const key = b.key ? `<span class="opens">${keyHTML(b.key)}</span>` : '';
  const note = b.note ? `<p class="note">${b.note}</p>` : '';
  return `<div class="wrap"><section class="${cls}"><h2><span class="ic">${esc(b.ic)}</span>${esc(b.t)}${key}</h2><div class="rows">${rows}</div>${note}</section></div>`;
}

function page(c, boxes, n) {
  const head = n === 1 ? `
  <header>
    <div class="win"><i></i><i></i><i></i><span>~/sshtermy — ${esc(c.title.toLowerCase())}</span></div>
    <div class="brand">
      <div class="logo"><span class="prompt">&gt;_</span>sshTermy<span class="cur"></span></div>
      <div class="meta"><div class="doc">${esc(c.title)}</div><div>${esc(c.sub)}</div><div class="credit">${esc(c.credit)} · v${esc(version)} · ${esc(c.stand)}</div></div>
    </div>
  </header>` : `
  <header class="small"><div class="logo"><span class="prompt">&gt;_</span>sshTermy</div><div class="doc">${esc(c.title)} · 2/2</div></header>`;
  return `<div class="page">${head}
  <main class="cols">${boxes.map(box).join('')}</main>
  <footer><span>${c.hint}</span><span class="url">${esc(c.web)}</span><span class="pg">${n}/2</span></footer>
</div>`;
}

const css = `
@page { size: A4; margin: 0; }
* { box-sizing: border-box; }
html, body { margin: 0; }
body { font-family: Inter, "Helvetica Neue", Arial, sans-serif; color: #15161c; font-size: 8.6pt; line-height: 1.27;
  -webkit-print-color-adjust: exact; print-color-adjust: exact; }
:root { --ink: #15161c; --dim: #5b6070; --line: #2a2d38; --soft: #c9ccd6; --ai: #6b3fc4; --warn: #c2283c; --acc: #1f5fd1; --ok: #1d7a3a;
  --mono: "JetBrains Mono", "DejaVu Sans Mono", monospace; }
.page { width: 210mm; height: 297mm; padding: 9mm 9mm 8mm; display: flex; flex-direction: column; page-break-after: always; overflow: hidden; }
.page:last-child { page-break-after: auto; }

header { flex-shrink: 0; border: 0.9pt solid var(--line); border-radius: 2.2mm; margin-bottom: 4.2mm; overflow: hidden; }
.win { display: flex; align-items: center; gap: 1.4mm; padding: 1.4mm 2.6mm; border-bottom: 0.6pt solid var(--soft); font: 6.6pt var(--mono); color: var(--dim); }
.win i { width: 2.3mm; height: 2.3mm; border-radius: 50%; border: 0.7pt solid var(--line); display: inline-block; }
.win i:nth-child(1) { border-color: var(--warn); } .win i:nth-child(2) { border-color: #c28a12; } .win i:nth-child(3) { border-color: var(--ok); }
.win span { margin-left: 2mm; }
.brand { display: flex; align-items: center; justify-content: space-between; padding: 3mm 4mm 3.2mm; }
.logo { font: 800 25pt/1 var(--mono); letter-spacing: -0.5pt; display: flex; align-items: center; }
.logo .prompt { color: var(--ai); margin-right: 2.4mm; font-weight: 700; }
.logo .cur { display: inline-block; width: 3.4mm; height: 7mm; background: var(--ok); margin-left: 1.4mm; }
.meta { text-align: right; color: var(--dim); font-size: 7.6pt; }
.meta .doc { font: 700 13pt var(--mono); color: var(--ink); text-transform: uppercase; letter-spacing: 1.6pt; margin-bottom: 0.6mm; }
.meta .credit { font-style: italic; margin-top: 0.4mm; }
header.small { display: flex; justify-content: space-between; align-items: center; padding: 2mm 4mm; }
header.small .logo { font-size: 13pt; }
header.small .doc { font: 700 8pt var(--mono); text-transform: uppercase; letter-spacing: 1.2pt; color: var(--dim); }

.cols { flex: 1 1 0; min-height: 0; column-count: 3; column-gap: 4.2mm; column-fill: balance; }
.wrap { break-inside: avoid; padding: 2.6mm 0 1.4mm; }
.box { border: 0.8pt solid var(--line); border-radius: 1.6mm; padding: 3.6mm 2.4mm 2.2mm; position: relative; }
.box h2 { position: absolute; top: -2.3mm; left: 2mm; right: 2mm; margin: 0; display: flex; align-items: center; font: 700 8pt var(--mono); text-transform: uppercase; letter-spacing: 0.7pt; white-space: nowrap; color: var(--ink); }
.box h2 > .ic { background: #fff; padding: 0 0.6mm 0 0.8mm; color: var(--acc); }
.box h2 span.opens { text-transform: none; letter-spacing: 0; font-weight: 500; color: var(--dim); margin-left: auto; background: #fff; padding: 0 0.8mm; }
.box.ai { border-color: var(--ai); }
.box.ai h2 > .ic { color: var(--ai); }
.box.warn { border-color: var(--warn); border-style: dashed; }
.box.warn h2 > .ic { color: var(--warn); }
.box h2 .t { background: #fff; padding-right: 1mm; }
.rows { display: grid; grid-template-columns: auto 1fr; column-gap: 2mm; row-gap: 1.2mm; align-items: center; }
.k { white-space: nowrap; display: flex; align-items: center; flex-wrap: nowrap; }
.v { color: #2b2e38; }
.v.ai { color: var(--ai); font-weight: 600; }
.v.warn { color: var(--warn); font-weight: 600; }
kbd { font: 600 7.6pt/1 var(--mono); border: 0.6pt solid #4a4e5c; border-bottom-width: 1.2pt; border-radius: 0.9mm; padding: 0.55mm 0.9mm 0.45mm; background: #fff; color: var(--ink); min-width: 3.4mm; text-align: center; display: inline-block; }
.opens kbd { font-size: 6pt; padding: 0.35mm 0.7mm; }
.plus, .then { font: 6pt var(--mono); color: var(--dim); margin: 0 0.35mm; }
.or { font: 6.4pt var(--mono); color: var(--dim); margin: 0 0.8mm; }
.mouse { font: italic 7.4pt Inter, sans-serif; color: var(--dim); border-bottom: 0.6pt dotted var(--dim); }
.txt { font: 7pt var(--mono); color: var(--ink); }
code { font: 600 6.8pt var(--mono); color: var(--ok); }
.note { margin: 1.6mm 0 0.2mm; padding-top: 1.4mm; border-top: 0.5pt dashed var(--soft); color: var(--dim); font-size: 7.2pt; }
.note b { color: var(--ai); }
.lg { font: 700 8.4pt var(--mono); color: var(--ink); }
.lg.ok { color: var(--ok); font-size: 10pt; } .lg.mid { color: #c28a12; font-size: 10pt; } .lg.bad { color: var(--warn); font-size: 10pt; }
.lg.ghost { color: #8a8f9e; font-weight: 400; font-style: italic; } .lg.danger { color: var(--warn); text-decoration: underline wavy var(--warn); text-underline-offset: 1pt; }
.lg.warn { color: #fff; background: var(--warn); padding: 0.4mm 1mm; border-radius: 0.6mm; font-size: 6.6pt; }
footer { flex-shrink: 0; display: flex; gap: 4mm; align-items: center; border-top: 0.8pt solid var(--line); padding-top: 1.8mm; font-size: 6.8pt; color: var(--dim); }
footer b { font-family: var(--mono); color: var(--ink); }
footer .url { margin-left: auto; font: 600 7pt var(--mono); color: var(--acc); }
footer .pg { font: 700 7pt var(--mono); color: var(--ink); }
`;

// Titel als eigenes Element mit weißem Hintergrund, damit die Rahmenlinie dahinter verschwindet.
const fixTitles = (html) => html.replace(/<span class="ic">([^<]*)<\/span>([^<]*)/g, '<span class="ic">$1</span><span class="t">$2</span>');

function doc(c) {
  return `<!doctype html><html lang="${c.lang}"><head><meta charset="utf-8"><title>sshTermy – ${esc(c.title)}</title><style>${css}</style></head>
<body>${fixTitles(page(c, c.page1, 1) + page(c, c.page2, 2))}</body></html>`;
}

const browser = await chromium.launch();
const pg = await browser.newPage();
for (const c of [de, en]) {
  const html = doc(c);
  if (process.argv.includes('--html')) writeFileSync(join(out, c.file.replace(/\.pdf$/, '.html')), html);
  await pg.setContent(html, { waitUntil: 'load' });
  await pg.evaluate(() => document.fonts.ready);
  // Passt eine Seite nicht, sieht man es hier statt erst auf dem Papier.
  const over = await pg.evaluate(() => [...document.querySelectorAll('.cols')].map((el) => el.scrollWidth > el.clientWidth + 1));
  if (over.some(Boolean)) console.warn(`${c.file}: Inhalt läuft über (Spalten zu voll)`);
  await pg.pdf({ path: join(out, c.file), format: 'A4', printBackground: true, preferCSSPageSize: true });
  console.log('geschrieben:', join('site', 'handout', c.file));
}
await browser.close();
