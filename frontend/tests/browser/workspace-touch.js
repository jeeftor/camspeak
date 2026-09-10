async (page) => {
  const check = (condition, message) => { if (!condition) throw new Error(message); };
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole('combobox', { name: 'Select camera' }).selectOption('Front Door');
  const card = page.getByRole('article', { name: 'Front Door', exact: true });
  const cdp = await page.context().newCDPSession(page);
  await cdp.send('Emulation.setTouchEmulationEnabled', { enabled: true });
  async function swipe(target, dx, dy) {
    await target.scrollIntoViewIfNeeded();
    const r = await target.boundingBox();
    const x = r.x + r.width / 2 - dx / 2;
    const y = r.y + r.height / 2 - dy / 2;
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x, y }] });
    for (let i = 1; i <= 6; i++) {
      await cdp.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: x + dx * i / 6, y: y + dy * i / 6 }] });
    }
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
    await page.waitForTimeout(200);
  }
  try {
    await swipe(card.getByRole('img', { name: 'Front Door preview', exact: true }), -120, 0);
    check(await page.getByRole('combobox', { name: 'Select camera' }).inputValue() === 'Backyard', 'Swipe left selects next camera');
    const backyard = page.getByRole('article', { name: 'Backyard', exact: true });
    await swipe(backyard.getByRole('img', { name: 'Backyard preview', exact: true }), 120, 0);
    check(await page.getByRole('combobox', { name: 'Select camera' }).inputValue() === 'Front Door', 'Swipe right selects previous camera');
    await swipe(card.getByRole('img', { name: 'Front Door preview', exact: true }), 0, -100);
    check(await page.getByRole('combobox', { name: 'Select camera' }).inputValue() === 'Front Door', 'Vertical scrolling must not switch cameras');
    await swipe(card.getByRole('slider', { name: 'Front Door volume' }), -80, 0);
    check(await page.getByRole('combobox', { name: 'Select camera' }).inputValue() === 'Front Door', 'Volume gesture must not switch cameras');
  } finally { await cdp.detach(); }
  await card.getByRole('button', { name: 'Files', exact: true }).click();
  await card.locator('input[type=file]').evaluate(input => {
    const data = new DataTransfer();
    data.items.add(new File([new Uint8Array(44)], 'mock.wav', { type: 'audio/wav' }));
    input.files = data.files;
    input.dispatchEvent(new Event('change', { bubbles: true }));
  });
  await card.getByText('Last action · File · mock.wav', { exact: true }).waitFor();
  await card.getByRole('button', { name: 'Device info', exact: true }).click();
  await page.getByRole('dialog', { name: 'Front Door · Device information' }).waitFor();
  await page.getByText('Mock speaker', { exact: true }).waitFor();
  await page.screenshot({ path: 'output/playwright/workspace-device-mobile.png' });
  await page.keyboard.press('Escape');
  await card.getByRole('button', { name: 'Test speaker', exact: true }).click();
  await card.getByText('Last action · Test speaker', { exact: true }).waitFor();
  await card.getByRole('button', { name: 'Settings', exact: true }).click();
  await page.getByRole('dialog', { name: 'Edit Camera — Front Door' }).waitFor();
  await page.screenshot({ path: 'output/playwright/workspace-settings-mobile.png' });
  const result = await page.evaluate(() => window.workspaceEvidence());
  check(result.errors.length === 0, `Browser errors: ${result.errors.join(', ')}`);
  check(result.actions.some(action => action.path === '/api/beep' && action.body.camera === 'Front Door'), 'Test speaker uses selected camera');
  check(result.actions.some(action => action.path === '/api/play' && action.body.preset === 'Uploaded'), 'Upload plays converted preset');
  return { passed: 'Real Chromium touch swipes, vertical scrolling, volume gestures, upload, device info, speaker test and camera settings', errors: result.errors };
}
