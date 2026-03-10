import { defineConfig, devices } from "@playwright/test";

const isCI = !!process.env.CI;

export default defineConfig({
  testDir: "./e2e",
  timeout: 30_000,
  fullyParallel: false,
  forbidOnly: isCI,
  retries: isCI ? 2 : 0,
  workers: 1,
  reporter: [["list"], ["html", { open: "never" }]],
  use: {
    baseURL: "http://127.0.0.1:5173",
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
    locale: "zh-CN",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
  webServer: [
    {
      command: "go run ../cmd/e2e-server",
      url: "http://127.0.0.1:18080/healthz",
      timeout: 120_000,
      reuseExistingServer: !isCI,
      env: {
        HTTP_ADDR: "127.0.0.1:18080",
        RUN_MONITOR_WORKER: "true",
        RUN_ALERT_WORKER: "false",
      },
    },
    {
      command: "npm run dev -- --host 127.0.0.1 --port 5173",
      url: "http://127.0.0.1:5173/login",
      timeout: 120_000,
      reuseExistingServer: !isCI,
      env: {
        VITE_API_BASE: "http://127.0.0.1:18080",
        VITE_LOCK_API_BASE: "always",
      },
    },
  ],
});
