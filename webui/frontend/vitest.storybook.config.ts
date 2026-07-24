import path from "node:path";
import { fileURLToPath } from "node:url";
import { storybookTest } from "@storybook/addon-vitest/vitest-plugin";
import { playwright } from "@vitest/browser-playwright";
import { defineConfig } from "vitest/config";

const dirname = path.dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  // A Storybook browser run shares one Vite server across all Story files.
  // Automatic dependency discovery can therefore reload an unrelated canvas
  // while its play() function is mid-interaction. Serve dependencies without
  // the dev optimizer so the tester document stays mounted for the whole run.
  optimizeDeps: {
    noDiscovery: true,
  },
  plugins: [
    storybookTest({
      configDir: path.join(dirname, ".storybook"),
    }),
  ],
  test: {
    name: "storybook",
    dir: path.join(dirname, ".storybook"),
    browser: {
      enabled: true,
      provider: playwright(),
      headless: true,
      instances: [{ browser: "chromium" }],
    },
    testTimeout: 90_000,
  },
});
