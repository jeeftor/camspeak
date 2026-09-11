async (page) => {
  // Run after workspace-fixture.js; audio and camera traffic remain mocked.
  await page.goto('http://127.0.0.1:4176/#/cameras');
  await page.evaluate(() => {
    window.testVuLevel = 0.08;
    window.EventSource = class {
      constructor() {
        this.timer = setInterval(() => this.onmessage?.({ data: JSON.stringify({ 'Front Door': window.testVuLevel }) }), 50);
      }
      close() { clearInterval(this.timer); }
    };
  });
  const card = page.getByRole('article', { name: 'Front Door', exact: true });
  const output = card.getByRole('region', { name: 'Front Door Camera Output' });
  await card.getByRole('textbox', { name: 'Message', exact: true }).fill('Meter layout test');
  await card.locator('[data-primary-action]').click();
  await card.getByText('Last action · Speak', { exact: true }).waitFor();
  const results = [];
  for (const width of [390, 1440]) {
    await page.setViewportSize({ width, height: 1000 });
    for (const orientation of ['horizontal', 'vertical']) {
      if (orientation === 'vertical') {
        await card.getByRole('button', { name: 'Presets', exact: true }).click();
        await card.getByRole('combobox', { name: 'Preset', exact: true }).selectOption(JSON.stringify(['Messages', 'Dinner is ready']));
        await card.locator('[data-primary-action]').click();
        await card.getByText('Last action · Preset · Dinner is ready', { exact: true }).waitFor();
      } else {
        await card.getByRole('group', { name: 'Audio source' }).getByRole('button', { name: 'Speak', exact: true }).click();
        await card.locator('[data-primary-action]').click();
        await card.getByText('Last action · Speak', { exact: true }).waitFor();
      }
      let baseline;
      for (const percent of [8, 42, 100, 2]) {
        await page.evaluate(value => { window.testVuLevel = value / 100; }, percent);
        const meter = output.getByRole('meter');
        await meter.locator('..').getByText(`${percent}%`, { exact: true }).waitFor();
        const box = await meter.evaluate(node => {
          const parent = node.parentElement.getBoundingClientRect();
          const bar = node.getBoundingClientRect();
          const text = node.nextElementSibling.getBoundingClientRect();
          return { width: parent.width, height: parent.height, barX: bar.x - parent.x, textWidth: text.width };
        });
        baseline ??= box;
        if (JSON.stringify(box) !== JSON.stringify(baseline)) throw new Error(`VU resized at ${width}px/${orientation}/${percent}%`);
      }
      results.push({ width, orientation, stable: baseline });
    }
  }
  return { passed: 'VU geometry stays fixed at 8%, 42%, 100%, and 2%', results };
}
