async (page) => {
  const requests = [];
  await page.route('**/api/config/tts', route => route.fulfill({ json: { presets: [{ name: 'Lemonade test', endpoint: 'http://example.com/speech', model: 'kokoro-v1', default_voice: 'af_sky', is_active: true }] } }));
  await page.route('**/api/config/tts/benchmark', async route => {
    const body = route.request().postDataJSON();
    requests.push(body);
    await route.fulfill({ json: { mode: body.mode, first_byte_ms: body.mode === 'streaming' ? 100 : 900, total_ms: 1000, bytes: 48000,
      audio: 'data:audio/wav;base64,UklGRiQAAABXQVZFZm10IBAAAAABAAEAwF0AAIC7AAACABAAZGF0YQAAAAA=' } });
  });
  await page.evaluate(() => { location.hash = '/config'; });
  await page.getByRole('button', { name: 'TTS Presets', exact: true }).click();
  await page.getByRole('button', { name: 'Edit TTS preset', exact: true }).click();
  const panel = page.locator('section').filter({ has: page.getByRole('heading', { name: 'TTS streaming test · experimental', exact: true }) });
  await panel.getByRole('button', { name: 'Compare buffered vs streaming', exact: true }).click();
  await panel.getByText('Comparison finished', { exact: true }).waitFor();
  if (requests.length !== 2 || requests[0].mode !== 'buffered' || requests[1].mode !== 'streaming' || requests[0].text !== requests[1].text || requests[0].preset !== requests[1].preset) throw new Error('Comparison did not use identical inputs');
  if (await panel.locator('audio').count() !== 2 || await panel.locator('audio[autoplay]').count()) throw new Error('Local previews missing or auto-playing');
  for (const width of [1440, 390, 320]) {
    await page.setViewportSize({ width, height: 1000 });
    await panel.scrollIntoViewIfNeeded();
    if (await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)) throw new Error(`Overflow ${width}`);
    await page.screenshot({ path: `output/playwright/tts-streaming-${width}.png`, fullPage: true });
  }
  await page.getByRole('dialog').getByRole('button', { name: 'Close', exact: true }).click();
  await page.evaluate(() => { location.hash = '/benchmark'; });
  await page.getByRole('button', { name: 'Test All Cameras…', exact: true }).click();
  if (!(await page.getByText('Front Door', { exact: true }).count())) throw new Error('Matrix omitted camera summaries');
  return { passed: 'Buffered/streaming comparison, matching inputs, local-only previews, responsive layout, matrix camera selection' };
}
