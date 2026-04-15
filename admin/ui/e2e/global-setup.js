import { spawn, execSync, execFileSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { waitForHealth } from './helpers/services.js';
import { TEST_USER, PW_CHANGE_USER, TEST_PASSWORD } from './helpers/constants.js';

const ROOT = path.resolve(import.meta.dirname, '..', '..', '..');

export default async function globalSetup() {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'chaperone-e2e-'));
  const dbPath = path.join(tmpDir, 'test.db');
  const binDir = path.join(ROOT, 'bin');
  const authDir = path.join(import.meta.dirname, '.auth');

  fs.mkdirSync(authDir, { recursive: true });

  // Store paths for teardown
  process.env.E2E_TMP_DIR = tmpDir;
  process.env.E2E_DB_PATH = dbPath;

  // 1. Build admin binary + seed-user
  console.log('[e2e] Building admin binary...');
  execSync('make build-admin', { cwd: ROOT, stdio: 'pipe' });
  console.log('[e2e] Building seed-user...');
  execSync(
    `cd admin && go build -o ../bin/seed-user ./cmd/seed-user`,
    { cwd: ROOT, stdio: 'pipe' },
  );

  // 2. Start mock chaperone fleet
  console.log('[e2e] Starting mock chaperone fleet...');
  const mockProc = spawn(
    'node',
    [path.join(ROOT, 'test', 'mock-chaperone', 'mock-chaperone.js')],
    { stdio: 'pipe' },
  );
  process.env.E2E_MOCK_PID = String(mockProc.pid);
  mockProc.on('error', (err) => console.error('[e2e] mock-chaperone error:', err));

  await waitForHealth('http://127.0.0.1:19091/_ops/health', 15_000);
  console.log('[e2e] Mock fleet ready');

  // 3. Seed test users
  console.log('[e2e] Seeding test users...');
  const seedBin = path.join(binDir, 'seed-user');
  execFileSync(seedBin, ['--db', dbPath, '--username', TEST_USER, '--password', TEST_PASSWORD], {
    cwd: ROOT,
    stdio: 'pipe',
  });
  execFileSync(seedBin, ['--db', dbPath, '--username', PW_CHANGE_USER, '--password', TEST_PASSWORD], {
    cwd: ROOT,
    stdio: 'pipe',
  });

  // 4. Start admin server
  console.log('[e2e] Starting admin server...');
  const adminProc = spawn(
    path.join(binDir, 'chaperone-admin'),
    [],
    {
      stdio: 'pipe',
      env: {
        ...process.env,
        CHAPERONE_ADMIN_SERVER_ADDR: '127.0.0.1:8080',
        CHAPERONE_ADMIN_DATABASE_PATH: dbPath,
        CHAPERONE_ADMIN_SERVER_SECURE_COOKIES: 'false',
        CHAPERONE_ADMIN_SCRAPER_INTERVAL: '3s',
        CHAPERONE_ADMIN_SCRAPER_TIMEOUT: '2s',
        CHAPERONE_ADMIN_LOG_LEVEL: 'warn',
      },
    },
  );
  process.env.E2E_ADMIN_PID = String(adminProc.pid);
  adminProc.on('error', (err) => console.error('[e2e] admin server error:', err));

  await waitForHealth('http://127.0.0.1:8080/api/health', 15_000);
  console.log('[e2e] Admin server ready');
}
