// Real Chromium integration; no live TypeSafe calls. Set PLAYWRIGHT_MODULE if
// Playwright is not installed in the environment. No extension test backdoors.
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { readFile, writeFile, mkdir, rm } from 'node:fs/promises';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { resolve } from 'node:path';
import { pageOperation } from '../page.js';
import { syncPageMarks } from '../marks.js';

const { chromium } = await import(process.env.PLAYWRIGHT_MODULE ? pathToFileURL(process.env.PLAYWRIGHT_MODULE).href : 'playwright');
const root = fileURLToPath(new URL('../', import.meta.url));
const artifacts = resolve(root, '.artifacts');
await mkdir(artifacts, { recursive: true });
const server = createServer(async (req, res) => {
  if (req.url === '/cat.svg') {
    res.writeHead(200, { 'Content-Type': 'image/svg+xml' });
    res.end('<svg xmlns="http://www.w3.org/2000/svg" width="200" height="130"><rect width="200" height="130" fill="#ddd7b0"/><text x="50" y="80" font-size="54">CAT</text></svg>');
  } else if (req.url === '/frame') {
    res.writeHead(200, { 'Content-Type': 'text/html' }); res.end('<a href="/frame-link" style="display:block;padding:20px">Frame link</a>');
  } else if (req.url === '/destination') {
    res.writeHead(200, { 'Content-Type': 'text/html' }); res.end('<h1>Destination page</h1>');
  } else {
    res.writeHead(200, { 'Content-Type': 'text/html' }); res.end(await readFile(new URL('fixture.html', import.meta.url)));
  }
});
await new Promise(r => server.listen(0, '127.0.0.1', r));
const origin = `http://127.0.0.1:${server.address().port}`;
let context;
try {
  const profile = resolve(artifacts, 'browser-profile');
  context = await chromium.launchPersistentContext(profile, {
    ...(process.env.CHROMIUM_EXECUTABLE ? { executablePath: process.env.CHROMIUM_EXECUTABLE } : { channel: 'chromium' }),
    headless: true, viewport: { width: 1100, height: 900 },
    args: [`--disable-extensions-except=${root}`, `--load-extension=${root}`]
  });
  let worker = context.serviceWorkers()[0];
  if (!worker) worker = await context.waitForEvent('serviceworker');
  const extensionId = new URL(worker.url()).host;
  const extension = `chrome-extension://${extensionId}`;
  const fixture = await context.newPage(); await fixture.goto(origin); await fixture.bringToFront();
  const point = async selector => fixture.locator(selector).evaluate(el => {
    const r = el.getBoundingClientRect(); return { x: r.x + r.width / 2, y: r.y + r.height / 2 };
  });
  const operate = (action, p, expected, context) => fixture.evaluate(({ source, action, p, expected, context }) => (0, eval)(`(${source})`)(action, p, expected, context), { source: pageOperation.toString(), action, p, expected, context });
  assert.equal((await operate('CLICK', await point('#like'))).ok, true);
  assert.equal(await fixture.locator('#like').getAttribute('aria-pressed'), 'true');
  assert.equal((await operate('SAVE_TEXT', await point('#text'))).value, 'A cat is asleep in the sunshine.');
  assert.equal((await operate('SAVE_IMAGE', await point('#cat'))).value, `${origin}/cat.svg`);
  assert.equal((await operate('SAVE_LINK', await point('#link'))).value, `${origin}/destination`);
  assert.equal((await operate('SAVE_LINK', await point('#text'))).ok, false);
  const old = await operate('META'); await fixture.evaluate(() => scrollBy(0, 50));
  assert.equal((await operate('CLICK', { x: 70, y: 100 }, old)).ok, false);
  await fixture.evaluate(() => scrollTo(0, 0));
  assert.equal((await operate('CLICK', await point('#shadow-button'))).ok, true);
  const frameLink = await fixture.frameLocator('#frame').locator('a').boundingBox();
  assert.equal((await operate('SAVE_LINK', { x: frameLink.x + frameLink.width / 2, y: frameLink.y + frameLink.height / 2 })).value, `${origin}/frame-link`);
  assert.match((await operate('SCROLL_DOWN')).message, /Scrolled/);
  assert.match((await operate('SCROLL_UP')).message, /Scrolled/);
  console.log('Real DOM: click, text/image/link extraction, shadow DOM, same-origin iframe, stale viewport, scroll passed.');

  await fixture.reload();
  let markRecords = []; let markStep = 0;
  const sync = (id = 'marks-test', records = markRecords) => fixture.evaluate(({ source, id, records }) => (0, eval)(`(${source})`)(id, records), { source: syncPageMarks.toString(), id, records });
  await sync();
  const markedAction = async (action, selector, allowRepeat = false) => {
    const p = await point(selector);
    const context = { runId: 'marks-test', step: ++markStep, action, allowRepeat };
    const prepared = await operate('PREPARE', p, null, context);
    if (!prepared.ok) return prepared;
    if (action === 'CLICK') markRecords = [...markRecords.filter(m => m.id !== prepared.mark.id), prepared.mark];
    const result = await operate(action, p, null, { ...context, prepared: prepared.mark });
    if (result.mark) markRecords = [...markRecords.filter(m => m.id !== result.mark.id), result.mark];
    return result;
  };
  const assertOutline = async selector => {
    await fixture.evaluate(() => new Promise(requestAnimationFrame));
    const pixels = (await fixture.screenshot()).toString('base64');
    const rgb = await fixture.evaluate(async ({ pixels, selector }) => {
      const image = await createImageBitmap(await (await fetch(`data:image/png;base64,${pixels}`)).blob());
      const canvas = document.createElement('canvas'); canvas.width = image.width; canvas.height = image.height;
      const ctx = canvas.getContext('2d'); ctx.drawImage(image, 0, 0); image.close();
      const r = document.querySelector(selector).getBoundingClientRect();
      return [...ctx.getImageData(Math.round(r.left + 1), Math.round(r.top + r.height / 2), 1, 1).data];
    }, { pixels, selector });
    assert.deepEqual(rgb.slice(0, 3), [239, 23, 45], `${selector}: expected actual red outline pixels`);
  };
  await fixture.evaluate(() => { window.testClicks = 0; document.getElementById('like').addEventListener('click', () => window.testClicks++); });
  assert.equal((await markedAction('CLICK', '#like')).ok, true);
  await assertOutline('#like');
  assert.equal((await markedAction('CLICK', '#like')).blocked, true);
  assert.equal(await fixture.evaluate(() => window.testClicks), 1);
  assert.equal((await markedAction('SAVE_TEXT', '#like')).value, 'Liked');
  assert.equal((await markedAction('CLICK', '#like', true)).ok, true);
  assert.equal((await markedAction('CLICK', '#like', true)).retryAllowed, false);
  assert.equal(await fixture.evaluate(() => window.testClicks), 2);
  assert.equal((await markedAction('SAVE_IMAGE', '#cat')).ok, true);
  assert.equal((await markedAction('SAVE_LINK', '#link')).ok, true);
  assert.equal((await markedAction('SAVE_TEXT', '#text')).ok, true);
  assert.equal((await markedAction('SAVE_TEXT', '#text')).blocked, true);
  await fixture.locator('#text').evaluate(el => { const child = document.createElement('span'); child.textContent = el.textContent; el.replaceChildren(child); });
  assert.equal((await markedAction('SAVE_TEXT', '#text span')).blocked, true, 'A nested span must not evade the text repeat guard.');
  const hit = await fixture.evaluate(p => document.elementFromPoint(p.x, p.y).id, await point('#cat'));
  assert.equal(hit, 'cat');
  assert.equal(await fixture.locator('[data-jevbot-marks]').evaluate(el => el.shadowRoot), null);
  await fixture.evaluate(() => scrollBy(0, 60)); await assertOutline('#cat');
  await fixture.evaluate(() => { document.getElementById('cat').style.marginLeft = '80px'; }); await assertOutline('#cat');
  await fixture.setViewportSize({ width: 950, height: 850 }); await assertOutline('#cat');
  await fixture.screenshot({ path: resolve(artifacts, 'visual-marks.png'), fullPage: false });
  const savedText = await operate('SAVE_TEXT', { x: 20, y: 20 });
  assert.doesNotMatch(savedText.value || '', /Text saved|Image saved|Link saved|Click attempted/);
  // A virtualized node reused for new content must not inherit old marks.
  await fixture.evaluate(() => { document.getElementById('cat').src = '/other-cat.svg'; });
  await sync(); assert.equal((await markedAction('SAVE_IMAGE', '#cat')).ok, true);
  // Save-link must not prevent clicking that same anchor.
  const linkContext = { runId: 'marks-test', action: 'CLICK', step: ++markStep };
  const linkPoint = await point('#link'); const linkPrepared = await operate('PREPARE', linkPoint, null, linkContext);
  assert.equal(linkPrepared.ok, true); markRecords.push(linkPrepared.mark);
  await operate('CLICK', linkPoint, null, { ...linkContext, prepared: linkPrepared.mark }).catch(() => {});
  await fixture.waitForURL(`${origin}/destination`); await fixture.goBack();
  await sync();
  const restored = await markedAction('CLICK', '#link'); assert.equal(restored.blocked, true);
  await assertOutline('#link');
  // Re-created nodes bind by content only when there is a unique match.
  await fixture.evaluate(() => { const link = document.getElementById('link'); link.replaceWith(link.cloneNode(true)); });
  await sync(); assert.equal((await markedAction('CLICK', '#link')).blocked, true);
  await fixture.evaluate(() => { const link = document.getElementById('link'); const clone = link.cloneNode(true); clone.removeAttribute('id'); link.after(clone); });
  await sync('clear-before-ambiguous', []); await sync();
  const ambiguous = await operate('PREPARE', await point('#link'), null, { runId: 'marks-test', action: 'CLICK', step: ++markStep });
  assert.equal(ambiguous.ok, true, 'Ambiguous restoration must not guess from coordinates.');
  await sync('new-run', []);
  assert.equal((await operate('PREPARE', await point('#text'), null, { runId: 'new-run', action: 'SAVE_TEXT', step: 1 })).ok, true);
  await sync(null, []); assert.equal(await fixture.locator('[data-jevbot-marks]').count(), 0);
  // Some pages forbid inline stylesheet elements. The memory overlay must work
  // there too, without asking the page to relax its Content Security Policy.
  await fixture.evaluate(() => {
    const csp = document.createElement('meta'); csp.httpEquiv = 'Content-Security-Policy'; csp.content = "style-src 'none'"; document.head.append(csp);
  });
  await sync('strict-style', []);
  const strictContext = { runId: 'strict-style', action: 'SAVE_TEXT', step: 1 };
  const strictPoint = await point('#text');
  const strictPrepared = await operate('PREPARE', strictPoint, null, strictContext);
  assert.equal((await operate('SAVE_TEXT', strictPoint, null, { ...strictContext, prepared: strictPrepared.mark })).ok, true);
  await assertOutline('#text');
  console.log('Visual memory: actual red pixels, scroll/resize/layout alignment, pointer transparency, operation guard, one retry, navigation/recreated-node restore, recycled-node and ambiguous-match safety, fresh-run reset passed.');

  const options = await context.newPage(); await options.goto(`${extension}/options.html`);
  await options.locator('#api-key').fill('mock-key-for-local-test');
  await options.locator('#settle-ms').fill('500');
  await options.locator('button[type=submit]').click();
  await options.waitForFunction(() => document.getElementById('saved').textContent.includes('Saved'));
  await options.reload(); assert.equal(await options.locator('#api-key').inputValue(), 'mock-key-for-local-test');
  await options.screenshot({ path: resolve(artifacts, 'settings.png'), fullPage: true });

  await fixture.reload();
  const panel = await context.newPage(); await panel.goto(`${extension}/panel.html`);
  await fixture.bringToFront();
  const plan = [ ['CLICK', '#like'], ['SAVE_TEXT', '#text'], ['SAVE_IMAGE', '#cat'], ['SAVE_LINK', '#link'], ['CLICK', '#link'], ['GO_BACK'], ['SCROLL_DOWN'], ['SCROLL_UP'], ['DONE'] ];
  let index = 0, calls = [], lastRef;
  await context.route('https://api.typesafe.ai/v1/systemone', async route => {
    try {
      const req = route.request(); const body = req.postDataJSON();
      assert.equal(req.headers()['typesafe-danger-zone-images'], 'true');
      assert.equal(req.headers().authorization, 'Bearer mock-key-for-local-test');
      assert.equal(body.model, 'jev-preview');
      const image = body.assets[body.state.screenshot]; assert.equal(image.format, 'png'); assert.ok(image.data_base64.length > 100);
      assert.doesNotMatch(JSON.stringify(body), /"fingerprint"|"marks"|"frameUrl"/);
      assert.notEqual(body.state.screenshot, lastRef); lastRef = body.state.screenshot;
      const question = body.questions.next; calls.push(question.type);
      const [action, selector] = plan[index];
      if (index === 1 && question.type === 'choice') {
        const redPixels = await panel.evaluate(async base64 => {
          const bitmap = await createImageBitmap(await (await fetch(`data:image/png;base64,${base64}`)).blob());
          const canvas = document.createElement('canvas'); canvas.width = bitmap.width; canvas.height = bitmap.height;
          const ctx = canvas.getContext('2d'); ctx.drawImage(bitmap, 0, 0); bitmap.close();
          const pixels = ctx.getImageData(0, 0, canvas.width, canvas.height).data;
          let red = 0; for (let i = 0; i < pixels.length; i += 4) if (pixels[i] > 180 && pixels[i + 1] < 65 && pixels[i + 2] < 85) red++;
          return red;
        }, image.data_base64);
        assert.ok(redPixels > 100, 'The actual model screenshot must include the red action outline.');
        await writeFile(resolve(artifacts, 'model-screenshot.png'), Buffer.from(image.data_base64, 'base64'));
      }
      let answer;
      if (question.type === 'choice') {
        answer = { type: 'choice', choice: action, confidence: .99 };
        if (!selector) index++;
      } else {
        assert.equal(question.criteria, undefined); assert.match(question.instructions, new RegExp(action));
        const rect = await fixture.locator(selector).evaluate(el => {
          const r = el.getBoundingClientRect(); return { left: r.left / innerWidth, right: r.right / innerWidth, top: r.top / innerHeight, bottom: r.bottom / innerHeight };
        });
        answer = { type: 'box', box: rect, confidence: .99 }; index++;
      }
      await route.fulfill({ json: { answers: { next: answer } } });
    } catch (error) {
      console.error(error); await route.fulfill({ status: 500, json: { error: error.message } });
    }
  });
  // Dispatch the form without focusing its tab: this is the same extension page
  // as the sidebar, but testing in a tab leaves the fixture as capture target.
  await panel.locator('#task').fill('Like the comment, collect the cat text, image and link, visit the link then go back, scroll down and up, then finish.');
  await fixture.bringToFront();
  await panel.evaluate(() => document.getElementById('task-form').requestSubmit());
  await panel.waitForFunction(() => ['Done', 'Error'].includes(document.getElementById('status').textContent), null, { timeout: 45000 });
  const history = await panel.evaluate(() => JSON.parse(localStorage.getItem('jevbot.history.v1')));
  assert.equal(history[0].status, 'done', JSON.stringify(history[0], null, 2));
  assert.equal(history[0].results.length, 3); assert.equal(history[0].actions.length, 9);
  assert.deepEqual(calls, ['choice', 'box', 'choice', 'box', 'choice', 'box', 'choice', 'box', 'choice', 'box', 'choice', 'choice', 'choice', 'choice']);
  assert.equal(history[0].actions[4].status, 'success');
  assert.equal(history[0].marks.length, 5);
  await panel.setViewportSize({ width: 390, height: 900 });
  await panel.screenshot({ path: resolve(artifacts, 'panel-actions.png'), fullPage: true });
  await panel.locator('#results-tab').click();
  assert.equal(await panel.locator('#results li').count(), 3);
  const downloadPromise = panel.waitForEvent('download'); await panel.locator('#export').click();
  const download = await downloadPromise; await download.saveAs(resolve(artifacts, 'export.json'));
  assert.equal(JSON.parse(await readFile(resolve(artifacts, 'export.json'))).results.length, 3);
  await panel.screenshot({ path: resolve(artifacts, 'panel-results.png'), fullPage: true });
  await panel.reload(); await panel.locator('#history button').first().click();
  assert.match(await panel.locator('#task').inputValue(), /collect the cat/);
  assert.equal(await panel.locator('#results li').count(), 3);
  console.log(`Extension integration: ${calls.length} mocked Jev calls, all 8 actions, navigation, screenshot captures, settings persistence, history restore, results and JSON export passed.`);
  await context.unroute('https://api.typesafe.ai/v1/systemone');
  const repeatSequence = [ 'SAVE_IMAGE', 'box', 'SAVE_IMAGE', 'box', 'RETRY_ONCE', 'SAVE_IMAGE', 'box', 'DONE' ];
  await context.route('https://api.typesafe.ai/v1/systemone', async route => {
    const event = repeatSequence.shift(); const question = route.request().postDataJSON().questions.next;
    let answer;
    if (event === 'box') {
      assert.equal(question.type, 'box');
      const box = await fixture.locator('#cat').evaluate(el => { const r = el.getBoundingClientRect(); return { left: r.left / innerWidth, right: r.right / innerWidth, top: r.top / innerHeight, bottom: r.bottom / innerHeight }; });
      answer = { type: 'box', box };
    } else {
      assert.equal(question.type, 'choice');
      if (event === 'RETRY_ONCE') assert.ok(question.criteria.RETRY_ONCE);
      answer = { type: 'choice', choice: event };
    }
    await route.fulfill({ json: { answers: { next: answer } } });
  });
  await fixture.bringToFront();
  await panel.evaluate(() => document.getElementById('task-form').requestSubmit());
  await panel.waitForFunction(() => document.getElementById('action-count').textContent === '4' && document.getElementById('status').textContent === 'Done');
  const repeated = await panel.evaluate(() => JSON.parse(localStorage.getItem('jevbot.history.v1'))[0]);
  assert.equal(repeatSequence.length, 0); assert.equal(repeated.results.length, 1);
  assert.equal(repeated.actions[1].guard.decision, 'RETRY_ONCE');
  assert.equal(repeated.actions[2].result.retryAllowed, false); assert.equal(repeated.actions[2].status, 'failed');
  assert.equal(repeated.marks.length, 1); assert.equal(repeated.marks[0].retries, 1);
  console.log('Actual extension repeat sequence: marked screenshot sent to Jev, one explicit retry, further repeat blocked, descriptors kept local passed.');
  // Exercise termination through the actual panel and storage rather than only
  // through the pure loop harness.
  await context.unroute('https://api.typesafe.ai/v1/systemone');
  await context.route('https://api.typesafe.ai/v1/systemone', route => route.fulfill({ json: { answers: { next: { type: 'choice', choice: 'SCROLL_DOWN' } } } }));
  await panel.locator('#max-steps').fill('1'); await fixture.bringToFront();
  await panel.evaluate(() => document.getElementById('task-form').requestSubmit());
  await panel.waitForFunction(() => document.getElementById('status').textContent === 'Step limit');
  assert.match(await panel.locator('#notice').textContent(), /Step limit/);

  await context.unroute('https://api.typesafe.ai/v1/systemone');
  let releaseBox, reachedBox;
  const boxReached = new Promise(resolve => { reachedBox = resolve; });
  const boxReleased = new Promise(resolve => { releaseBox = resolve; });
  await context.route('https://api.typesafe.ai/v1/systemone', async route => {
    if (route.request().postDataJSON().questions.next.type === 'choice') {
      await route.fulfill({ json: { answers: { next: { type: 'choice', choice: 'CLICK' } } } });
    } else {
      reachedBox(); await boxReleased;
      await route.fulfill({ json: { answers: { next: { type: 'box', box: { left: .1, right: .2, top: .1, bottom: .2 } } } } }).catch(() => {});
    }
  });
  await panel.locator('#max-steps').fill('10'); await fixture.bringToFront();
  await panel.evaluate(() => document.getElementById('task-form').requestSubmit());
  await boxReached;
  await panel.evaluate(() => document.getElementById('stop').click());
  await panel.waitForFunction(() => document.getElementById('status').textContent === 'Stopped');
  releaseBox();
  const stopped = await panel.evaluate(() => JSON.parse(localStorage.getItem('jevbot.history.v1'))[0]);
  assert.equal(stopped.actions.length, 1); assert.equal(stopped.actions[0].status, 'interrupted');
  assert.equal(stopped.actions[0].point, undefined);
  // Simulate abrupt panel destruction leaving a persisted pending intent.
  await panel.evaluate(() => {
    const history = JSON.parse(localStorage.getItem('jevbot.history.v1'));
    history[0].status = 'running'; history[0].actions[0].status = 'executing';
    localStorage.setItem('jevbot.history.v1', JSON.stringify(history));
  });
  await panel.reload();
  await panel.waitForFunction(() => document.getElementById('status').textContent === 'Interrupted');
  console.log('Termination: actual panel step limit, Stop during pending Box, and interrupted-intent recovery passed.');
  await panel.locator('#clear-history').click();
  await panel.waitForFunction(() => localStorage.getItem('jevbot.history.v1') === null);
  assert.equal(await fixture.locator('[data-jevbot-marks]').count(), 0);
  console.log(`Screenshots/export: ${artifacts}`);
} finally {
  await context?.close(); await new Promise(r => server.close(r));
  // Only the temporary test profile is removed; preserve screenshots and export.
  const cleanupTarget = resolve(artifacts, 'browser-profile');
  if (cleanupTarget !== resolve(root, '.artifacts', 'browser-profile') || !cleanupTarget.startsWith(root)) throw new Error('Refusing test-profile cleanup outside the extension workspace.');
  await rm(cleanupTarget, { recursive: true, force: true });
}
