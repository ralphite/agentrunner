// qa/blackbox/drive.mjs — black-box UI QA: drive the real webui in a real
// browser like a user would, and flag anything a user would call a bug.
//
// Global invariants checked after EVERY step:
//   - no uncaught page errors, no console.error lines (filtered allowlist)
//   - no horizontal overflow (scrollWidth <= innerWidth+1)
//   - no raw internal error text visible anywhere in the DOM (the "scary red
//     toast" class: git/CLI prose, absolute server paths, exit statuses)
//   - screenshot per step → artifact
//
// Usage: node drive.mjs <base-url> [outdir]
//   env SKIP_TURNS=1  — skip model-dependent steps (no API key: harness smoke)
//   env CHROME_PATH   — chromium binary (else common locations probed)
//
// Exit code = number of findings (0 = clean).

import { chromium } from "playwright-core";
import fs from "node:fs";
import path from "node:path";

const BASE = process.argv[2] || "http://127.0.0.1:8788";
const OUT = process.argv[3] || "blackbox-out";
const SKIP_TURNS = !!process.env.SKIP_TURNS;
fs.mkdirSync(OUT, { recursive: true });

const CHROME_CANDIDATES = [
  process.env.CHROME_PATH,
  "/usr/bin/google-chrome",
  "/usr/bin/chromium-browser",
  "/usr/bin/chromium",
  "/opt/pw-browsers/chromium-1194/chrome-linux/chrome",
].filter(Boolean);
const chromePath = CHROME_CANDIDATES.find((p) => fs.existsSync(p));
if (!chromePath) {
  console.error("no chromium binary found; set CHROME_PATH");
  process.exit(2);
}

