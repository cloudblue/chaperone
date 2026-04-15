import http from 'node:http';

/**
 * Wait for an HTTP endpoint to return 200.
 * @param {string} url
 * @param {number} timeoutMs
 */
export function waitForHealth(url, timeoutMs = 15_000) {
  const start = Date.now();
  return new Promise((resolve, reject) => {
    function attempt() {
      if (Date.now() - start > timeoutMs) {
        reject(new Error(`Timed out waiting for ${url}`));
        return;
      }
      const req = http.get(url, (res) => {
        if (res.statusCode === 200) {
          res.resume();
          resolve();
        } else {
          res.resume();
          setTimeout(attempt, 250);
        }
      });
      req.on('error', () => setTimeout(attempt, 250));
      req.setTimeout(2000, () => {
        req.destroy();
        setTimeout(attempt, 250);
      });
    }
    attempt();
  });
}
