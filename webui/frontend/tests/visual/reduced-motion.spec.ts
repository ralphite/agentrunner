import { expect, test } from "@playwright/test";

test("Spinner honors the reduced-motion preference", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto(
    "/iframe.html?id=foundations-feedback-status-and-loading--spinner-reduced-motion&viewMode=story",
    { waitUntil: "networkidle" },
  );

  const spinner = page.getByRole("status", {
    name: "Motion stops; loading remains announced",
  });
  await expect(spinner).toBeVisible();
  await expect
    .poll(() =>
      spinner.locator("svg").evaluate((node) => {
        const style = getComputedStyle(node);
        return {
          animationDuration: style.animationDuration,
          animationName: style.animationName,
          reducedMotion: matchMedia("(prefers-reduced-motion: reduce)").matches,
        };
      }),
    )
    .toEqual({
      animationDuration: "0s",
      animationName: "none",
      reducedMotion: true,
    });
});
