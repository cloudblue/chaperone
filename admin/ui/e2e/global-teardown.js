import fs from 'node:fs';

export default async function globalTeardown() {
  // Kill admin server
  const adminPid = process.env.E2E_ADMIN_PID;
  if (adminPid) {
    try {
      process.kill(Number(adminPid), 'SIGTERM');
    } catch {
      // Process may have already exited
    }
  }

  // Kill mock chaperone fleet
  const mockPid = process.env.E2E_MOCK_PID;
  if (mockPid) {
    try {
      process.kill(Number(mockPid), 'SIGTERM');
    } catch {
      // Process may have already exited
    }
  }

  // Remove temp directory
  const tmpDir = process.env.E2E_TMP_DIR;
  if (tmpDir) {
    try {
      fs.rmSync(tmpDir, { recursive: true, force: true });
    } catch {
      // Best-effort cleanup
    }
  }
}
