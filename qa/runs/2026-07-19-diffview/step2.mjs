// step2: open Changes (Review), measure squash + scrollability, screenshot (RED evidence)
import { chromium } from '/home/user/agentrunner/qa/blackbox/node_modules/playwright-core/index.mjs';

const SID = '20260719-220109-say-hi-ec60af753aabc198';
const vp = process.argv[2] === 'desktop' ? { width: 1280, height: 800 } : { width: 390, height: 844 };
const tag = process.argv[2] === 'desktop' ? 'desktop' : 'phone';
const browser = await chromium.launch({ executablePath: '/opt/pw-browsers/chromium', headless: true });
const page = await browser.newPage({ viewport: vp });
const errors = [];
page.on('console', m => { if (m.type() === 'error') errors.push(m.text()); });
await page.goto(`http://127.0.0.1:8788/#/s/${SID}`, { waitUntil: 'domcontentloaded' });
await page.waitForTimeout(2500);
await page.getByRole('button', { name: 'Review', exact: true }).click();
await page.waitForSelector('.diffwrap .filediff', { timeout: 15000 });
await page.waitForTimeout(1500);

const m = await page.evaluate(() => {
  const dw = document.querySelector('.diffwrap');
  const cards = [...document.querySelectorAll('.diffwrap .filediff')];
  const cs = getComputedStyle(dw);
  const before = dw.scrollTop;
  dw.scrollTop = 500;
  const afterOwn = dw.scrollTop;
  dw.scrollTop = before;
  const panel = document.querySelector('.changes-panel');
  return {
    diffwrap: { clientH: dw.clientHeight, scrollH: dw.scrollHeight, overflowY: cs.overflowY, canScroll: afterOwn > 0 },
    panel: panel ? { clientH: panel.clientHeight, scrollH: panel.scrollHeight, overflowY: getComputedStyle(panel).overflowY } : null,
    cards: cards.map(c => ({
      path: c.querySelector('.fd-path')?.textContent?.slice(-30),
      open: c.open,
      clientH: c.clientHeight,
      scrollH: c.scrollHeight,
    })),
    hscroll: document.documentElement.scrollWidth - window.innerWidth,
  };
});
console.log(JSON.stringify({ tag, m, errors }, null, 1));
await page.screenshot({ path: new URL(`./shot2-diff-${tag}-green.png`, import.meta.url).pathname });
await browser.close();
