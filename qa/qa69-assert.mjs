// QA-69 browser assertions (real Chromium layout — the folding decision needs
// real scrollHeight, which jsdom cannot provide; that is WHY these two SPEC
// rows had no scripted anchor until now).
import { createRequire } from "node:module";
const require = createRequire(process.env.NODE_PATH + "/");
const { chromium } = require("playwright");

const { SID, ADDR, RUNDIR } = process.env;
const fail = (msg) => {
  console.error("QA-69 FAIL:", msg);
  process.exit(1);
};

const launch = async () => {
  try {
    return await chromium.launch();
  } catch {
    return await chromium.launch({ executablePath: "/opt/pw-browsers/chromium" });
  }
};

const browser = await launch();
const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });

// ---- A. user-message folding (INC-36 red lines) ----
await page.goto(`http://${ADDR}/#s/${SID}`, { waitUntil: "domcontentloaded" });
const utext = page.locator(".utext").first();
await utext.waitFor({ timeout: 15000 });
await page.screenshot({ path: `${RUNDIR}/a1-clamped.png`, fullPage: true });
if (!(await utext.getAttribute("class")).includes("clamped")) fail("tall user message is not clamped");
const clampedH = (await utext.boundingBox()).height;
const show = page.locator("button.ushow");
if ((await show.textContent()).trim() !== "Show more") fail("Show more toggle missing on clamped message");
await show.click();
await page.waitForTimeout(200);
const openH = (await utext.boundingBox()).height;
if (!(openH > clampedH + 50)) fail(`expanding did not grow the message (clamped ${clampedH}px → open ${openH}px)`);
if ((await show.textContent()).trim() !== "Show less") fail("toggle did not flip to Show less");
await show.click();
await page.waitForTimeout(200);
if (!(await utext.getAttribute("class")).includes("clamped")) fail("Show less did not re-clamp");
await page.screenshot({ path: `${RUNDIR}/a2-expanded-collapsed.png`, fullPage: true });
console.log(`A. folding ok (clamped ${Math.round(clampedH)}px, open ${Math.round(openH)}px)`);

// ---- B. composer progressive disclosure (root action groups) ----
await page.goto(`http://${ADDR}/`, { waitUntil: "domcontentloaded" });
const add = page.locator('button[aria-label="Add and advanced options"]');
await add.waitFor({ timeout: 15000 });
await add.click();
const menu = page.locator(".cx-add-menu");
await menu.waitFor({ timeout: 5000 });
const labels = await menu.locator(".pop-section-label").allTextContents();
const want = ["Add", "Advanced"];
if (JSON.stringify(labels) !== JSON.stringify(want)) {
  fail(`add-menu groups = ${JSON.stringify(labels)}, want ${JSON.stringify(want)}`);
}
const rootItems = await menu.locator("[role=menuitem] .pop-title").allTextContents();
const rootWant = ["Files and folders", "Goal", "Plan mode", "Automation"];
if (JSON.stringify(rootItems) !== JSON.stringify(rootWant)) {
  fail(`add-menu root actions = ${JSON.stringify(rootItems)}, want ${JSON.stringify(rootWant)}`);
}
await page.screenshot({ path: `${RUNDIR}/b1-composer-menu.png`, fullPage: true });
await menu.getByRole("menuitem", { name: /^Automation/ }).click();
const automationItems = await menu.locator("[role=menuitem] .pop-title").allTextContents();
const automationWant = ["Loop", "Best of N", "Background run", "Agent"];
if (JSON.stringify(automationItems) !== JSON.stringify(automationWant)) {
  fail(`Automation actions = ${JSON.stringify(automationItems)}, want ${JSON.stringify(automationWant)}`);
}
await menu.getByRole("menuitem", { name: /^Agent/ }).click();
if (await menu.getByRole("menuitem", { name: /Edit agent spec \(YAML\)/ }).count() !== 1) {
  fail("Agent submenu does not expose Edit agent spec (YAML)");
}
await page.screenshot({ path: `${RUNDIR}/b2-automation-agent-menu.png`, fullPage: true });
console.log(`B. composer disclosure ok (${rootItems.length} exact root actions; Automation/Agent nested)`);

await browser.close();
