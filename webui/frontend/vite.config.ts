import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Build → dist/ (embedded by the Go binary). base './' keeps asset URLs
// relative so the SPA works no matter where it's mounted.
export default defineConfig({
  plugins: [react()],
  base: "./",
  build: { outDir: "dist", emptyOutDir: true },
  server: {
    port: 5188,
    proxy: {
      "/api": { target: "http://127.0.0.1:8788", changeOrigin: true, ws: false },
    },
  },
});
