const { defineConfig } = require('@playwright/test');
module.exports = defineConfig({
  testDir: './tests/hosted',
  workers: 1,
  retries: 0,
  timeout: 60000,
  reporter: [['list'], ['json', { outputFile: 'test-results/hosted-results.json' }]],
  use: { reducedMotion: 'reduce', trace: 'retain-on-failure', screenshot: 'only-on-failure', permissions: ['clipboard-read', 'clipboard-write'] },
  projects: [
    { name: 'hosted-mobile', use: { viewport: { width: 400, height: 850 } } },
    { name: 'hosted-desktop', use: { viewport: { width: 1280, height: 900 } } },
  ],
});
