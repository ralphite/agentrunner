import { expect, test } from "@playwright/test";

test("Core Session Playback supports manual control, autoplay, and replay", async ({
  page,
}) => {
  const runtimeErrors: string[] = [];
  page.on("console", (message) => {
    if (
      message.type() === "error" &&
      !message.text().startsWith("Failed to load resource:")
    ) {
      runtimeErrors.push(message.text());
    }
  });
  page.on("pageerror", (error) => runtimeErrors.push(error.message));
  page.on("response", (response) => {
    if (response.status() < 400) return;
    const url = new URL(response.url());
    runtimeErrors.push(`HTTP ${response.status()} ${url.pathname}${url.search}`);
  });

  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(
    "/iframe.html?id=demos-core-session-playback--demo&viewMode=story",
    { waitUntil: "networkidle" },
  );

  const controls = page.getByRole("region", {
    name: "Core session playback",
  });
  const status = controls.getByRole("status");
  const play = controls.getByRole("button", { name: "Play", exact: true });
  await expect(play).toBeEnabled();
  await controls
    .getByRole("combobox", { name: "Playback speed" })
    .selectOption("2");

  await play.click();
  await expect(status.locator("b")).toHaveText("running");
  await controls.getByRole("button", { name: "Pause" }).click();
  await expect(status.locator("b")).toHaveText("paused");

  await controls.getByRole("button", { name: "Next" }).click();
  await expect(status).toContainText("Step 2 / 8");
  await controls.getByRole("button", { name: "Reset" }).click();
  await expect(status.locator("b")).toHaveText("idle");
  await expect(status).toContainText("Step 1 / 8");

  await controls.getByRole("checkbox", { name: "Autoplay" }).check();
  await expect(status.locator("b")).toHaveText("completed", {
    timeout: 20_000,
  });
  await expect(status).toContainText("Step 8 / 8");
  await expect(page.locator(".diffwrap")).toBeVisible();

  await controls.getByRole("button", { name: "Replay" }).click();
  await expect(status.locator("b")).toHaveText("running");
  await expect(status.locator("b")).toHaveText("completed", {
    timeout: 20_000,
  });
  await expect(status).toContainText("Step 8 / 8");
  await expect(runtimeErrors).toEqual([]);
});
