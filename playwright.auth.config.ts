import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  fullyParallel: false,
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  reporter: 'list',
  testDir: './tests',
  testMatch: 'auth.spec.ts',
  use: {
    baseURL: 'http://localhost:3100',
    browserName: 'chromium',
    permissions: ['clipboard-read', 'clipboard-write'],
    trace: 'retain-on-failure',
    viewport: { width: 1920, height: 1080 }
  },
  webServer: [
    {
      command:
        'rm -rf server-data/e2e && make -C server-go build && DATA_DIR=server-data/e2e PORT=18799 ./server-go/bin/mermaid-storage',
      url: 'http://127.0.0.1:18799/api/folders',
      reuseExistingServer: false
    },
    {
      command: 'API_PROXY_TARGET=http://127.0.0.1:18799 pnpm dev --port 3100 --strictPort',
      url: 'http://localhost:3100',
      reuseExistingServer: false
    }
  ],
  workers: 1
});
