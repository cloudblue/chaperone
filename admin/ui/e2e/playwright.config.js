// @ts-check
import { defineConfig } from '@playwright/test';
import path from 'node:path';

const baseURL = 'http://127.0.0.1:8080';

export default defineConfig({
  testDir: '.',
  testMatch: ['specs/**/*.spec.js', 'auth.setup.js'],
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 30_000,
  expect: {
    timeout: 10_000,
  },
  use: {
    baseURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    testIdAttribute: 'data-testid',
  },
  projects: [
    {
      name: 'setup',
      testMatch: 'auth.setup.js',
    },
    {
      name: 'chromium',
      use: {
        browserName: 'chromium',
        storageState: path.join(import.meta.dirname, '.auth', 'user.json'),
      },
      dependencies: ['setup'],
      testMatch: 'specs/**/*.spec.js',
      testIgnore: 'specs/auth.spec.js',
    },
    {
      name: 'auth',
      use: {
        browserName: 'chromium',
      },
      testMatch: 'specs/auth.spec.js',
    },
  ],
  outputDir: './results',
  globalSetup: './global-setup.js',
  globalTeardown: './global-teardown.js',
});
