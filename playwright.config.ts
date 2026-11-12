import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './tests/e2e',
  // The scene renders through SwiftShader in CI; the showroom IBL and the reflective floor
  // make each frame expensive, so workers are serialised and the budget is generous.
  fullyParallel: false,
  workers: 1,
  timeout: 90_000,
  use: {
    baseURL: 'http://127.0.0.1:4173',
    viewport: { width: 1366, height: 768 },
    launchOptions: { args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] },
  },
  webServer: {
    command: 'npm run dev -- --host 127.0.0.1 --port 4173',
    url: 'http://127.0.0.1:4173',
    reuseExistingServer: !process.env.CI,
    timeout: 120000,
  },
})
