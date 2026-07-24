import { expect, test, type Page } from "@playwright/test";
import { globalStatePairs } from "../../src/storybook/storyManifest";

function collectRuntimeIssues(page: Page) {
  const issues: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") {
      issues.push(`console.error: ${message.text()}`);
    }
  });
  page.on("pageerror", (error) => issues.push(`pageerror: ${error.message}`));
  page.on("requestfailed", (request) => {
    if (/^(?:about|blob|data):/.test(request.url())) return;
    issues.push(
      `requestfailed: ${request.method()} ${request.url()} (${request.failure()?.errorText ?? "unknown"})`,
    );
  });
  return issues;
}

for (const pair of globalStatePairs) {
  test(`global pair: ${pair.pairId}`, async ({ page }) => {
    const runtimeIssues = collectRuntimeIssues(page);
    await page.setViewportSize(pair.viewport);
    const globals = encodeURIComponent(`theme:${pair.theme}`);
    const url = `/iframe.html?id=${pair.storyId}&viewMode=story&globals=${globals}`;

    const expectStoryReady = async () => {
      await expect(page.locator("#storybook-root")).not.toBeEmpty();
      await expect(page.locator("html")).toHaveAttribute(
        "data-theme",
        pair.theme,
      );
      await expect(page.locator(pair.evidenceSelector)).toBeVisible();
    };

    // Stories may intentionally keep mocked streams or polling requests open.
    // Readiness is the rendered Story root and its state-specific evidence,
    // not a globally idle network.
    await page.goto(url, { waitUntil: "domcontentloaded" });
    await expectStoryReady();

    if (pair.reload) {
      const issuesBeforeReload = runtimeIssues.length;
      await page.reload({ waitUntil: "domcontentloaded" });
      const reloadIssues = runtimeIssues.splice(issuesBeforeReload);
      runtimeIssues.push(
        ...reloadIssues.filter(
          (issue) =>
            !issue.startsWith("requestfailed:") ||
            !issue.endsWith("(net::ERR_ABORTED)"),
        ),
      );
      await expectStoryReady();
    }

    const width = await page.locator("body").evaluate((body) => ({
      client: body.clientWidth,
      scroll: body.scrollWidth,
    }));
    expect(width.scroll).toBeLessThanOrEqual(width.client);
    expect(runtimeIssues).toEqual([]);
  });
}
