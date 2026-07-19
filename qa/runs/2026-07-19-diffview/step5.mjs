// step5: reconcile split-view DOM rows for bigmodule.py against the diff facts
import { chromium } from '/home/user/agentrunner/qa/blackbox/node_modules/playwright-core/index.mjs';
const SID = '20260719-220109-say-hi-ec60af753aabc198';
const browser = await chromium.launch({ executablePath: '/opt/pw-browsers/chromium', headless: true });
const page = await browser.newPage({ viewport: { width: 1920, height: 1080 } });
const errors = [];
page.on('console', m => { if (m.type() === 'error') errors.push(m.text()); });
await page.goto(`http://127.0.0.1:8788/#/s/${SID}`, { waitUntil: 'domcontentloaded' });
await page.waitForTimeout(2500);
await page.getByRole('button', { name: 'Review', exact: true }).click();
await page.waitForSelector('.diffwrap .filediff', { timeout: 15000 });
await page.locator('button[title="Split view"]').click();
await page.waitForTimeout(800);
const m = await page.evaluate(() => {
  const cards = [...document.querySelectorAll('.filediff')];
  const big = cards.find(c => c.querySelector('.fd-path')?.textContent?.includes('bigmodule'));
  const rows = [...big.querySelectorAll('.dls')];
  return {
    rowCount: rows.length,
    first6: rows.slice(0, 6).map(r => {
      const nos = [...r.querySelectorAll('.dl-no')].map(n => n.textContent);
      const halves = [...r.querySelectorAll('.dls-half')].map(h => ({ text: h.textContent.slice(0, 50), w: h.getBoundingClientRect().width, h: h.getBoundingClientRect().height }));
      return { nos, halves, rect: { h: r.getBoundingClientRect().height } };
    }),
  };
});
console.log(JSON.stringify({ m, errors }, null, 1));
await browser.close();
