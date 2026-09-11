async (page) => {
  // Run after workspace-fixture.js. All API traffic and audio generation are mocked.
  const check = (condition, message) => { if (!condition) throw new Error(message); };
  const actions = [];
  const saved = [];
  await page.route('**/api/library', async route => {
    if (route.request().method() === 'POST') {
      const body = route.request().postDataJSON();
      actions.push({ path: '/api/library', body });
      saved.push({ ...body, duration: body.url ? 0 : 2 });
      return route.fulfill({ json: { status: 'ok' } });
    }
    return route.fulfill({ json: saved });
  });
  await page.route('**/api/library/upload', async route => {
    actions.push({ path: '/api/library/upload' });
    saved.push({ name: 'chime', category: 'uploads', duration: 2 });
    return route.fulfill({ status: 202, json: { job_id: 'fixture-job' } });
  });
  await page.route('**/api/tts/preview', async route => {
    actions.push({ path: '/api/tts/preview', body: route.request().postDataJSON() });
    return route.fulfill({ contentType: 'audio/wav', body: 'mock-audio', headers: { 'X-TTS-Ms': '80' } });
  });
  await page.getByRole('button', { name: 'Library', exact: true }).click();
  await page.getByRole('button', { name: 'Add preset', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Add preset', exact: true });
  await dialog.waitFor();
  for (const width of [1440, 390, 320]) {
    await page.setViewportSize({ width, height: 844 });
    check(await dialog.evaluate(node => node.scrollWidth <= node.clientWidth), `Dialog overflow at ${width}`);
    await page.screenshot({ path: `output/playwright/preset-modal-upload-${width}.png` });
  }
  await dialog.getByLabel('Audio file', { exact: true }).evaluate(input => {
    const transfer = new DataTransfer();
    transfer.items.add(new File(['mock-audio'], 'chime.wav', { type: 'audio/wav' }));
    input.files = transfer.files;
    input.dispatchEvent(new Event('change', { bubbles: true }));
  });
  await dialog.getByRole('button', { name: 'Save', exact: true }).click();
  await dialog.getByText('✓ Uploaded', { exact: true }).waitFor();
  check(actions.some(action => action.path === '/api/library/upload'), 'Upload reuses existing async upload endpoint');
  await dialog.getByRole('button', { name: 'Stream', exact: true }).click();
  await dialog.getByRole('textbox', { name: 'Stream URL', exact: true }).fill('https://example.com/radio.m3u');
  await dialog.getByRole('textbox', { name: 'Name', exact: true }).fill('Test radio');
  const before = await page.evaluate(() => window.workspaceEvidence());
  check(!before.actions.some(action => action.path === '/api/play-stream'), 'Opening and filling stream form must not play audio');
  check(await dialog.getByRole('button', { name: 'Test stream', exact: true }).isDisabled(), 'Stream test requires explicit camera selection');
  await dialog.getByRole('combobox', { name: 'Test on camera', exact: true }).selectOption('Backyard');
  await dialog.getByRole('button', { name: 'Test stream', exact: true }).click();
  await dialog.getByText('Stream sent to Backyard.', { exact: false }).waitFor();
  await page.screenshot({ path: 'output/playwright/preset-modal-stream-mobile.png' });
  await page.keyboard.press('Escape');
  await dialog.waitFor({ state: 'hidden' });
  await page.waitForTimeout(150);
  const after = await page.evaluate(() => window.workspaceEvidence());
  check(after.actions.some(action => action.path === '/api/play-stream' && action.body.camera === 'Backyard'), 'Test only targets selected camera');
  check(after.actions.some(action => action.path === '/api/stop' && action.body.camera === 'Backyard'), 'Escape stops the active stream test');
  await page.getByRole('button', { name: 'Add preset', exact: true }).click();
  check(await dialog.getByRole('textbox', { name: 'Name', exact: true }).inputValue() === 'Test radio', 'Dialog close preserves draft');
  await dialog.getByRole('button', { name: 'Save Stream Preset', exact: true }).click();
  await dialog.getByText('✓ Saved', { exact: true }).waitFor();
  await dialog.getByRole('button', { name: 'Generate TTS', exact: true }).click();
  await page.evaluate(() => {
    window.presetAudioPlayCalls = 0;
    window.presetOriginalPlay = HTMLMediaElement.prototype.play;
    HTMLMediaElement.prototype.play = function () { window.presetAudioPlayCalls++; return Promise.resolve(); };
  });
  await dialog.getByRole('textbox', { name: 'Text', exact: true }).fill('Please leave the package at the door.');
  await dialog.getByRole('button', { name: 'Generate audio', exact: true }).click();
  await dialog.getByRole('button', { name: 'Preview generated audio', exact: true }).waitFor();
  check(await page.evaluate(() => window.presetAudioPlayCalls === 0), 'Generation must not auto-play browser audio');
  await dialog.getByRole('button', { name: 'Preview generated audio', exact: true }).click();
  check(await page.evaluate(() => window.presetAudioPlayCalls === 1), 'Generated preview requires an explicit click');
  await page.evaluate(() => { HTMLMediaElement.prototype.play = window.presetOriginalPlay; });
  await dialog.getByRole('textbox', { name: 'Name', exact: true }).fill('Package message');
  await dialog.getByRole('button', { name: 'Save', exact: true }).click();
  await dialog.getByText('✓ Saved', { exact: true }).waitFor();
  await dialog.getByRole('button', { name: 'Close', exact: true }).click();
  await page.getByRole('button', { name: 'Package message', exact: true }).waitFor();
  await page.getByRole('button', { name: 'Test radio', exact: true }).waitFor();
  check(actions.filter(action => action.path === '/api/library').length === 2, 'Stream and TTS share the existing preset save API');
  return { passed: 'Unified preset modal: mobile layout, upload, explicit stream test and close-stop, TTS generation/save, refreshed browse', actions };
}
