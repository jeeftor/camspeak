async (page) => {
  // Run after workspace-fixture.js. SSE and clipboard are controlled; no camera audio is sent.
  const check = (condition, message) => { if (!condition) throw new Error(message); };
  await page.addInitScript(() => {
    window.eventCopies = [];
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: async text => window.eventCopies.push(text) } });
    window.EventSource = class {
      constructor(url) {
        if (url !== '/api/events') return;
        const legacy = { id: 1, camera: 'Backyard', action: 'play', text: 'Old preset', at: '2026-09-10T21:00:00Z' };
        const latest = { id: 2, camera: 'Front Door', action: 'play', text: "It's music", voice: 'af_heart', at: '2026-09-10T21:00:01Z', replay: { method: 'POST', path: '/api/play', body: { camera: 'Front Door', preset: "It's $(touch /tmp/nope)", category: 'alerts', loop: -1, gain: 0 } } };
        const redacted = { id: 3, camera: 'Driveway', action: 'play-stream', text: 'https://example.com/radio', at: '2026-09-10T21:00:02Z', replay: { method: 'POST', path: '/api/play-stream', body: { camera: 'Driveway', url: 'https://example.com/radio' }, redacted: true } };
        this.timer = setTimeout(() => {
          this.onopen?.();
          for (const event of [legacy, latest, redacted, latest, { id: 4, action: 'config', at: latest.at }]) this.onmessage?.({ data: JSON.stringify(event) });
        }, 40);
      }
      close() { clearTimeout(this.timer); }
    };
  });
  await page.evaluate(() => { location.hash = '/events'; });
  await page.reload();
  await page.getByRole('heading', { name: 'Playback activity' }).waitFor();
  await page.getByText("It's music", { exact: true }).waitFor();
  check(await page.getByRole('article').count() === 3, 'History/live duplicate IDs produce one row and exclude configuration traffic');
  const preset = page.getByRole('article').filter({ hasText: "It's music" });
  const old = page.getByRole('article').filter({ hasText: 'Old preset' });
  check(await old.getByRole('button').count() === 0, 'Legacy preset does not offer an incomplete command');
  check((await old.innerText()).includes('did not save enough request options'), 'Legacy limitation is explained');
  await page.setViewportSize({ width: 1440, height: 1000 });
  await preset.getByRole('button', { name: 'Copy cURL', exact: true }).hover();
  check(await preset.locator('.curl-tooltip.show').isVisible(), 'Hover previews the cURL command');
  await preset.getByRole('button', { name: 'Copy JSON body', exact: true }).focus();
  check(await preset.locator('.curl-tooltip.show').last().isVisible(), 'Keyboard focus previews JSON');
  await page.screenshot({ path: 'output/playwright/events-desktop.png', fullPage: true });
  for (const width of [390, 320]) {
    await page.setViewportSize({ width, height: 844 });
    await preset.getByRole('button', { name: 'Copy JSON body', exact: true }).click();
    const copied = await page.evaluate(() => window.eventCopies.at(-1));
    const body = JSON.parse(copied);
    check(body.gain === 0 && body.loop === -1 && body.category === 'alerts' && body.preset === "It's $(touch /tmp/nope)", 'Mobile JSON copy preserves all request options');
    await preset.getByRole('button', { name: 'Copy cURL', exact: true }).click();
    check((await page.evaluate(() => window.eventCopies.at(-1))).includes("It'\\''s $(touch /tmp/nope)"), 'Copied cURL shell-quotes untrusted preset text');
    const details = preset.locator('details');
    if ((await details.getAttribute('open')) === null) await details.locator('summary').click();
    check(await preset.getByText('POST /api/play', { exact: true }).isVisible(), 'Endpoint stays visible with JSON body');
    await page.mouse.move(0, 0);
    await page.evaluate(() => document.activeElement?.blur());
    check(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `Activity fits ${width}px`);
    await page.screenshot({ path: `output/playwright/events-${width}.png`, fullPage: true });
  }
  check((await page.getByRole('article').first().innerText()).includes('credentials and query parameters were removed'), 'Redacted stream commands explain missing credentials');
  const evidence = await page.evaluate(() => window.workspaceEvidence());
  check(evidence.errors.length === 0, `No browser exceptions: ${evidence.errors.join(', ')}`);
  return { passed: 'Playback history/live deduplication, legacy safety, hover/focus previews, mobile JSON/cURL copy, shell escaping, redaction and responsive widths', rows: 3 };
}
