const { test, expect } = require('@playwright/test');
const endpoint = /https:\/\/[a-z0-9]+\.lambda-url\.[a-z0-9-]+\.on\.aws\//;
const answer = { answer: 'Install from [the guide](https://serenity.sire.run/get-started/#source).', mode: 'answer', citations: [{ title: 'Install', url: 'https://serenity.sire.run/get-started/#source' }] };

async function observe(page) {
  const events = [];
  await page.exposeFunction('captureAdoption', event => events.push(event));
  await page.addInitScript(() => window.addEventListener('serenity:adoption', event => window.captureAdoption(event.detail)));
  return events;
}
async function stub(page, data = answer, status = 200) {
  await page.route(endpoint, route => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(data) }));
}
async function noOverflow(page) {
  const size = await page.evaluate(() => ({ width: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
  expect(size.scroll).toBeLessThanOrEqual(size.width + 1);
}

test('landing install CTA and docs navigation emit only fixed events', async ({ page }) => {
  const events = await observe(page);
  await page.goto('/?private_query=NEVER-IN-EVENT');
  await page.getByRole('navigation', { name: 'Main navigation' }).getByRole('link', { name: /^Install/ }).click();
  await expect(page).toHaveURL(/\/get-started\/$/);
  await expect(page.getByRole('heading', { name: /Choose your install/ })).toBeVisible();
  await expect.poll(() => events.map(x => x.name)).toEqual(['install_cta', 'docs_open']);
  await page.getByRole('navigation', { name: 'Main navigation' }).getByRole('link', { name: 'Docs', exact: true }).click();
  await expect(page).toHaveURL(/\/docs\/$/);
  await expect.poll(() => events.length).toBe(3);
  expect(events).toEqual([
    { version: 1, name: 'install_cta', page: 'home' },
    { version: 1, name: 'docs_open', page: 'install' },
    { version: 1, name: 'docs_open', page: 'docs' },
  ]);
  await noOverflow(page);
});

test('chat keyboard happy path emits started and answered without prompt content', async ({ page }) => {
  const events = await observe(page);
  await stub(page);
  await page.goto('/chat/');
  const input = page.getByRole('textbox', { name: 'Your question about Serenity' });
  await input.fill('install PRIVATE-PROMPT-MARKER');
  await input.press('Enter');
  await expect(page.locator('#thread').getByRole('link', { name: 'the guide' })).toBeVisible();
  await expect(input).toBeFocused();
  await expect(page.locator('#askForm')).not.toHaveAttribute('aria-busy', 'true');
  await expect.poll(() => events.length).toBe(2);
  expect(events).toEqual([{ version: 1, name: 'chat_started', page: 'chat' }, { version: 1, name: 'chat_answered', page: 'chat', outcome: 'answer' }]);
  expect(JSON.stringify(events)).not.toContain('PRIVATE');
  expect(await page.evaluate(() => ({ local: localStorage.length, session: sessionStorage.length, cookie: document.cookie }))).toEqual({ local: 0, session: 0, cookie: '' });
  await noOverflow(page);
});

test('chat rate error offers retry and records recovery', async ({ page }) => {
  const events = await observe(page);
  let requests = 0;
  await page.route(endpoint, route => { requests++; return route.fulfill({ status: requests === 1 ? 429 : 200, contentType: 'application/json', body: JSON.stringify(requests === 1 ? { error: 'not reflected' } : answer) }); });
  await page.goto('/chat/');
  await page.getByRole('textbox', { name: 'Your question about Serenity' }).fill('How do I install?');
  await page.getByRole('button', { name: 'Send question' }).click();
  await expect(page.getByRole('button', { name: 'Try again' })).toBeVisible();
  await expect(page.locator('#thread')).toContainText('limit');
  await page.getByRole('button', { name: 'Try again' }).click();
  await expect(page.locator('#thread').getByRole('link', { name: 'the guide' })).toBeVisible();
  await expect.poll(() => events.length).toBe(4);
  expect(events.map(x => [x.name, x.outcome])).toEqual([['chat_started', undefined], ['chat_failed', 'rate_limit'], ['chat_started', undefined], ['chat_answered', 'answer']]);
  await expect(page.locator('#thread .user')).toHaveCount(1);
});

test('documentation fallback is labeled and distinct from a generated answer', async ({ page }) => {
  const events = await observe(page);
  await stub(page, { ...answer, mode: 'search' });
  await page.goto('/chat/');
  await page.getByRole('button', { name: /Can I use it without AI/ }).click();
  await expect(page.locator('.mode-note')).toContainText('Documentation search');
  await expect.poll(() => events.find(x => x.name === 'chat_answered')?.outcome).toBe('search');
});

test('network failure and malformed response retain a recovery action', async ({ page }) => {
  const events = await observe(page);
  await page.route(endpoint, route => route.abort('failed'));
  await page.goto('/chat/');
  await page.getByRole('button', { name: /Can I use it without AI/ }).click();
  await expect(page.getByRole('button', { name: 'Try again' })).toBeVisible();
  await expect.poll(() => events.find(x => x.name === 'chat_failed')?.outcome).toBe('network');
  await page.unroute(endpoint);
  await stub(page, { answer: '' });
  await page.getByRole('button', { name: 'Try again' }).click();
  await expect(page.locator('#thread')).toContainText('empty answer');
  await expect.poll(() => events.filter(x => x.name === 'chat_failed').at(-1)?.outcome).toBe('invalid_response');
});

test('event API drops arbitrary names and payloads and sends no analytics requests', async ({ page }) => {
  const events = await observe(page);
  const outgoing = [];
  page.on('request', request => { if (request.method() !== 'GET') outgoing.push(request.url()); });
  await page.goto('/chat/?secret=NEVER-COLLECT#PRIVATE');
  await page.evaluate(() => {
    serenityAdoption.emit('PRIVATE-NAME', 'PRIVATE-TEXT');
    serenityAdoption.emit('chat_failed', { message: 'PRIVATE-TEXT' });
    serenityAdoption.emit('chat_answered', 'PRIVATE-TEXT');
  });
  await expect.poll(() => events.length).toBe(2);
  expect(events).toEqual([{ version: 1, name: 'chat_failed', page: 'chat', outcome: 'unknown' }, { version: 1, name: 'chat_answered', page: 'chat', outcome: 'answer' }]);
  expect(outgoing).toEqual([]);
  expect(await page.evaluate(() => serenityAdoption.counts())).toEqual({ install_cta: 0, docs_open: 0, chat_started: 0, chat_answered: 1, chat_failed: 1 });
});

test('responsive layout preserves navigation, long answers and reduced motion', async ({ page }, testInfo) => {
  await page.goto('/');
  await noOverflow(page);
  await expect(page.getByRole('navigation', { name: 'Main navigation' }).getByRole('link', { name: /^Install/ })).toBeVisible();
  expect(await page.locator('video').evaluateAll(videos => videos.every(v => v.paused))).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('home.png'), fullPage: true });
  await stub(page, { ...answer, answer: 'A long public explanation. '.repeat(100) + answer.answer });
  await page.goto('/chat/');
  await page.getByRole('textbox', { name: 'Your question about Serenity' }).fill('install');
  await page.getByRole('button', { name: 'Send question' }).click();
  await expect(page.locator('#thread').getByRole('link', { name: 'the guide' })).toBeVisible();
  await noOverflow(page);
  await expect(page.getByRole('button', { name: 'Send question' })).toBeInViewport();
  await page.screenshot({ path: testInfo.outputPath('chat.png'), fullPage: true });
});
