// @ts-check
import { defineConfig } from '@playwright/test';
import path from 'node:path';

const baseURL = 'http://127.0.0.1:8080';
const storageState = path.join(import.meta.dirname, '.auth', 'user.json');

// Specs that are self-contained (no cross-test state dependencies) and safe
// to run on any browser after the Chromium full suite has seeded state.
const crossBrowserSpecs = [
  'specs/smoke.spec.js',
  'specs/settings.spec.js',
  'specs/instance-detail.spec.js',
];

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
    // --- Chromium (primary): full suite ---
    {
      name: 'setup',
      testMatch: 'auth.setup.js',
    },
    {
      name: 'chromium',
      use: { browserName: 'chromium', storageState },
      dependencies: ['setup'],
      testMatch: 'specs/**/*.spec.js',
      testIgnore: ['specs/auth.spec.js', 'specs/accessibility.spec.js'],
    },
    {
      name: 'auth',
      use: { browserName: 'chromium' },
      testMatch: 'specs/auth.spec.js',
    },

    // --- Accessibility: runs after main suite to avoid state interference ---
    {
      name: 'a11y',
      use: { browserName: 'chromium', storageState },
      dependencies: ['chromium'],
      testMatch: 'specs/accessibility.spec.js',
    },

    // --- Firefox: cross-browser subset ---
    {
      name: 'firefox',
      use: { browserName: 'firefox', storageState },
      dependencies: ['chromium'],
      testMatch: crossBrowserSpecs,
    },
    {
      name: 'auth-firefox',
      use: { browserName: 'firefox' },
      dependencies: ['auth'],
      testMatch: 'specs/auth.spec.js',
    },

    // --- WebKit: cross-browser subset ---
    {
      name: 'webkit',
      use: { browserName: 'webkit', storageState },
      dependencies: ['chromium'],
      testMatch: crossBrowserSpecs,
    },
    {
      name: 'auth-webkit',
      use: { browserName: 'webkit' },
      dependencies: ['auth'],
      testMatch: 'specs/auth.spec.js',
    },
  ],
  outputDir: './results',
  globalSetup: './global-setup.js',
  globalTeardown: './global-teardown.js',
});
