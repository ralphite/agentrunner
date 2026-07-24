import path from "node:path";
import { fileURLToPath } from "node:url";
import { storybookTest } from "@storybook/addon-vitest/vitest-plugin";
import { playwright } from "@vitest/browser-playwright";
import { defineConfig } from "vitest/config";

const dirname = path.dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  // A Storybook browser run shares one Vite server across all Story files.
  // Automatic dependency discovery can therefore reload an unrelated canvas
  // while its play() function is mid-interaction. Disable discovery so the
  // tester document stays mounted. The fixed allowlist covers CJS interop and
  // keeps a cold checkout fast without allowing runtime discovery reloads.
  optimizeDeps: {
    noDiscovery: true,
    include: [
      "@phosphor-icons/react",
      "aria-query",
      "highlight.js/lib/languages/bash",
      "highlight.js/lib/languages/c",
      "highlight.js/lib/languages/cpp",
      "highlight.js/lib/languages/csharp",
      "highlight.js/lib/languages/css",
      "highlight.js/lib/languages/diff",
      "highlight.js/lib/languages/dockerfile",
      "highlight.js/lib/languages/go",
      "highlight.js/lib/languages/ini",
      "highlight.js/lib/languages/java",
      "highlight.js/lib/languages/javascript",
      "highlight.js/lib/languages/json",
      "highlight.js/lib/languages/markdown",
      "highlight.js/lib/languages/python",
      "highlight.js/lib/languages/rust",
      "highlight.js/lib/languages/sql",
      "highlight.js/lib/languages/typescript",
      "highlight.js/lib/languages/xml",
      "highlight.js/lib/languages/yaml",
      "lowlight",
      "lz-string",
      "mermaid",
      "msw-storybook-addon",
      "pretty-format",
      "react",
      "react-dom",
      "react-dom/client",
      "react-dom/test-utils",
      "react-is",
      "react-markdown",
      "react/jsx-dev-runtime",
      "react/jsx-runtime",
      "rehype-katex",
      "remark-gfm",
      "remark-math",
      "storybook/internal/preview/runtime",
      "storybook/preview-api",
      "storybook/test",
      "unist-util-visit",
      "zustand",
      "zustand/vanilla",
    ],
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
