const { test, expect } = require('@playwright/test');
const { spawn, execFileSync } = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const http = require('node:http');

let service, provider, callbackServer, directory, origin, log = '';
test.afterEach(async () => { if (callbackServer) { await close(callbackServer); callbackServer = null; } });
const listen = server => new Promise(resolve => server.listen(0, '127.0.0.1', () => resolve(server.address().port)));
const close = server => new Promise(resolve => server.close(resolve));

test.beforeAll(async () => {
  const binary = process.env.SERENITY_TEST_BINARY;
  if (!binary || !fs.existsSync(binary)) throw new Error('Build Serenity and set SERENITY_TEST_BINARY; this test must not skip missing service setup.');
  directory = fs.mkdtempSync(path.join(os.tmpdir(), 'serenity-hosted-browser-'));
  fs.mkdirSync(path.join(directory, 'secrets'), { mode: 0o700 });
  fs.writeFileSync(path.join(directory, 'secrets', 'EMBEDDINGS_API_KEY'), 'local-test-only', { mode: 0o600 });
  // Explicit test adapter: exercises HTTP plumbing, never semantic quality.
  provider = http.createServer((request, response) => {
    request.resume();
    request.on('end', () => {
      response.writeHead(200, { 'Content-Type': 'application/json' });
      response.end(JSON.stringify({ data: [{ embedding: [1, 2, 3] }], model: 'test' }));
    });
  });
  const providerPort = await listen(provider);
  const reservation = http.createServer();
  const servicePort = await listen(reservation);
  await close(reservation);
  origin = `http://127.0.0.1:${servicePort}`;
  const config = { bind: `127.0.0.1:${servicePort}`, data_dir: path.join(directory, 'data'), secrets_dir: path.join(directory, 'secrets'), public_origin: origin, embedding_model: 'test', embedding_version: 'v1', embedding_base_url: `http://127.0.0.1:${providerPort}`, max_open: 2, max_in_flight: 4, account_cap: 10 };
  fs.writeFileSync(path.join(directory, 'config.json'), JSON.stringify(config));
  service = spawn(binary, ['hosted', 'serve', '--config', path.join(directory, 'config.json')], { env: { ...process.env, SERENITY_HOSTED_DEV: '1' }, stdio: ['ignore', 'pipe', 'pipe'] });
  service.stdout.on('data', data => { log += data; });
  service.stderr.on('data', data => { log += data; });
  await expect.poll(async () => {
    if (service.exitCode !== null) throw new Error('Hosted test service exited before readiness');
    try { return (await fetch(`${origin}/readyz`)).status; } catch { return 0; }
  }, { timeout: 15000 }).toBe(200);
});

test.afterAll(async () => {
  if (service && service.exitCode === null) {
    await new Promise(resolve => {
      const timer = setTimeout(() => service.kill('SIGKILL'), 10000);
      service.once('exit', () => { clearTimeout(timer); resolve(); });
      service.kill('SIGTERM');
    });
  }
  if (provider) await close(provider);
  if (directory) fs.rmSync(directory, { recursive: true, force: true });
});

