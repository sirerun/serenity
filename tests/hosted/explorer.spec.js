const { test, expect } = require('@playwright/test');
const { spawn } = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const binary = process.env.SERENITY_EXPLORER_FIXTURE_BINARY;
test.skip(!binary, 'Requires the compiled local non-customer explorer fixture');
let fixture, origin, receiptRoot;

test.beforeAll(async () => {
  receiptRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'serenity-explorer-browser-'));
  const receipt = path.join(receiptRoot, 'receipt.json');
  fixture = spawn(binary, ['-test.run', '^TestExplorerBrowserFixture$', '-test.v', '-test.timeout=10m'], {
    env: { ...process.env, SERENITY_EXPLORER_FIXTURE_RECEIPT: receipt }, stdio: 'ignore',
  });
  await expect.poll(() => {
    if (fixture.exitCode != null) throw new Error('Synthetic fixture exited before readiness');
    try { return JSON.parse(fs.readFileSync(receipt, 'utf8')).url; } catch { return ''; }
  }, { timeout: 30000 }).toMatch(/^http:\/\/127\.0\.0\.1:\d+$/);
  origin = JSON.parse(fs.readFileSync(receipt, 'utf8')).url;
});

test.afterAll(async () => {
  if (origin) await fetch(`${origin}/__fixture/stop`).catch(() => {});
  if (fixture && fixture.exitCode == null) {
    await new Promise(resolve => {
      const timer = setTimeout(() => { fixture.kill('SIGTERM'); resolve(); }, 5000);
      fixture.once('exit', () => { clearTimeout(timer); resolve(); });
    });
  }
  if (receiptRoot) fs.rmSync(receiptRoot, { recursive: true, force: true });
});

test('owned read-only explorer pages, facets, inert text and provenance work with real HTTP', async ({ page }, testInfo) => {
  const requests = [], dialogs = [];
  page.on('request', request => requests.push(request.url()));
  page.on('dialog', async dialog => { dialogs.push(dialog.message()); await dialog.dismiss(); });
  await page.goto(`${origin}/__fixture/owner`);
  await expect(page.getByRole('link', { name: 'Serenity memory explorer' })).toHaveAttribute('href', '/dashboard');
  await expect(page.getByText('Private to you', { exact: true })).toBeVisible();
  await page.getByRole('combobox', { name: 'Memory type' }).selectOption('fact');
  await expect(page.locator('.workspace-count strong')).toHaveText('223');
  await page.getByRole('button', { name: 'List', exact: true }).click();
  await expect(page.locator('.collection-footer')).toContainText('100 matching loaded');
  await page.getByRole('button', { name: 'Load more memories' }).click();
  await expect(page.locator('.collection-footer')).toContainText('200 matching loaded');
  await page.getByRole('button', { name: 'Load more memories' }).click();
  await expect(page.locator('.collection-footer')).toContainText('223 matching loaded');
  const list = page.getByRole('list', { name: 'Loaded memories' });
  const listSize = await list.evaluate(el => ({ height: el.getBoundingClientRect().height, scroll: el.scrollHeight, client: el.clientHeight }));
  expect(listSize.height).toBeLessThanOrEqual(480);
  expect(listSize.scroll).toBeGreaterThan(listSize.client);
  await list.getByRole('button').last().scrollIntoViewIfNeeded();
  await expect(list.getByRole('button').last()).toBeVisible();
  await expect(page.getByRole('button', { name: 'Load more memories' })).toHaveCount(0);
  await page.getByRole('button', { name: /Synthetic browser memory/ }).first().click();
  const note = page.getByRole('complementary', { name: 'Selected memory' });
  await expect(note.getByRole('heading', { name: /Synthetic browser memory/ })).toBeVisible();
  await expect(note.locator('svg[onload]')).toHaveCount(0);
  await expect(note.locator('.source-item')).toHaveCount(1);
  await page.screenshot({ path: testInfo.outputPath('private-explorer-note.png'), fullPage: true });
  await note.locator('.source-item').click();
  await expect(note.locator('.detail-kind')).toContainText('source');
  await page.getByRole('region', { name: 'Browse by year' }).getByRole('button', { name: /^2025(?:\s|$)/ }).click();
  await expect(page.getByRole('heading', { name: 'The 2025 collection' })).toBeVisible();
  await expect(page.locator('.workspace-count strong')).toHaveText('39');
  await page.goBack();
  await expect(page.getByRole('heading', { name: 'A connected library' })).toBeVisible();
  await page.getByRole('combobox', { name: 'Memory type' }).selectOption('source');
  await page.getByRole('button', { name: /Unknown/ }).click();
  await expect(page.locator('.workspace-count strong')).toHaveText('223');
  expect(dialogs).toEqual([]);
  expect(requests.some(url => /\/assets\/explorer\/demo-/.test(url))).toBe(false);
  expect(requests.every(url => new URL(url).origin === origin)).toBe(true);
  expect(await page.evaluate(() => [localStorage.length, sessionStorage.length])).toEqual([0, 0]);
});

test('foreign brains stay fenced and expired sessions clear the rendered account state', async ({ page }) => {
  await page.goto(`${origin}/__fixture/owner`);
  await page.getByRole('combobox', { name: 'Memory type' }).selectOption('fact');
  await page.getByRole('button', { name: 'List', exact: true }).click();
  await expect(page.getByRole('button', { name: /Synthetic browser memory/ }).first()).toBeVisible();
  const foreign = await page.request.get(`${origin}/api/inspector/v1/brains/BrainOtherABCDEFGHIJKLMNOP/graph`);
  expect(foreign.status()).toBe(404);
  await page.request.get(`${origin}/__fixture/expire`);
  await page.getByRole('combobox', { name: 'Memory type' }).selectOption('source');
  await expect(page.getByRole('heading', { name: 'Your session has ended' })).toBeVisible();
  await expect(page.getByText(/Synthetic browser memory/)).toHaveCount(0);
  await expect(page.locator('canvas')).toHaveCount(0);
  await page.goto(`${origin}/__fixture/other`);
  await page.getByRole('button', { name: 'List', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'No matching memories' })).toBeVisible();
  await expect(page.getByText(/Synthetic browser memory/)).toHaveCount(0);
});
