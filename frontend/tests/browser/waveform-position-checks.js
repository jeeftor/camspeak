async (page) => {
  // Run after workspace-fixture.js; no real speaker requests are made.
  const check = (condition, message) => { if (!condition) throw new Error(message); };
  let state = { state: 'preparing', source: 'play', detail: 'Dinner is ready (loop 5x)', started_at: new Date().toISOString(), can_pause: true };
  await page.route('**/api/playback', route => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ 'Front Door': state }) }));
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('http://127.0.0.1:4176/#/cameras');
  const card = page.getByRole('article', { name: 'Front Door', exact: true });
  await card.getByRole('button', { name: 'Presets', exact: true }).click();
  await card.getByRole('combobox', { name: 'Preset', exact: true }).selectOption(JSON.stringify(['Messages', 'Dinner is ready']));
  await card.locator('[data-primary-action]').click();
  const output = card.getByRole('region', { name: 'Front Door Camera Output' });
  const waveform = output.getByRole('img', { name: 'Audio waveform' });
  const pixels = () => waveform.evaluate(canvas => canvas.toDataURL());
  await waveform.waitFor();
  await output.getByText(/reference waveform/).waitFor();
  await page.waitForTimeout(250);
  const preparing = await pixels();
  await page.waitForTimeout(350);
  check(preparing === await pixels(), 'Preparation does not animate the waveform');
  state = { ...state, state: 'playing', started_at: new Date(Date.now() - 1000).toISOString() };
  await output.getByText(/estimated playback position/).waitFor();
  const playing = await pixels();
  await page.waitForTimeout(350);
  check(playing !== await pixels(), 'Camera Output waveform advances during matching preset playback');
  state = { ...state, state: 'paused', paused_at: new Date().toISOString() };
  await output.getByText('Paused', { exact: true }).waitFor();
  await page.waitForTimeout(150);
  const paused = await pixels();
  await page.waitForTimeout(350);
  check(paused === await pixels(), 'Paused waveform holds its position');
  state = { state: 'idle' };
  await output.getByText(/reference waveform/).waitFor();
  check(preparing === await pixels(), 'Stop resets the waveform');
  check(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'Waveform output fits mobile');
  await page.screenshot({ path: 'output/playwright/waveform-position-mobile.png', fullPage: true });
  return { passed: 'Static preparation, advancing estimated preset position, paused hold, stopped reset, mobile fit' };
}
