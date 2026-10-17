import test from 'node:test';
import assert from 'node:assert/strict';
import { imageLayout, boxPoint, validateBox, requestBody, choiceQuestion, runLoop } from '../core.js';
import { askJev, ENDPOINT } from '../api.js';
import { createRun, saveRun, loadHistory, HISTORY_KEY } from '../history.js';

const frame = { base64: 'ZmFrZQ==', page: { url: 'https://fixture.test', width: 1000, height: 800 }, image: { width: 1250, height: 1000, content: { x: 0, y: 0, width: 1250, height: 1000 } } };
const box = { left: .1, right: .3, top: .2, bottom: .4 };
const choice = action => ({ type: 'choice', choice: action });
const target = { type: 'box', box };
function harness(answers, overrides = {}) {
  const requests = [], acts = [], commits = [];
  const deps = {
    snapshot: async () => frame,
    assertCurrent: async () => {},
    ask: async body => { requests.push(body); return answers.shift(); },
    act: async (action, point) => { acts.push({ action, point }); return { ok: true, value: 'https://fixture.test/cat.png', message: 'Saved image.' }; },
    persist: async run => commits.push(structuredClone(run)), phase: () => {}, ...overrides
  };
  return { deps, requests, acts, commits };
}
const run = (steps = 10) => createRun('Find every cat image', steps, { id: 1, windowId: 2, url: frame.page.url }, 'jev-preview');

