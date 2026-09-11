async (page) => {
  // Run with playwright-cli run-code --filename. All camera traffic is mocked.
  await page.unrouteAll({ behavior: 'ignoreErrors' });
  const cameras = ['Front Door', 'Backyard', 'Driveway'].map(name => ({
    name, type: 'hikvision', online: true, enabled: true, gain: 3,
    capabilities: { speak: true, live_stream: true, snapshot: true },
  }));
  const presets = [
    { name: 'Dinner is ready', category: 'Messages', duration: 3 },
    { name: 'Doorbell', category: 'Sounds', duration: 2 },
    { name: 'Jazz Radio', category: 'Streams', url: 'https://example.com/radio.m3u', duration: 0 },
  ];
  const states = {};
  const actions = [];
  const snapshots = [];
  const errors = [];
  const jobs = {};
  const polls = [];
  let describeScenario = 'normal';
  page.on('pageerror', error => errors.push(error.message));
  const binding = `workspaceEvidence${Date.now()}`;
  await page.exposeFunction(binding, scenario => {
    if (scenario) describeScenario = scenario;
    return { actions, snapshots, errors, polls };
  });
  await page.addInitScript(binding => {
    window.workspaceEvidence = () => window[binding]();
    window.workspaceDescribeScenario = scenario => window[binding](scenario);
    // A stable synthetic SSE source avoids reconnection noise from finite fixtures.
    window.EventSource = class {
      constructor() {
        this.timer = setInterval(() => this.onmessage?.({ data: JSON.stringify({ 'Front Door': 0.6, Backyard: 0.4 }) }), 100);
      }
      close() { clearInterval(this.timer); }
    };
  }, binding);
  await page.route('**/api/**', async route => {
    const request = route.request();
    const path = '/' + request.url().split('/').slice(3).join('/').split('?')[0];
    let data = {};
    if (path.startsWith('/api/snapshot/')) {
      const camera = decodeURIComponent(path.split('/').pop());
      snapshots.push(camera);
      return route.fulfill({ contentType: 'image/svg+xml', body: `<svg xmlns="http://www.w3.org/2000/svg" width="640" height="360"><rect width="640" height="360" fill="#173640"/><path d="M0 280L200 150L330 230L480 110L640 280V360H0Z" fill="#376255"/><text x="24" y="42" fill="white" font-family="sans-serif" font-size="22">${camera} · mock preview</text></svg>` });
    }
    if (request.method() !== 'GET') {
      const body = request.headers()['content-type']?.includes('application/json') ? request.postDataJSON() : {};
      actions.push({ path, body });
      if (path === '/api/stop') {
        delete states[body.camera];
        for (const job of Object.values(jobs)) {
          if (job.camera === body.camera && job.status === 'running') {
            job.status = 'canceled'; job.stage = 'canceled'; job.error = 'Describe canceled';
          }
        }
      }
      else if (path === '/api/pause' || path === '/api/resume') states[body.camera].state = path === '/api/pause' ? 'paused' : 'playing';
      else if (body.camera) states[body.camera] = {
        state: 'playing', source: path.split('/').pop(), detail: body.text || body.preset || body.url,
        can_pause: path === '/api/play-stream' || !!body.loop || body.preset === 'Jazz Radio',
      };
      data = { status: 'ok', total_ms: 2400, ttfs_ms: 1300, timings: { tts_ms: 500, send_ms: 1100, send_open_ms: 100 } };
      if (path === '/api/describe/jobs') {
        if (describeScenario === 'proxy-start') return route.fulfill({ status: 524, contentType: 'text/html', body: '<!DOCTYPE html><html>Cloudflare timeout</html>' });
        const id = `describe-${Object.keys(jobs).length + 1}`;
        data = jobs[id] = { id, camera: body.camera, status: 'running', stage: 'snapshot', elapsed_ms: 0, result: { timings: {} }, polls: 0, scenario: describeScenario };
        states[body.camera] = { state: 'preparing', source: 'describe', detail: 'Preparing description' };
        return route.fulfill({ status: 202, contentType: 'application/json', body: JSON.stringify(data) });
      }
      if (path === '/api/describe') {
        await page.waitForTimeout(500);
        data.description = `${body.camera} is clear. A package is beside the door.`;
        data.timings = { snapshot_ms: 200, snap_ms: 200, vision_ms: 900, tts_ms: 500, transcode_ms: 0, send_open_ms: 100, send_ms: 1100 };
      }
      if (path === '/api/library/upload') data = { job_id: 'fixture-job' };
    } else if (path === '/api/cameras' || path === '/api/config/cameras') data = cameras;
    else if (path === '/api/voices') data = ['af_heart', 'af_bella'];
    else if (path === '/api/library') data = presets;
    else if (path === '/api/health') data = { version: 'workspace-fixture' };
    else if (path === '/api/playback') data = states;
    else if (path.startsWith('/api/describe/jobs/')) {
      const job = jobs[path.split('/').pop()];
      polls.push({ id: job.id, time: Date.now() });
      if (job.status === 'running') {
        job.polls++;
        const stages = ['snapshot', 'vision', 'tts', 'transcode', 'connecting', 'playing', 'done'];
        const index = Math.min(job.polls, job.scenario === 'hold' ? 4 : 6);
        job.stage = stages[index];
        job.elapsed_ms = job.polls * 500;
        job.result.timings = { snapshot_ms: 200, snap_ms: 200 };
        if (index >= 2) { job.result.description = `${job.camera} is clear. A package is beside the door.`; job.result.timings.vision_ms = 900; }
        if (index >= 3) job.result.timings.tts_ms = 500;
        if (index >= 4) job.result.timings.transcode_ms = 0;
        if (index >= 5) {
          job.result.timings.send_open_ms = 100;
          job.result.ttfs_ms = 1700;
          states[job.camera] = { state: 'playing', source: 'describe', detail: job.result.description };
        }
        if (index === 6) {
          job.status = 'done'; job.result.total_ms = 2800; job.result.timings.send_ms = 1100;
          delete states[job.camera];
        }
      }
      data = job;
    }
    else if (path.endsWith('/peaks')) data = { peaks: [0.1, 0.4, 0.7, 0.3, 0.5, 0.8, 0.2], duration: 3 };
    else if (path.endsWith('/info')) data = { online: true, device: { model: 'Mock speaker', firmware: 'test' } };
    else if (path === '/api/library/upload/jobs/fixture-job') data = { status: 'done', percent: 100, step: 'Complete', name: 'Uploaded', category: 'Uploads' };
    else if (path === '/api/config/tts' || path === '/api/config/vision/prompts') data = [];
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(data) });
  });
  await page.goto('http://127.0.0.1:4176');
  await page.getByRole('img', { name: 'Front Door preview', exact: true }).waitFor();
  return { fixture: 'Three cameras; finite presets and saved stream; all API traffic mocked.' };
}
