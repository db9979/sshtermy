// SVG → PNG mit Chromium (Playwright), transparenter Hintergrund.
import { createRequire } from 'module';
import { readFileSync } from 'fs';
const { chromium } = createRequire(import.meta.url)('playwright');
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';
const dir = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'assets', 'icon');
const jobs = [
  ['sshtermy.svg', 'mac', [1024]],
  ['sshtermy-full.svg', 'sshtermy', [128, 256, 512, 1024]],
  ['sshtermy-small.svg', 'sshtermy', [16, 24, 32, 48, 64]],
];
const b = await chromium.launch();
const p = await b.newPage({ viewport: { width: 1024, height: 1024 } });
for (const [svg, prefix, sizes] of jobs) {
  await p.setContent(`<body style="margin:0;background:transparent">${readFileSync(join(dir, svg), 'utf8')}</body>`);
  for (const s of sizes) {
    // Chromium verkleinert selbst (bessere Glut als Nachskalieren); 1024 als Ausgangsgröße
    await p.setViewportSize({ width: s, height: s });
    await p.evaluate(s => { const e = document.querySelector('svg'); e.setAttribute('width', s); e.setAttribute('height', s); }, s);
    await p.screenshot({ path: join(dir, `${prefix}-${s}.png`), omitBackground: true });
  }
}
await b.close();
