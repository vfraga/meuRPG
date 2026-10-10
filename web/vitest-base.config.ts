import { fileURLToPath } from 'node:url';

import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    // Loaded by Vite, not bundled by Angular, so Vitest runs it again for every spec file (see src/test-hooks.ts).
    setupFiles: [fileURLToPath(new URL('./src/test-hooks.ts', import.meta.url))],
  },
});
