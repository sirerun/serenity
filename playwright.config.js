const { defineConfig } = require('@playwright/test');
module.exports = defineConfig({
  testDir: './tests/site',
  workers: 2,
  retries: 0,
  timeout: 30000,
  reporter: [['list'], ['json', { outputFile: 'test-results/site-results.json' }]],
  use: { baseURL: 'http://127.0.0.1:8937', reducedMotion: 'reduce', trace: 'retain-on-failure', screenshot: 'only-on-failure' },
  projects: [
    { name: 'mobile', use: { viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true } },
    { name: 'laptop', use: { viewport: { width: 1024, height: 768 } } },
    { name: 'wide', use: { viewport: { width: 1440, height: 1000 } } },
    { name: 'ultrawide', use: { viewport: { width: 2880, height: 1800 } } },
  ],
  webServer: { command: 'python3 -m http.server 8937 --bind 127.0.0.1 --directory site', url: 'http://127.0.0.1:8937', reuseExistingServer: false, stdout: 'ignore', stderr: 'ignore' },
});