test('signup, one-time credential, save, export, revoke, logout and expired link', async ({ page }, testInfo) => {
  await page.addInitScript(() => {
    window.cspViolations = [];
    document.addEventListener('securitypolicyviolation', event => window.cspViolations.push(event.violatedDirective));
  });
  for (const route of ['/docs/', '/chat/', '/get-started/', '/product/', '/pricing/', '/docs/connections/', '/docs/connections/claude-code/', '/docs/connections/codex/', '/docs/connections/other-mcp/', '/docs/connections/rakazo/', '/docs/connections/claude-web/', '/docs/connections/chatgpt/', '/']) {
    const response = await page.goto(origin + route);
    expect(response.status(), route).toBe(200);
    await page.evaluate(() => document.fonts.ready);
    expect(await page.evaluate(() => window.cspViolations)).toEqual([]);
  }
  await expect(page.locator('.dot-svg').first()).toBeAttached();
  await page.getByRole('link', { name: 'Sign in', exact: true }).click();
  await expect(page).toHaveURL(`${origin}/login`);
  await expect(page.locator('.brand img')).toBeVisible();
  expect(await page.locator('body').evaluate(el => getComputedStyle(el).margin)).toBe('0px');
  await page.screenshot({ path: testInfo.outputPath('login.png'), fullPage: true });
  await page.getByLabel('Email address').fill(`${testInfo.project.name}@example.test`);
  await page.getByRole('button', { name: 'Send sign-in link' }).click();
  await expect(page.getByRole('heading', { name: 'Check your inbox' })).toBeVisible();
  await expect.poll(() => /Development login: (http:\/\/\S+)/.test(log)).toBe(true);
  const link = log.match(/Development login: (http:\/\/\S+)/)[1];
  await page.goto(link);
  await expect(page.getByRole('heading', { name: 'Your memory, at a glance' })).toBeVisible();
  await expect(page).toHaveURL(`${origin}/dashboard`);
  await expect(page.getByRole('link', { name: 'Overview', exact: true })).toHaveAttribute('aria-current', 'page');
  await expect(page.getByRole('button', { name: 'Delete my account', exact: true })).toHaveCount(0);
  await page.screenshot({ path: testInfo.outputPath('dashboard.png'), fullPage: true });
  let frame;
  for (const route of ['/dashboard', '/dashboard/connections', '/dashboard/memories', '/dashboard/usage', '/dashboard/settings', '/login', '/docs/', '/pricing/', '/oauth/connections']) {
    await page.goto(origin + route);
    const geometry = await page.locator('main').evaluate(el => {
      const r = el.getBoundingClientRect();
      return { x: r.x, width: r.width };
    });
    if (!frame) frame = geometry;
    expect(geometry, route).toEqual(frame);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), route).toBe(true);
  }
  await page.goto(origin + '/dashboard/connections');
  await expect(page.getByRole('heading', { name: 'Connect Claude on the web' })).toBeVisible();
  await page.getByText('Manual connection tokens', {exact:true}).click();
  await page.getByRole('button', { name: 'Create connection token' }).click();
  const token = await page.getByLabel('Bearer token — shown once').inputValue();
  expect(token).toMatch(/^sk_live_[a-f0-9]{8}_[A-Za-z0-9_-]{43}$/);
  await page.getByRole('button', { name: 'Copy token', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Copied', exact: true })).toBeVisible();
  await page.goto(`${origin}/dashboard`);
  await expect(page.getByLabel('Bearer token — shown once')).toHaveCount(0);
  await page.goto(`${origin}/dashboard/memories`);
  const marker = 'My browser verification marker is amber heron.';
  await page.getByLabel('What should your agent remember?').fill(marker);
  await page.getByRole('button', { name: 'Save memory', exact: true }).click();
  await expect(page).toHaveURL(`${origin}/dashboard/memories`);
  await page.goto(`${origin}/dashboard/usage`);
  for (const label of ['Writes', 'Recalls', 'Input tokens', 'Brains', 'Live memories', 'Storage (bytes, including history)']) {
    await expect(page.getByRole('rowheader', { name: label, exact: true })).toBeVisible();
  }
  await expect(page.getByRole('columnheader', { name: 'Remaining' })).toBeVisible();
  const sizes = await page.evaluate(() => ({ width: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
  expect(sizes.scroll).toBeLessThanOrEqual(sizes.width + 1);
  await page.goto(`${origin}/dashboard/memories`);
  const downloadPromise = page.waitForEvent('download');
  await page.getByRole('link', { name: 'Export memory' }).click();
  const download = await downloadPromise;
  const downloaded = await download.path();
  const facts = execFileSync('python3', ['-c', 'import zipfile,sys; z=zipfile.ZipFile(sys.argv[1]); assert "brain.bundle" in z.namelist(); print(z.read("facts.json").decode())', downloaded], { encoding: 'utf8' });
  expect(facts).toContain(marker);
  await page.goto(`${origin}/dashboard/settings`);
  await page.getByText('Replace or revoke project credentials', {exact:true}).click();
  await page.getByRole('button', { name: 'Revoke access', exact: true }).click();
  const denied = await fetch(`${origin}/mcp`, { method: 'POST', headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' }, body: '{}' });
  expect(denied.status).toBe(401);
  await page.getByRole('button', { name: 'Sign out', exact: true }).click();
  await expect(page.getByLabel('Email address')).toBeVisible();
  await page.goto(link);
  await expect(page.getByRole('heading', { name: 'This link is no longer available' })).toBeVisible();
});

test('OAuth login resumes consent and browser follows only the approved callback', async ({ page }, testInfo) => {
  const crypto = require('node:crypto');
  const verifier = 'v'.repeat(43);
  const challenge = crypto.createHash('sha256').update(verifier).digest('base64url');
  let callbackURL = '';
  callbackServer = http.createServer((request, response) => {
    callbackURL = `http://${request.headers.host}${request.url}`;
    response.writeHead(200, { 'Content-Type': 'text/html' });
    response.end('<h1>Agent connected</h1>');
  });
  const callback = `http://127.0.0.1:${await listen(callbackServer)}/callback`;
  const registration = await page.request.post(origin + '/oauth/register', { data: { client_name: 'Synthetic browser agent', redirect_uris: [callback], grant_types: ['authorization_code', 'refresh_token'], token_endpoint_auth_method: 'none' } });
  expect(registration.status()).toBe(201);
  const client = await registration.json();
  const query = new URLSearchParams({ client_id: client.client_id, redirect_uri: callback, response_type: 'code', state: 'browser-state', code_challenge_method: 'S256', code_challenge: challenge, resource: origin + '/mcp', scope: 'memory:read memory:write' });
  await page.goto(origin + '/oauth/authorize?' + query);
  await expect(page.getByLabel('Email address')).toBeVisible();
  const before = log.length;
  await page.getByLabel('Email address').fill(`oauth-${testInfo.project.name}@example.test`);
  await page.getByRole('button', { name: 'Send sign-in link' }).click();
  await expect.poll(() => /Development login: (http:\/\/\S+)/.test(log.slice(before))).toBe(true);
  await page.goto(log.slice(before).match(/Development login: (http:\/\/\S+)/)[1]);
  await expect(page.getByRole('heading', { name: 'Connect your agent.', exact: true })).toBeVisible();
  await expect(page.getByLabel('Read memories only')).toBeChecked();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('oauth-consent.png'), fullPage: true });
  await page.getByRole('button', { name: 'Connect agent', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Agent connected' })).toBeVisible();
  const returned = new URL(callbackURL);
  expect(returned.searchParams.get('state')).toBe('browser-state');
  expect(returned.searchParams.get('code')).toBeTruthy();
  const exchange = await page.request.post(origin + '/oauth/token', { form: { grant_type: 'authorization_code', client_id: client.client_id, redirect_uri: callback, code: returned.searchParams.get('code'), code_verifier: verifier, resource: origin + '/mcp' } });
  expect(exchange.status()).toBe(200);
  const tokens = await exchange.json();
  expect(tokens.scope).toBe('memory:read');
  expect(tokens.refresh_token).toBeTruthy();
  await page.goto(origin + '/oauth/connections');
  await expect(page.getByRole('heading', { name: 'Synthetic browser agent' })).toBeVisible();
  await page.getByRole('button', { name: 'Disconnect agent' }).click();
  await expect(page.getByText('No active OAuth connections.', { exact: false })).toBeVisible();
});
