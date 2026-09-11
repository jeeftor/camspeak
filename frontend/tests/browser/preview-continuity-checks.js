async (page) => {
  const image = page.getByRole('img', { name: 'Front Door preview', exact: true });
  await image.waitFor();
  const original = await image.getAttribute('src');
  let fail = true;
  await page.route('**/api/snapshot/**', async route => {
    if (fail) return route.fulfill({ status: 503, body: 'Camera temporarily unavailable' });
    return route.fallback();
  });
  await page.getByText(/Last frame .*Retrying preview/).waitFor();
  if (await image.getAttribute('src') !== original) throw new Error('Failed refresh replaced the last frame');
  if (!(await image.getAttribute('class')).includes('opacity-60')) throw new Error('Stale frame is not dimmed');
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 1000 });
    if (await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)) throw new Error('Preview overflow');
    await page.screenshot({ path: `output/playwright/preview-stale-${width}.png`, fullPage: true });
  }
  fail = false;
  await page.waitForFunction(old => document.querySelector('img[alt="Front Door preview"]')?.getAttribute('src') !== old, original);
  if ((await image.getAttribute('class')).includes('opacity-60')) throw new Error('Recovered frame remains dimmed');
  const good = await image.getAttribute('src');
  await page.route('**/api/snapshot/**', route => route.fulfill({ contentType: 'text/html', body: '<html>Bad gateway</html>' }));
  await page.getByText(/Last frame .*Retrying preview/).waitFor();
  if (await image.getAttribute('src') !== good) throw new Error('Invalid image discarded the last good frame');
  console.log('Preview continuity passed: failure, dimming, recovery, invalid image, desktop/mobile.');
}
