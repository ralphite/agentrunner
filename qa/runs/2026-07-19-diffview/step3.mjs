// step3: deeper usability sweep on phone after fix — sticky bar under scroll,
// bottom of list reachable, per-file horizontal code scroll, wrap toggle path.
import { chromium } from '/home/user/agentrunner/qa/blackbox/node_modules/playwright-core/index.mjs';

const SID = '20260719-220109-say-hi-ec60af753aabc198';
const browser = await chromium.launch({ executablePath: '/opt/pw-browsers/chromium', headless: true });
const page = await browser.newPage({ viewport: { width: 390, height: 844 } });
const errors = [];
page.on('console', m => { if (m.type() === 'error') errors.push(m.text()); });
await page.goto(`http://127.0.0.1:8788/#/s/${SID}`, { waitUntil: 'domcontentloaded' });
await page.waitForTimeout(2500);
await page.getByRole('button', { name: 'Review', exact: true }).click();
await page.waitForSelector('.diffwrap .filediff', { timeout: 15000 });
await page.waitForTimeout(1000);

// 1) scroll to bottom — last card must be fully laid out & visible
const bottom = await page.evaluate(() => {
  const dw = document.querySelector('.diffwrap');
  dw.scrollTop = dw.scrollHeight;
  const bar = document.querySelector('.diffbar').getBoundingClientRect();
  const cards = [...document.querySelectorAll('.filediff')];
  const last = cards[cards.length - 1].getBoundingClientRect();
  return { barTop: bar.top, barVisible: bar.top >= 0 && bar.bottom > 0, lastCardBottom: last.bottom, innerH: window.innerHeight, scrolledTo: dw.scrollTop };
});
await page.screenshot({ path: new URL('./shot3-bottom-green.png', import.meta.url).pathname });

// 2) per-file horizontal scroll of long code lines
const hscroll = await page.evaluate(() => {
  const bodies = [...document.querySelectorAll('.fd-body')];
  const b = bodies.find(x => x.scrollWidth > x.clientWidth);
  if (!b) return { anyOverflowing: false };
  const before = b.scrollLeft;
  b.scrollLeft = 200;
  const moved = b.scrollLeft > before;
  b.scrollLeft = before;
  return { anyOverflowing: true, canScrollX: moved, scrollW: b.scrollWidth, clientW: b.clientWidth };
});

// 3) wrap toggle lives in "…" on tight bar — exercise it end to end
await page.evaluate(() => { document.querySelector('.diffwrap').scrollTop = 0; });
await page.getByRole('button', { name: 'More changes actions' }).click();
await page.waitForTimeout(400);
const wrapItem = await page.getByText('Wrap long lines', { exact: false }).count();
let wrapWorks = null;
if (wrapItem > 0) {
  await page.getByText('Wrap long lines', { exact: false }).first().click();
  await page.waitForTimeout(600);
  wrapWorks = await page.evaluate(() => {
    const bodies = [...document.querySelectorAll('.fd-body')];
    return { wrapClass: !!document.querySelector('.diffwrap.diff-wrap'), stillOverflowing: bodies.some(x => x.scrollWidth > x.clientWidth + 2) };
  });
  await page.screenshot({ path: new URL('./shot3-wrapped-green.png', import.meta.url).pathname });
}
console.log(JSON.stringify({ bottom, hscroll, wrapItem, wrapWorks, errors }, null, 1));
await browser.close();
