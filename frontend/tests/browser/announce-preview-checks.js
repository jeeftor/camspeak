async (page) => {
  const actions = [];
  await page.route('**/api/config', route => route.fulfill({ json: { cameras: {
    'Front Door': { type: 'hikvision', enabled: true },
    Backyard: { type: 'hikvision', enabled: true },
    Doorbell: { type: 'reolink', enabled: false },
    Unsupported: { type: 'onvif', enabled: false },
  } } }));
  await page.route('**/api/config/settings', route => route.fulfill({ json: {} }));
  await page.route('**/api/announce', async route => {
    actions.push(route.request().postDataJSON());
    await route.fulfill({ json: { source_camera: 'Doorbell', target_camera: 'Front Door',
      description: 'A parcel is at the door.', image: 'data:image/svg+xml,%3Csvg xmlns="http://www.w3.org/2000/svg" width="640" height="360"/%3E',
      timings: { snapshot_ms: 200, vision_ms: 900, tts_ms: 500, transcode_ms: 50, send_open_ms: 100, send_ms: 1000 },
      ttfs_ms: 1750, total_ms: 2800 } });
  });
  await page.evaluate(() => { location.hash = '/announce'; });
  const source = page.getByLabel('Source (capture + vision)');
  const target = page.getByLabel('Target (speaker)');
  await source.getByRole('option', { name: 'Doorbell (Playback disabled)', exact: true }).waitFor({ state: 'attached' });
  if (await source.getByRole('option', { name: /Unsupported/ }).count()) throw new Error('Unsupported capture source shown');
  if (await target.getByRole('option', { name: /Doorbell/ }).count()) throw new Error('Disabled speaker target shown');
  await source.selectOption('Doorbell');
  await target.selectOption('Front Door');
  await page.getByRole('button', { name: 'Preview source', exact: true }).click();
  await page.getByRole('img', { name: 'Doorbell source preview', exact: true }).waitFor();
  if (actions.length) throw new Error('Preview triggered an announcement');
  await page.getByRole('button', { name: 'Announce', exact: true }).click();
  await page.getByText('A parcel is at the door.', { exact: true }).waitFor();
  if (actions.length !== 1 || actions[0].source_camera !== 'Doorbell' || actions[0].target_camera !== 'Front Door') throw new Error('Wrong announce routing');
  if (await page.locator('dl[aria-label="Announce timing flow"] dt').count() !== 6) throw new Error('Missing timing stages');
  for (const width of [1440, 390, 320]) {
    await page.setViewportSize({ width, height: 1000 });
    if (await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)) throw new Error(`Overflow at ${width}`);
    await page.screenshot({ path: `output/playwright/announce-preview-${width}.png`, fullPage: true });
  }
  return { passed: 'Disabled capture source, enabled speaker target, silent preview, announce routing, timings, responsive layout' };
}
