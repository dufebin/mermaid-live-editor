import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  forbidOnly: !!process.env.CI,
  fullyParallel: true,
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] }
    }
  ],
  reporter: process.env.CI ? [['github'], ['list']] : 'list',
  retries: process.env.CI ? 2 : 0,
  testDir: './tests',
  // auth.spec.ts registers fresh users with no login fallback, so it only runs
  // against the throwaway backend of `pnpm test:e2e:auth` (which wipes its
  // DATA_DIR per run) — not against whatever backend this suite happens to use.
  testIgnore: 'auth.spec.ts',
  use: {
    baseURL: 'http://localhost:3000',
    browserName: 'chromium',
    permissions: ['clipboard-read', 'clipboard-write'],
    trace: 'retain-on-failure',
    viewport: { width: 1920, height: 1080 }
  },
  webServer: {
    command: `pnpm ${process.env.CI ? 'preview' : 'dev'}`,
    url: 'http://localhost:3000',
    reuseExistingServer: !process.env.CI
  },
  workers: process.env.CI ? 3 : undefined
});
