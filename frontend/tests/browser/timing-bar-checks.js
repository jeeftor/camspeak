async (page) => {
  await page.route('**/api/describe/jobs/*', async route => route.fulfill({ json: {
    id: route.request().url().split('/').pop(), camera: 'Front Door', status: 'done', stage: 'done', elapsed_ms: 6000,
    result: { status: 'ok', tts_mode: 'streaming', capture_source: 'Direct API / main', description: 'A package is beside the front door.', ttfs_ms: 2000, total_ms: 6000,
      timings: { snapshot_ms: 300, vision_ms: 900, tts_ms: 650, transcode_ms: 50, send_open_ms: 100, send_playback_ms: 4000 } }
  } }));
  await page.evaluate(() => { location.hash = '/cameras'; });
  const camera = page.getByRole('article', { name: 'Front Door', exact: true });
  await camera.getByRole('button', { name: 'Describe', exact: true }).click();
  await camera.getByRole('button', { name: 'Describe and speak', exact: true }).click();
  await camera.getByText('TTS · Streaming', { exact: true }).waitFor();
  if (!(await camera.getByText('Captured using Direct API / main', { exact: true }).count())) throw new Error('Missing actual source');
  const bar = camera.getByLabel('Stage duration bar', { exact: true });
  if (await bar.getByRole('button').count() !== 6) throw new Error('Missing stage segments');
  await bar.getByRole('button', { name: 'TTS: 650ms', exact: true }).click();
  for (const width of [1440, 390, 320]) {
    await page.setViewportSize({ width, height: 1000 });
    await bar.scrollIntoViewIfNeeded();
    const barBox = await bar.boundingBox();
    const playbackBox = await bar.getByRole('button', { name: 'Playback: 4.0s', exact: true }).boundingBox();
    if (!barBox || !playbackBox || Math.abs(playbackBox.width / barBox.width - 2/3) > 0.02) throw new Error(`Timing segments lost proportions at ${width}px`);
    if (await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)) throw new Error(`Timing overflow ${width}`);
    await page.screenshot({ path: `output/playwright/timing-bar-${width}.png` });
  }
  return { passed: 'Actual streaming label, source metadata, six colored segments, tap details, desktop/mobile layout' };
}
