async (page) => {
  // Run after workspace-fixture.js. No requests reach a real camera or WAN host.
  const check = (condition, message) => { if (!condition) throw new Error(message); };
  const card = page.getByRole('article', { name: 'Front Door', exact: true });
  const output = card.getByRole('region', { name: 'Front Door Camera Output' });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.evaluate(() => window.workspaceDescribeScenario('hold'));
  await card.getByRole('button', { name: 'Describe', exact: true }).click();
  await output.getByText('Connecting to speaker', { exact: true }).first().waitFor();
  check((await card.locator('[data-primary-action]').innerText()).trim() === 'Connecting to speaker', 'Primary action names the real stage');
  check(await output.getByText('Preparing audio', { exact: true }).isVisible(), 'Connecting must not claim playback');
  check(await output.getByText('Front Door is clear. A package is beside the door.', { exact: true }).isVisible(), 'Description appears before audio finishes');
  check(await output.locator('dl[aria-label="Describe timing flow"] dt').count() === 6, 'All pipeline stages are visible on mobile');
  check(await output.getByText('In progress…', { exact: true }).count() === 1, 'Only the active stage is in progress');
  check(await output.getByRole('meter').getAttribute('aria-valuenow') === '0', 'No fake level before audio starts');
  check(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'Live flow fits a phone');
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({ path: 'output/playwright/describe-connecting-mobile.png', fullPage: true });
  await output.getByRole('button', { name: 'Stop', exact: true }).click();
  await output.getByText('Last action · Describe', { exact: true }).waitFor();
  check((await output.innerText()).includes('Describe canceled'), 'Stop is reflected by the terminal job');
  check((await output.innerText()).includes('A package is beside the door.'), 'Stop retains generated text and timings');
  await page.evaluate(() => window.workspaceDescribeScenario('proxy-start'));
  await card.getByRole('button', { name: 'Describe', exact: true }).click();
  await output.getByRole('alert').filter({ hasText: 'HTTP 524' }).waitFor();
  check(!(await card.innerText()).includes('<!DOCTYPE'), 'Proxy errors never display raw HTML');
  check((await output.innerText()).includes('not retried'), 'Uncertain starts explain that audio was not retried');
  const evidence = await page.evaluate(() => window.workspaceEvidence());
  check(evidence.actions.filter(action => action.path === '/api/describe/jobs').length === 2, 'Two clicks produce only two starts');
  check(evidence.errors.length === 0, `Browser errors: ${evidence.errors.join(', ')}`);
  await page.screenshot({ path: 'output/playwright/describe-proxy-error-mobile.png', fullPage: true });
  return { passed: 'Mobile live stages, early description, honest playback and VU, cancellation with retained result, readable proxy error and no duplicate start', polls: evidence.polls.length };
}
