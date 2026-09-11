async (page) => {
  // Run after workspace-fixture.js; all model and camera traffic remains mocked.
  await page.route('**/api/vision/test-all/stream', route => route.fulfill({
    contentType: 'text/event-stream',
    body: [
      { type: 'models', models: ['Mock vision'] },
      { type: 'result', model: 'Mock vision', description: 'A package beside the door.', ttfs_ms: 1500, gen_ms: 500, total_ms: 2000 },
      { type: 'done', count: 1 },
    ].map(event => `data: ${JSON.stringify(event)}\n\n`).join(''),
  }));
  await page.evaluate(() => { location.hash = '/benchmark'; });
  await page.getByRole('button', { name: 'Test All Models', exact: true }).waitFor();
  await page.locator('select').filter({ has: page.locator('option[value="Front Door"]') }).first().selectOption('Front Door');
  await page.getByRole('button', { name: 'Test All Models', exact: true }).click();
  const table = page.getByLabel('Mock vision model timing', { exact: true });
  await table.waitFor();
  const bar = page.getByLabel('Stage duration bar', { exact: true });
  const first = bar.getByRole('button', { name: 'First token (TTFT): 1.5s; accumulated 1.5s', exact: true });
  const generation = bar.getByRole('button', { name: 'Generate answer: 500ms; accumulated 2.0s', exact: true });
  await first.hover();
  if (!(await first.getAttribute('title')).includes('Model loading is not measured separately')) throw new Error('Missing stage explanation');
  await generation.focus();
  await page.getByText('Σ 2.0s accumulated', { exact: true }).waitFor();
  for (const width of [1440, 390, 320]) {
    await page.setViewportSize({ width, height: 1000 });
    await first.click();
    await bar.scrollIntoViewIfNeeded();
    const box = await bar.boundingBox();
    const segment = await first.boundingBox();
    if (!box || !segment || Math.abs(segment.width / box.width - 0.75) > 0.02) throw new Error(`Bad proportions at ${width}px`);
    if (await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)) throw new Error(`Overflow at ${width}px`);
    await page.screenshot({ path: `output/playwright/model-timing-${width}.png` });
  }
  return { passed: 'Shared model timing bar/table, hover title, focus/tap cumulative details, desktop and mobile proportions' };
}
