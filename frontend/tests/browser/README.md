# Camera workspace browser checks

These reusable Playwright CLI scripts intercept every `/api/**` request. They do
not contact cameras or test physical audio, backend timing accuracy, or real SSE
delivery. Run against the production build from the repository root:

```sh
cd frontend
bun run build
bun run preview --host 127.0.0.1 --port 4176 --strictPort
```

In another terminal at the repository root, use Playwright CLI with bundled
Chromium or Chrome Headless Shell (not the full macOS Chrome application):

```sh
mkdir -p output/playwright
playwright-cli -s=workspace open about:blank --config=output/playwright/browser.json
playwright-cli -s=workspace run-code --filename frontend/tests/browser/workspace-fixture.js
playwright-cli -s=workspace snapshot
playwright-cli -s=workspace run-code --filename frontend/tests/browser/workspace-checks.js
playwright-cli -s=workspace snapshot
playwright-cli -s=workspace run-code --filename frontend/tests/browser/workspace-touch.js
playwright-cli -s=workspace console warning
playwright-cli -s=workspace close
```

Create the local `browser.json` with your headless executable path:

```json
{
  "browser": {
    "browserName": "chromium",
    "launchOptions": { "headless": true, "executablePath": "/path/to/chrome-headless-shell" },
    "contextOptions": { "viewport": { "width": 1440, "height": 1000 }, "hasTouch": true }
  },
  "outputDir": "output/playwright"
}
```

Screenshots are written under `output/playwright/`. Layout checks cover 320,
390, 768, 1024, and 1440 pixel widths; touch checks use Chromium's native touch
events, not synthetic pointer events. Re-run the fixture to reset mocked state,
then run the two check scripts in order. Resize to 1440 × 1000 before restarting.
