async (page) => {
  // Run after workspace-fixture.js; all requests stay mocked, including after reload.
  const check = (condition, message) => { if (!condition) throw new Error(message); };
  let cameras = ['Front Door', 'Disabled Camera', 'Backyard'].map((name, index) => ({
    name, enabled: name !== 'Disabled Camera', online: true, type: 'hikvision',
    ip: '192.0.2.10', channel: 1, gain: 3, airplay_enabled: true, sort_order: index + 1,
    capabilities: { speak: true, snapshot: true },
  }));
  let failOrder = false;
  const saves = [];
  await page.route('**/api/**', async route => {
    const request = route.request();
    const path = '/' + request.url().split('/').slice(3).join('/').split('?')[0];
    let data;
    if (path === '/api/config/cameras/reorder') {
      const names = request.postDataJSON().cameras;
      saves.push(names);
      if (failOrder) return route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: 'Order storage unavailable' }) });
      cameras = names.map((name, index) => ({ ...cameras.find(camera => camera.name === name), sort_order: index + 1 }));
      data = { status: 'ok' };
    } else if (path === '/api/config/cameras') data = [...cameras].reverse(); // Config endpoint is an unordered map.
    else if (path === '/api/cameras') data = cameras.filter(camera => camera.enabled);
    else if (path === '/api/config/vision') data = { url: 'https://example.com/v1', model: 'qwen-vl', prompt: 'Describe briefly' };
    else if (path === '/api/config/vision/test') data = { ok: true, data: { data: [{ id: 'qwen-vl' }, { id: 'llava' }] } };
    else return route.fallback();
    return route.fulfill({ contentType: 'application/json', body: JSON.stringify(data) });
  });
  const go = async hash => { await page.evaluate(hash => { location.hash = hash; }, hash); };
  const openSettings = async () => {
    await go('/config');
    await page.getByRole('button', { name: 'Cameras', exact: true }).last().click();
    await page.getByRole('list', { name: 'Camera order' }).waitFor();
  };
  const rows = () => page.getByRole('list', { name: 'Camera order' }).getByRole('listitem');
  await page.setViewportSize({ width: 1440, height: 1000 });
  await openSettings();
  check(await rows().count() === 3, 'Disabled cameras remain in settings order');
  await page.getByRole('button', { name: 'Move Backyard earlier', exact: true }).click();
  await page.getByText('Camera order saved', { exact: true }).waitFor();
  check(saves.at(-1).join('|') === 'Front Door|Backyard|Disabled Camera', 'Order API includes disabled camera');
  await page.reload();
  await page.getByRole('button', { name: 'Cameras', exact: true }).last().click();
  check((await rows().nth(1).innerText()).includes('Backyard'), 'Saved order survives reload');
  failOrder = true;
  await page.getByRole('button', { name: 'Move Backyard earlier', exact: true }).click();
  await page.getByRole('alert').filter({ hasText: 'Order storage unavailable' }).waitFor();
  check((await rows().nth(0).innerText()).includes('Front Door'), 'Failed save rolls back the displayed order');
  failOrder = false;
  await page.getByRole('button', { name: 'Drag Backyard to reorder', exact: true }).dragTo(rows().nth(0));
  await page.getByText('Camera order saved', { exact: true }).waitFor();
  check(saves.at(-1)[0] === 'Backyard', 'Desktop handle drag saves order');
  await go('/cameras');
  await page.getByRole('navigation', { name: 'Camera selection' }).waitFor();
  check(await page.getByRole('button', { name: 'Arrange', exact: true }).count() === 0, 'Main workspace no longer has Arrange');
  check(await page.getByRole('navigation', { name: 'Camera selection' }).getByRole('button').count() === 2, 'Disabled camera excluded from main navigation');
  await page.setViewportSize({ width: 390, height: 844 });
  check((await page.getByRole('combobox', { name: 'Select camera' }).locator('option').allTextContents()).join('|') === 'Backyard|Front Door', 'Mobile navigation uses saved enabled camera order');
  await openSettings();
  await page.getByRole('button', { name: 'Move Front Door earlier', exact: true }).focus();
  await page.keyboard.press('Enter');
  await page.getByText('Camera order saved', { exact: true }).waitFor();
  check(saves.at(-1)[0] === 'Front Door', 'Keyboard/mobile arrow saves order');
  check(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'Camera settings fit mobile');
  await page.screenshot({ path: 'output/playwright/settings-order-mobile.png', fullPage: true });
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 1000 });
    if (width >= 1024) await page.getByRole('button', { name: 'Benchmark', exact: true }).click();
    else await page.getByRole('combobox', { name: 'More pages' }).selectOption('benchmark');
    await page.getByRole('heading', { name: 'Benchmark', exact: true }).waitFor();
    for (const label of ['Vision models', 'Capture methods', 'Advanced matrix']) {
      await page.getByRole('button', { name: label, exact: true }).click();
      check(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `${label} fits ${width}px`);
    }
    await page.getByRole('button', { name: 'Vision models', exact: true }).click();
    await page.screenshot({ path: `output/playwright/benchmark-${width}.png`, fullPage: true });
  }
  await go('/diagnostics');
  await page.getByRole('heading', { name: 'Benchmark', exact: true }).waitFor();
  await page.waitForURL('**/#/benchmark');
  check((await page.url()).endsWith('#/benchmark'), 'Legacy diagnostics link redirects to Benchmark');
  const evidence = await page.evaluate(() => window.workspaceEvidence());
  check(evidence.errors.length === 0, `Browser errors: ${evidence.errors.join(', ')}`);
  return { passed: 'Settings order, disabled cameras, persistence, rollback, desktop drag, keyboard/mobile arrows, Benchmark navigation and legacy links', saves };
}
