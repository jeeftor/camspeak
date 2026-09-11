async (page) => {
  const requests = [];
  const canceled = [];
  let hold = false;
  await page.route('**/api/config/tts', route => route.fulfill({ json: { presets: [{ name: 'Lemonade test', endpoint: 'http://example.com/speech', model: 'kokoro-v1', default_voice: 'af_sky', is_active: true }] } }));
  await page.route('**/api/config/tts/benchmark/speaker', async route => {
    requests.push(route.request().postDataJSON());
    await route.fulfill({ status: 202, json: { id: 'speaker-test', status: 'running', stage: 'tts', result: {} } });
  });
  await page.route('**/api/config/tts/benchmark/jobs/speaker-test', async route => {
    if (route.request().method() === 'DELETE') { canceled.push('speaker-test'); hold = false; await route.fulfill({ json: { status: 'canceling' } }); return; }
    await route.fulfill({ json: { id: 'speaker-test', status: hold ? 'running' : canceled.length ? 'canceled' : 'done', stage: hold ? 'tts' : 'done', result: {
      buffered: { timings: { tts_ms: 900, send_open_ms: 100 }, ttfs_ms: 1000, total_ms: 2000 },
      streaming: { timings: { tts_ms: 300, send_open_ms: 100 }, ttfs_ms: 400, total_ms: 1600 },
    } } });
  });
  await page.evaluate(() => { location.hash = '/config'; });
  await page.getByRole('button', { name: 'TTS Presets', exact: true }).click();
  await page.getByRole('button', { name: 'Edit TTS preset', exact: true }).click();
  const panel = page.locator('section').filter({ has: page.getByRole('heading', { name: 'End-to-end speaker comparison', exact: true }) });
  const start = panel.getByRole('button', { name: 'Play both modes on camera', exact: true });
  if (!(await start.isDisabled()) || requests.length) throw new Error('Playback was not opt-in');
  await panel.getByLabel('Benchmark speaker', { exact: true }).selectOption({ label: 'Front Door' });
  if (!(await start.isDisabled())) throw new Error('Missing confirmation gate');
  await panel.getByLabel('I understand this will play sound').check();
  await start.click();
  await panel.getByText('Speaker comparison finished', { exact: true }).waitFor();
  await panel.getByText('Streaming sent first audio 600ms earlier than buffered.', { exact: true }).waitFor();
  if (requests.length !== 1 || !requests[0].confirm_playback || requests[0].streaming_first) throw new Error('Incorrect benchmark request');
  await panel.getByLabel('Streaming first (reverse the order)').check();
  await start.click();
  await panel.getByText('Speaker comparison finished', { exact: true }).waitFor();
  if (requests.length !== 2 || !requests[1].streaming_first) throw new Error('Missing reversed order');
  for (const width of [1440, 390, 320]) {
    await page.setViewportSize({ width, height: 1000 });
    await panel.scrollIntoViewIfNeeded();
    if (await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)) throw new Error(`Overflow ${width}`);
    await page.screenshot({ path: `output/playwright/speaker-benchmark-${width}.png`, fullPage: true });
  }
  hold = true;
  await start.click();
  await panel.getByRole('button', { name: 'Stop comparison', exact: true }).click();
  await panel.getByText('Comparison stopped', { exact: true }).waitFor();
  if (canceled.length !== 1 || requests.length !== 3) throw new Error('Stop was not scoped or start was retried');
  return { passed: 'Explicit opt-in, matching preset, both timings, reverse order, scoped Stop, responsive layout' };
}