test('Choice -> separate Box -> action -> DONE, including history and saved results', async () => {
  const h = harness([choice('SAVE_IMAGE'), target, choice('DONE')]);
  const result = await runLoop(run(), h.deps);
  assert.equal(result.status, 'done'); assert.equal(result.results.length, 1);
  assert.deepEqual(h.requests.map(r => r.questions.next.type), ['choice', 'box', 'choice']);
  assert.equal(h.requests[1].questions.next.criteria, undefined);
  assert.match(h.requests[1].questions.next.instructions, /SAVE_IMAGE/);
  assert.equal(h.acts[0].action, 'SAVE_IMAGE');
  assert.equal(h.acts[0].point.x, 200); assert.ok(Math.abs(h.acts[0].point.y - 240) < 1e-8);
  assert.equal(h.requests[2].state.history[0].status, 'success');
  assert.ok(h.commits.some(r => r.actions[0]?.status === 'executing'));
  assert.equal(h.requests[0].model, 'jev-preview');
});
test('each targeted action requires Box, non-targeted actions never request Box', async () => {
  for (const action of ['CLICK', 'SAVE_TEXT', 'SAVE_LINK', 'SAVE_IMAGE', 'SCROLL_DOWN', 'SCROLL_UP', 'GO_BACK']) {
    const targeted = action.startsWith('SAVE') || action === 'CLICK';
    const h = harness([choice(action), ...(targeted ? [target] : []), choice('DONE')]);
    await runLoop(run(), h.deps);
    assert.equal(h.requests.filter(r => r.questions.next.type === 'box').length, targeted ? 1 : 0, action);
  }
});
test('DONE performs no page mutation; step cap ends with limit', async () => {
  const h = harness([choice('DONE')]);
  assert.equal((await runLoop(run(), h.deps)).status, 'done'); assert.equal(h.acts.length, 0);
  const bounded = harness([choice('SCROLL_DOWN'), choice('SCROLL_DOWN')]);
  assert.equal((await runLoop(run(2), bounded.deps)).status, 'limit'); assert.equal(bounded.acts.length, 2);
});
test('Box validates fractional edges and correctly maps device-pixel/padded images', () => {
  for (const invalid of [null, {}, { ...box, left: -1 }, { ...box, right: NaN }, { ...box, top: .9 }, { ...box, right: 2 }]) assert.throws(() => validateBox(invalid));
  const layout = imageLayout(4000, 200); assert.equal(layout.width, 1568); assert.equal(layout.height, 280);
  const padded = { image: layout, page: { width: 2000, height: 100 } };
  assert.throws(() => boxPoint({ left: .1, right: .3, top: .8, bottom: 1 }, padded), /padding/);
  assert.deepEqual(imageLayout(100, 100), { width: 280, height: 280, content: { x: 0, y: 0, width: 100, height: 100 } });
});
test('assets have UUID references and raw PNG base64', () => {
  const body = requestBody(run(), frame, choiceQuestion());
  assert.equal(Object.keys(body.assets).length, 1);
  assert.equal(body.assets[body.state.screenshot].data_base64, frame.base64);
  assert.equal(body.assets[body.state.screenshot].format, 'png');
  assert.notEqual(body.state.screenshot, requestBody(run(), frame, choiceQuestion()).state.screenshot);
});
test('API uses required header, bearer auth, fixed origin, and exact answer field', async () => {
  let captured;
  const answer = await askJev({ questions: {} }, 'secret', undefined, async (url, init) => {
    captured = { url, init }; return { ok: true, json: async () => ({ answers: { next: choice('DONE') } }) };
  });
  assert.equal(captured.url, ENDPOINT); assert.equal(captured.init.headers['TypeSafe-Danger-Zone-Images'], 'true');
  assert.equal(captured.init.headers.Authorization, 'Bearer secret'); assert.equal(answer.choice, 'DONE');
  await assert.rejects(askJev({}, 'secret', undefined, async () => ({ ok: false, status: 401, text: () => 'secret' })), e => /401/.test(e.message) && !e.message.includes('secret'));
  await assert.rejects(askJev({}, 'secret', undefined, async () => ({ ok: true, json: async () => ({}) })), /answers.next/);
});
test('stop after Choice aborts before Box or action', async () => {
  const controller = new AbortController();
  const h = harness([], { ask: async () => { controller.abort(); return choice('CLICK'); } });
  assert.equal((await runLoop(run(), h.deps, controller.signal)).status, 'stopped'); assert.equal(h.acts.length, 0);
});
test('stop during Box never dispatches action', async () => {
  const controller = new AbortController(); let n = 0;
  const h = harness([], { ask: async () => { if (++n === 1) return choice('CLICK'); controller.abort(); return target; } });
  const result = await runLoop(run(), h.deps, controller.signal);
  assert.equal(result.status, 'stopped'); assert.equal(result.actions[0].status, 'interrupted'); assert.equal(h.acts.length, 0);
});
test('stale viewport is recorded as a failed observation and never clicked', async () => {
  const h = harness([choice('CLICK'), target, choice('DONE')], { assertCurrent: async () => { throw new Error('stale viewport'); } });
  const result = await runLoop(run(), h.deps);
  assert.equal(result.actions[0].status, 'failed'); assert.equal(h.acts.length, 0); assert.equal(result.status, 'done');
});
test('three consecutive failures stop the loop; unknown actions fail closed', async () => {
  const h = harness([choice('SCROLL_DOWN'), choice('SCROLL_DOWN'), choice('SCROLL_DOWN')], { act: async () => ({ ok: false, message: 'Blocked' }) });
  assert.equal((await runLoop(run(), h.deps)).status, 'error');
  const invalid = harness([choice('__proto__')]);
  assert.equal((await runLoop(run(), invalid.deps)).status, 'error'); assert.equal(invalid.acts.length, 0);
});
test('invalid Box fails without performing a page action', async () => {
  const h = harness([choice('CLICK'), { type: 'box', box: { ...box, left: 2 } }, choice('DONE')]);
  const result = await runLoop(run(), h.deps); assert.equal(result.actions[0].status, 'failed'); assert.equal(h.acts.length, 0);
});
test('a Box in padding fails cleanly without persisting incomplete coordinates', async () => {
  const h = harness([choice('CLICK'), { type: 'box', box: { left: .1, right: .3, top: .8, bottom: 1 } }, choice('DONE')], {
    snapshot: async () => ({ ...frame, image: imageLayout(4000, 200) })
  });
  const result = await runLoop(run(), h.deps);
  assert.equal(result.actions[0].status, 'failed'); assert.equal(result.actions[0].box, undefined); assert.equal(h.acts.length, 0);
});
test('an uncertain click outcome stops the loop without replaying the click', async () => {
  let count = 0;
  const h = harness([choice('CLICK'), target], { act: async () => { count++; throw Object.assign(new Error('Outcome uncertain'), { fatal: true }); } });
  const result = await runLoop(run(), h.deps);
  assert.equal(count, 1); assert.equal(result.status, 'error'); assert.equal(result.actions[0].status, 'interrupted');
});
test('duplicate saved URLs are retained once and reported in action history', async () => {
  const h = harness([choice('SAVE_IMAGE'), target, choice('SAVE_IMAGE'), target, choice('DONE')]);
  const result = await runLoop(run(), h.deps);
  assert.equal(result.results.length, 1); assert.equal(result.actions[1].result.duplicate, true);
});
test('history persists runs and results, updates without duplicates, limits old requests', () => {
  const values = new Map(); const storage = { getItem: k => values.get(k), setItem: (k, v) => values.set(k, v) };
  const item = run(); saveRun(item, storage); item.status = 'done'; saveRun(item, storage);
  assert.equal(loadHistory(storage).length, 1); assert.equal(loadHistory(storage)[0].status, 'done');
  for (let n = 0; n < 45; n++) saveRun(run(), storage);
  assert.equal(loadHistory(storage).length, 40);
  values.set(HISTORY_KEY, '{bad'); assert.throws(() => loadHistory(storage), /unreadable/);
});
test('request validation rejects empty instructions and noninteger limits', () => {
  assert.throws(() => createRun(' ', 2, {}, ''), /instruction/);
  assert.throws(() => createRun('a', 2.1, {}, ''), /Step limit/);
});