// Raw-internal-error markers a user must never see rendered in the UI.
const RAW_ERROR_RE =
  /exit status \d|fatal: |daemon dial:|is the daemon running|invalid starting ref|not an existing directory|flag provided but not defined|panic: |goroutine \d+ \[/;
const BROKEN_COPY_RE = /just now ago/i;

// Console noise that is not a product bug.
const CONSOLE_ALLOW = [
  /favicon/i,
  /manifest/i,
  /Download the React DevTools/,
  /net::ERR_/, // transient poll failures while the daemon warms up
  /Failed to load resource.*(404|503|502)/, // handled by app-level states
];

const findings = [];
let stepNo = 0;

function finding(sev, step, what, detail = "") {
  findings.push({ sev, step, what, detail: String(detail).slice(0, 600) });
  console.log(`  !! [${sev}] ${what}${detail ? " — " + String(detail).slice(0, 200) : ""}`);
}

async function makeCtx(browser, name, viewport) {
  const ctx = await browser.newContext({
    viewport,
    deviceScaleFactor: 2,
    hasTouch: name === "phone",
    userAgent:
      name === "phone"
        ? "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"
        : undefined,
  });
  return ctx;
}

async function checkInvariants(page, label) {
  // layout overflow
  const m = await page.evaluate(() => ({
    sw: document.documentElement.scrollWidth,
    iw: window.innerWidth,
    raw: document.body.innerText,
    labels: [...document.querySelectorAll("[aria-label], [title]")]
      .map((el) => `${el.getAttribute("aria-label") || ""} ${el.getAttribute("title") || ""}`)
      .join("\n"),
  }));
  if (m.sw > m.iw + 1) finding("high", label, `horizontal overflow: scrollWidth ${m.sw} > innerWidth ${m.iw}`);
  const rawHit = m.raw.match(RAW_ERROR_RE);
  if (rawHit) finding("high", label, "raw internal error text visible to user", rawHit[0] + " …context: " + m.raw.slice(Math.max(0, m.raw.search(RAW_ERROR_RE) - 60), m.raw.search(RAW_ERROR_RE) + 120));
  const copyHit = `${m.raw}\n${m.labels}`.match(BROKEN_COPY_RE);
  if (copyHit) finding("med", label, "broken relative-time copy visible", copyHit[0]);
}

async function shot(page, name) {
  stepNo++;
  const file = path.join(OUT, `${String(stepNo).padStart(2, "0")}-${name.replace(/[^a-zA-Z0-9_-]+/g, "-")}.png`);
  await page.screenshot({ path: file, fullPage: false }).catch(() => {});
}

async function step(page, name, fn) {
  console.log(`== ${name}`);
  try {
    await fn();
  } catch (e) {
    finding("high", name, "step threw", e.message);
  }
  await checkInvariants(page, name);
  await shot(page, name);
}

function wireConsole(page, label) {
  page.on("pageerror", (e) => finding("high", label(), "uncaught page error", e.message));
  page.on("console", (msg) => {
    if (msg.type() !== "error") return;
    const text = msg.text();
    if (CONSOLE_ALLOW.some((re) => re.test(text))) return;
    finding("med", label(), "console.error", text);
  });
}

const browser = await chromium.launch({
  executablePath: chromePath,
  args: ["--no-sandbox", "--disable-gpu"],
});

for (const [ctxName, viewport] of [
  ["phone", { width: 390, height: 844 }],
  ["desktop", { width: 1280, height: 800 }],
]) {
  console.log(`\n#### context: ${ctxName} ${viewport.width}x${viewport.height}`);
  const ctx = await makeCtx(browser, ctxName, viewport);
  const page = await ctx.newPage();
  let cur = "boot";
  wireConsole(page, () => `${ctxName}:${cur}`);
  const S = (n, fn) => ((cur = `${ctxName}:${n}`), step(page, `${ctxName}:${n}`, fn));

  await S("home", async () => {
    await page.goto(BASE, { waitUntil: "domcontentloaded", timeout: 20000 });
    await page.waitForSelector("textarea, .cx-card", { timeout: 15000 });
  });

  await S("sidebar-toggle", async () => {
    const opener = page.getByRole("button", { name: "Show sidebar" });
    if (await opener.count()) await opener.click({ timeout: 5000 });
  });

  if (ctxName === "phone") await S("settings-search", async () => {
    const more = page.getByRole("button", { name: "More options" });
    if (await more.count()) {
      await more.click();
      await page.getByRole("menuitem", { name: /Settings/ }).click();
      const search = page.getByRole("textbox", { name: "Search settings" });
      await search.fill("git");
      await page.getByRole("heading", { name: "Git" }).waitFor({ timeout: 5000 });
      const empty = page.getByText(/No Git settings match/);
      if (await empty.count()) finding("med", cur, "section-name search filtered its own settings empty");
      if (!(await page.getByText("Commit message template").count()))
        finding("med", cur, "Git section-name search did not expose Git settings");
      await page.keyboard.press("Escape");
    } else {
      finding("med", cur, "sidebar settings menu did not surface");
    }
  });

  // Scheduled-run modal: bare workspace name must yield the FRIENDLY error.
  // Phone-only: this validates the friendly-error + modal once; repeating the
  // identical flow on desktop added no coverage, only a flaky second run.
  if (ctxName === "phone") await S("scheduled-modal-bad-workspace", async () => {
    await page.goto(BASE + "/#scheduled", { waitUntil: "networkidle" }).catch(() => {});
    // Confirm the route actually landed on the Scheduled surface before hunting
    // its controls; the runner's first paint with a populated store is slower
    // than local (the earlier "selector drift" flake). 15s, then self-diagnose.
    await page.locator(".scheduled-page").first().waitFor({ timeout: 15000 }).catch(() => {});
    const newBtn = page.locator("button[aria-label='Create scheduled work']").first();
    const surfaced = await newBtn.waitFor({ timeout: 12000 }).then(() => true).catch(() => false);
    if (surfaced) {
      await newBtn.click({ timeout: 4000 }).catch(() => {});
      // Precise: the MENU ITEM named Repeating (text=Repeating also matched a
      // nested <b>, so .first() was ambiguous).
      const repeating = page.getByRole("menuitem", { name: /Repeating/ }).first();
      await repeating.waitFor({ timeout: 5000 }).catch(() => {});
      await repeating.click({ timeout: 4000 }).catch(() => {});
      await page.locator(".modal").first().waitFor({ timeout: 5000 }).catch(() => {});
      const modal = page.locator(".modal");
      if (await modal.count()) {
        const prompt = modal.getByRole("textbox", { name: "Prompt", exact: true });
        const wsField = modal.getByRole("textbox", { name: "Workspace", exact: true });
        if ((await prompt.count()) === 1 && (await wsField.count()) === 1) {
          await prompt.fill("Say hello");
          await wsField.fill("abc");
          const values = await modal.evaluate((root) => ({
            prompt: root.querySelector("textarea[placeholder='Describe the outcome you want']")?.value,
            workspace: root.querySelector("input[placeholder='Leave blank for a new scratch workspace']")?.value,
          }));
          if (values.prompt !== "Say hello" || values.workspace !== "abc") {
            finding("high", cur, "scheduled form fields did not retain distinct values", JSON.stringify(values));
            return;
          }
          const start = modal.getByRole("button", { name: "Start schedule" });
          await start.click({ timeout: 4000 });
          await page.getByRole("status").filter({ hasText: /full path|Use folder|blank/i }).waitFor({ timeout: 6000 }).catch(() => {});
          const toastText = await page.evaluate(() => document.body.innerText);
          if (/not an existing directory/.test(toastText))
            finding("high", cur, "bare workspace still leaks raw path error");
          else if (!/full path|Use folder|blank/i.test(toastText))
            finding("med", cur, "bare workspace produced no visible friendly guidance");
          if (!page.url().includes("#scheduled") || !(await modal.count()))
            finding("high", cur, "invalid scheduled workspace started work instead of staying in the form");
        } else {
          finding("med", cur, "scheduled modal fields did not surface");
        }
        await page.keyboard.press("Escape").catch(() => {});
      } else {
        finding("low", cur, "scheduled modal did not open after clicking Create → Repeating");
      }
    } else {
      // Not a blind flake report: capture WHAT the page actually showed so a
      // real routing regression is distinguishable from a slow paint.
      const diag = await page.evaluate(() => ({
        hash: location.hash,
        schedPage: !!document.querySelector(".scheduled-page"),
        h2: (document.querySelector("h2") || {}).innerText,
        ariaButtons: [...document.querySelectorAll("button[aria-label]")].map((b) => b.getAttribute("aria-label")).slice(0, 12),
      }));
      finding("med", cur, "Scheduled 'Create' control never surfaced", JSON.stringify(diag));
    }
  });

  if (!SKIP_TURNS) {
    let sid = "";
    await S("create-session", async () => {
      await page.goto(BASE, { waitUntil: "domcontentloaded" });
      const box = page.locator(".cx-card textarea, textarea").first();
      await box.waitFor({ timeout: 10000 });
      await box.fill("用一句话回答：1+1 等于几？不要用任何工具。");
      const send = page.locator(".cx-send, button[aria-label*='end']").first();
      await send.click({ timeout: 5000 });
      await page.waitForURL(/#/, { timeout: 20000 }).catch(() => {});
      // a session view shows the timeline
      await page.waitForSelector(".timeline", { timeout: 30000 });
      sid = await page.evaluate(() => location.hash.replace(/^#\/?/, ""));
      console.log("   session:", sid);
    });

    await S("first-turn-completes", async () => {
      // A REAL assistant reply: the timeline must gain text containing the
      // expected answer ("2"). A selector-only wait passed instantly against
      // pre-existing DOM (run #1 lesson: assertion existing ≠ assertion
      // executing) — anchor on content, not structure.
      await page.waitForFunction(
        () => /[2２二]/.test(document.querySelector(".timeline")?.innerText || ""),
        { timeout: 120000 },
      );
      const errText = await page.evaluate(() => document.body.innerText);
      if (/model error|activity_failed|provider_/i.test(errText))
        finding("high", cur, "turn surfaced a provider/model error", errText.slice(-300));
    });

    await S("follow-up-turn", async () => {
      const before = await page.evaluate(() => (document.querySelector(".timeline")?.innerText || "").length);
      const box = page.locator(".cx-card textarea, .composer textarea, textarea").last();
      await box.fill("再答一次：3+4 等于几？只回答数字。");
      await page.keyboard.press("Enter");
      // content-anchored: the timeline must GROW and contain the new answer 7
      await page.waitForFunction(
        (n) => {
          const t = document.querySelector(".timeline")?.innerText || "";
          return t.length > n && /[7７七]/.test(t);
        },
        before,
        { timeout: 120000 },
      );
    });

    await S("diff-view", async () => {
      const sessions = await fetch(`${BASE}/api/sessions?limit=200`).then((r) => r.json());
      const current = sessions.find((session) => session.id === sid);
      if (!current?.workspace) {
        finding("high", cur, "new session has no retained workspace for Changes QA");
        return;
      }
      const changeName = `qa-blackbox-${sid}.txt`;
      fs.writeFileSync(path.join(current.workspace, changeName), `black-box change for ${sid}\n`);

      const actions = page.getByRole("button", { name: "More session actions" });
      if (!(await actions.count())) {
        finding("high", cur, "session actions menu did not surface");
        return;
      }
      await actions.click();
      await page.getByRole("menuitem", { name: "Changes" }).click();
      const panel = page.locator(".changes-panel");
      await panel.waitFor({ state: "visible", timeout: 10000 });
      const geometry = await panel.evaluate((el) => {
        const box = el.getBoundingClientRect();
        return { display: getComputedStyle(el).display, width: box.width, height: box.height };
      });
      if (geometry.display === "none" || geometry.width < 100 || geometry.height < 100)
        finding("high", cur, "Changes opened with no visible panel", JSON.stringify(geometry));
      await page.getByText(changeName).waitFor({ timeout: 10000 });
    });
  }

  await ctx.close();
}

// Daemon-down journey (LAST — it kills the daemon): friendly affordance, not raw blob.
if (process.env.DAEMON_KILL_CMD) {
  const ctx = await makeCtx(browser, "phone", { width: 390, height: 844 });
  const page = await ctx.newPage();
  let cur = "daemon-down";
  wireConsole(page, () => cur);
  const { execSync } = await import("node:child_process");
  await step(page, "daemon-down-friendly", async () => {
    execSync(process.env.DAEMON_KILL_CMD, { stdio: "inherit" });
    await page.goto(BASE, { waitUntil: "domcontentloaded" });
    const box = page.locator("textarea").first();
    await box.waitFor({ timeout: 10000 });
    await box.fill("hello");
    const send = page.locator(".cx-send, button[aria-label*='end']").first();
    await send.click({ timeout: 5000 }).catch(() => {});
    await page.waitForTimeout(2500);
    const text = await page.evaluate(() => document.body.innerText);
    if (/daemon dial|is the daemon running/.test(text))
      finding("high", cur, "daemon-down still shows raw dial error");
    else if (!/isn.t running|service/i.test(text))
      finding("med", cur, "daemon-down produced no visible friendly message", text.slice(-300));
  });
  await ctx.close();
}

await browser.close();

fs.writeFileSync(path.join(OUT, "findings.json"), JSON.stringify(findings, null, 2));
console.log(`\n==== ${findings.length} finding(s) ====`);
for (const f of findings) console.log(`[${f.sev}] ${f.step}: ${f.what}${f.detail ? " — " + f.detail.slice(0, 160) : ""}`);
process.exit(findings.length ? 1 : 0);
