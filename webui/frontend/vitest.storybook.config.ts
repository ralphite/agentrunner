import path from "node:path";
import { fileURLToPath } from "node:url";
import { storybookTest } from "@storybook/addon-vitest/vitest-plugin";
import { playwright } from "@vitest/browser-playwright";
import { defineConfig } from "vitest/config";

const dirname = path.dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  // Browser mode has no HTML entry for Vite to crawl. Scan Story files up
  // front so adding a page-level Story does not trigger a mid-test dependency
  // optimization reload (which discards the dynamically imported test module).
  optimizeDeps: {
    entries: ["src/**/*.stories.{ts,tsx}"],
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
