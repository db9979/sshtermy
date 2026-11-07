const { chromium } = require('playwright');
(async () => {
  const dir = process.argv[2], n = +process.argv[3];
  const b = await chromium.launch(); const p = await b.newPage({ deviceScaleFactor: 1, viewport: { width: 1200, height: 900 } });
  const fs = require('fs');
  for (let i = 0; i < n; i++) {
    const f = `${dir}/${String(i).padStart(4, '0')}`;
    await p.setContent(fs.readFileSync(f + '.html', 'utf8'));
    await p.locator('.win').screenshot({ path: f + '.png' });
  }
  await b.close();
})();
