// Real-environment browser gate for INC-96.
//
// Usage:
//   CHROME_PATH=/path/to/chrome node inc96-agent-catalog.mjs \
//     http://127.0.0.1:8809 ../runs/2026-07-23-INC96-agent-catalog

import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { chromium } from "playwright-core";

const base = process.argv[2] || "http://127.0.0.1:8809";
const out = path.resolve(process.argv[3] || "inc96-out");
const executablePath = process.env.CHROME_PATH;
if (!executablePath) throw new Error("CHROME_PATH is required");
fs.mkdirSync(out, { recursive: true });

const browser = await chromium.launch({
  executablePath,
  headless: true,
  args: ["--no-sandbox", "--disable-gpu"],
});
const context = await browser.newContext({ viewport: { width: 1100, height: 700 }, deviceScaleFactor: 2 });
const page = await context.newPage();
const browserLogs = [];
let launchBody = null;

page.on("pageerror", (error) => browserLogs.push({ type: "pageerror", text: error.message }));
page.on("console", (message) => {
  if (message.type() === "error" || message.type() === "warning") {
    browserLogs.push({ type: message.type(), text: message.text() });
  }
});
page.on("request", (request) => {
  if (request.method() === "POST" && request.url() === `${base}/api/sessions`) {
    launchBody = request.postDataJSON();
  }
});

try {
  await page.goto(base, { waitUntil: "networkidle", timeout: 30_000 });
  assert.equal(page.url(), `${base}/`);
  await page.getByPlaceholder("Do anything").waitFor({ timeout: 15_000 });
  assert.equal(await page.locator("body").evaluate((node) => node.scrollWidth <= window.innerWidth + 1), true);

  await page.getByRole("button", { name: "Add and advanced options" }).click();
  await page.getByRole("menuitem", { name: /Automation/ }).click();
  await page.getByRole("menuitem", { name: /^Agent / }).click();

  const titles = (await page.locator(".cx-add-agent .pop-title").allTextContents()).map((text) => text.trim());
  for (const label of ["Dev", "Team Lead", "Auditor", "Reviewer", "Chat", "Worker", "Explore", "Plan"]) {
    assert(titles.includes(label), `catalog is missing ${label}: ${titles.join(", ")}`);
  }
  const sources = await page.locator(".cx-add-agent .pop-desc").allTextContents();
  assert(sources.filter((text) => text.includes("· shipped")).length >= 8, "catalog source labels are missing");
  await page.screenshot({ path: path.join(out, "01-agent-catalog.png") });

  await page.getByRole("menuitem", { name: /^Chat / }).click();
  await page.locator('button[title="Model & effort"]').click();
  await page.getByRole("menuitem", { name: /^Effort / }).click();
  await page.getByRole("menuitem", { name: /^Light / }).click();
  assert.match(await page.locator('button[title="Model & effort"]').innerText(), /Gemini Flash[\s\S]*Light/);
  await page.screenshot({ path: path.join(out, "02-chat-light.png") });

  await page.getByPlaceholder("Do anything").fill("Reply with exactly INC96-BROWSER-OK and nothing else.");
  await page.getByPlaceholder("Do anything").press("Enter");
  await page.waitForFunction(() => location.hash.length > 1, null, { timeout: 30_000 });
  const sid = await page.evaluate(() => location.hash.replace(/^#\/?s?\//, "").replace(/^#/, ""));
  await page.getByText("INC96-BROWSER-OK", { exact: true }).waitFor({ timeout: 90_000 });
  await page.locator('button[aria-label="Stop active turn"]').waitFor({ state: "detached", timeout: 30_000 });

  assert(launchBody, "frontend did not POST /api/sessions");
  assert.deepEqual(
    {
      provider: launchBody.provider,
      model: launchBody.model,
      effort: launchBody.effort,
      specHasModel: /(^|\n)model:/.test(launchBody.spec),
    },
    {
      provider: "gemini",
      model: "gemini-flash-latest",
      effort: "light",
      specHasModel: false,
    },
  );
  await page.screenshot({ path: path.join(out, "03-session-complete.png") });

  await page.reload({ waitUntil: "domcontentloaded", timeout: 30_000 });
  assert.equal(await page.evaluate(() => location.hash.length > 1), true);
  await page.getByText("INC96-BROWSER-OK", { exact: true }).waitFor({ timeout: 20_000 });
  await page.getByText("Connected", { exact: true }).waitFor({ timeout: 20_000 });
  await page.getByText("Loading changes…", { exact: true }).waitFor({ state: "detached", timeout: 20_000 }).catch(() => {});
  await page.screenshot({ path: path.join(out, "04-session-reload.png") });
  assert.deepEqual(browserLogs, []);

  fs.writeFileSync(
    path.join(out, "result.json"),
    JSON.stringify({ base, sid, launchBody, browserLogs, url: page.url() }, null, 2) + "\n",
  );
  console.log(JSON.stringify({ status: "PASS", sid, url: page.url(), browserLogs }, null, 2));
} finally {
  await browser.close();
}
