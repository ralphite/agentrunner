import { expect, test, type Page } from "@playwright/test";

interface GoldenCase {
  name: string;
  storyId: string;
  theme: "light" | "dark";
  viewport: { width: number; height: number };
}

const goldenCases: GoldenCase[] = [
  {
    name: "home-intent-light-desktop",
    storyId: "pages-home--starter-intent-flow",
    theme: "light",
    viewport: { width: 1280, height: 720 },
  },
  {
    name: "home-intent-dark-phone",
    storyId: "pages-home--starter-intent-flow",
    theme: "dark",
    viewport: { width: 390, height: 844 },
  },
  {
    name: "session-default-light-desktop",
    storyId: "components-sessions-sessionview--default",
    theme: "light",
    viewport: { width: 1280, height: 720 },
  },
  {
    name: "session-empty-light-phone",
    storyId: "components-sessions-sessionview--empty",
    theme: "light",
    viewport: { width: 390, height: 844 },
  },
  {
    name: "session-approval-dark-desktop",
    storyId: "components-sessions-sessionview--approval-required",
    theme: "dark",
    viewport: { width: 1280, height: 720 },
  },
  {
    name: "changes-dark-desktop",
    storyId: "components-changes-changesoutcome--default",
    theme: "dark",
    viewport: { width: 1280, height: 720 },
  },
  {
    name: "scheduled-light-phone",
    storyId: "pages-scheduled--default",
    theme: "light",
    viewport: { width: 390, height: 844 },
  },
  {
    name: "settings-no-results-dark-phone",
    storyId: "pages-settings--search-no-matches",
    theme: "dark",
    viewport: { width: 390, height: 844 },
  },
];

function collectRuntimeIssues(page: Page) {
  const issues: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") {
      issues.push(`console.error: ${message.text()}`);
    }
  });
  page.on("pageerror", (error) => {
    issues.push(`pageerror: ${error.message}`);
  });
  page.on("requestfailed", (request) => {
    if (/^(?:about|blob|data):/.test(request.url())) return;
    const failure = request.failure()?.errorText ?? "unknown failure";
    issues.push(
      `requestfailed: ${request.method()} ${request.url()} (${failure})`,
    );
  });
  return issues;
}

async function settleStory(page: Page, golden: GoldenCase) {
  const globals = encodeURIComponent(`theme:${golden.theme}`);
  await page.goto(
    `/iframe.html?id=${golden.storyId}&viewMode=story&globals=${globals}`,
    { waitUntil: "networkidle" },
  );
  const root = page.locator("#storybook-root");
  await expect(root).not.toBeEmpty();
  await expect(page.locator("html")).toHaveAttribute("data-theme", golden.theme);
  await page.evaluate(async () => {
    await document.fonts.ready;
  });
  await page.addStyleTag({
    content: `
      *, *::before, *::after {
        animation-delay: 0s !important;
        animation-duration: 0s !important;
        caret-color: transparent !important;
        transition-delay: 0s !important;
        transition-duration: 0s !important;
      }
    `,
  });
}

for (const golden of goldenCases) {
  test(golden.name, async ({ page }) => {
    const runtimeIssues = collectRuntimeIssues(page);
    await page.setViewportSize(golden.viewport);
    await settleStory(page, golden);
    expect(runtimeIssues).toEqual([]);
    await expect(page).toHaveScreenshot(`${golden.name}.png`, {
      animations: "disabled",
      caret: "hide",
      scale: "css",
      maxDiffPixelRatio: 0.03,
    });
    expect(runtimeIssues).toEqual([]);
  });
}
