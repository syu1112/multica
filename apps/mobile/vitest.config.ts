import { dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vitest/config";

const mobileRoot = dirname(fileURLToPath(import.meta.url));

// Mobile vitest is intentionally minimal — Node environment only, scoped to
// pure-function tests in `lib/` and data serializers. We don't ship jsdom or RN test renderers
// here because the app runs on Hermes / native shims and any DOM-shaped
// runner would be a lie. Tests that need RN component rendering would
// need a separate jest+react-native-testing-library track; for now we
// keep this lane for helpers and serializers only.
//
// Co-located test files (foo.ts + foo.test.ts) match how the rest of the
// monorepo organises vitest suites.
export default defineConfig({
  resolve: {
    alias: {
      "@": mobileRoot,
    },
  },
  test: {
    environment: "node",
    globals: true,
    include: ["lib/**/*.test.ts", "data/**/*.test.ts"],
    passWithNoTests: true,
  },
});