test('mark descriptors stay local and screenshot instructions explain operation-specific marks', () => {
  const item = run(); item.marks.push({ fingerprint: 'secret internal descriptor' });
  item.actions.push({ action: 'CLICK', result: { ok: true, mark: { fingerprint: 'also internal' } } });
  const serialized = JSON.stringify(requestBody(item, frame, choiceQuestion()));
  assert.doesNotMatch(serialized, /secret internal descriptor|also internal|fingerprint/);
  assert.match(serialized, /Red outlines/); assert.match(serialized, /Different operations/);
});
test('blocked repeats ask a separate Choice and CHOOSE_ANOTHER never dispatches a retry', async () => {
  let attempts = 0;
  const h = harness([choice('CLICK'), target, choice('CHOOSE_ANOTHER'), choice('DONE')], {
    act: async () => { attempts++; return { ok: false, blocked: true, retryAllowed: true, message: 'Already clicked.' }; }
  });
  const result = await runLoop(run(), h.deps);
  assert.equal(result.status, 'done'); assert.equal(attempts, 1);
  assert.equal(result.actions[0].guard.decision, 'CHOOSE_ANOTHER');
  assert.deepEqual(Object.keys(h.requests[2].questions.next.criteria), ['CHOOSE_ANOTHER', 'RETRY_ONCE']);
});
test('explicit RETRY_ONCE goes through current-frame check and carries a retry flag', async () => {
  const optionsSeen = []; let checks = 0;
  const h = harness([choice('CLICK'), target, choice('RETRY_ONCE'), choice('DONE')], {
    assertCurrent: async () => { checks++; },
    act: async (action, point, frame, signal, options) => {
      optionsSeen.push(options.allowRepeat);
      if (!options.allowRepeat) return { ok: false, blocked: true, retryAllowed: true, message: 'Already clicked.' };
      const mark = { id: 'm', fingerprint: 'internal' };
      await options.beforeAction(mark);
      return { ok: true, mark, message: 'Explicit retry dispatched.' };
    }
  });
  const result = await runLoop(run(), h.deps);
  assert.equal(result.status, 'done'); assert.deepEqual(optionsSeen, [undefined, true]); assert.equal(checks, 2);
  assert.equal(result.marks[0].id, 'm'); assert.equal(result.actions[0].result.mark, undefined);
  assert.ok(h.commits.some(r => r.marks.length && r.actions[0].status === 'executing'));
});
test('exhausted retry budget does not ask another retry question', async () => {
  const h = harness([choice('SAVE_IMAGE'), target, choice('DONE')], {
    act: async () => ({ ok: false, blocked: true, retryAllowed: false, message: 'Retry exhausted.' })
  });
  assert.equal((await runLoop(run(), h.deps)).status, 'done'); assert.equal(h.requests.length, 3);
});
test('stop during retry Choice prevents an additional operation', async () => {
  const controller = new AbortController(); let calls = 0, acts = 0;
  const h = harness([], {
    ask: async () => { calls++; if (calls === 1) return choice('CLICK'); if (calls === 2) return target; controller.abort(); return choice('RETRY_ONCE'); },
    act: async () => { acts++; return { ok: false, blocked: true, retryAllowed: true }; }
  });
  assert.equal((await runLoop(run(), h.deps, controller.signal)).status, 'stopped'); assert.equal(acts, 1);
});
test('a known failed click rolls back its persisted intent without leaving a mark', async () => {
  const h = harness([choice('CLICK'), target, choice('DONE')], {
    act: async (action, point, frame, signal, options) => {
      await options.beforeAction({ id: 'not-dispatched', state: 'attempted' });
      return { ok: false, message: 'Target changed before dispatch.' };
    }
  });
  const result = await runLoop(run(), h.deps);
  assert.equal(result.marks.length, 0); assert.equal(result.actions[0].status, 'failed');
});
