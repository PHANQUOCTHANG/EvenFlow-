import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import path from "node:path";

// Unit + component test cho apps/web.
// E2E (Playwright) nam o tests/e2e/ tai goc repo, KHONG chay qua vitest.
export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./vitest.setup.ts"],
    include: ["src/**/*.{test,spec}.{ts,tsx}"],
    coverage: {
      provider: "v8",
      reporter: ["text", "html", "lcov"],
      include: ["src/**/*.{ts,tsx}"],
      exclude: ["src/**/*.d.ts", "src/app/layout.tsx"],
      // Cung nguong voi backend (issue 1006): duoi 70% thi CI chan PR.
      thresholds: { lines: 70, functions: 70, branches: 60, statements: 70 },
    },
  },
  resolve: {
    alias: { "@": path.resolve(__dirname, "./src") },
  },
});
