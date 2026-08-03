import { defineConfig } from "vitest/config";

// Covers plain-TS unit tests (utils/) only — no Nuxt/Vue component testing
// setup here, keep that scope for a future @nuxt/test-utils addition if
// component-level tests become necessary.
export default defineConfig({
  test: {
    include: ["**/*.test.ts"],
    environment: "node",
  },
});
