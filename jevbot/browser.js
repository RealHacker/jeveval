import { imageLayout, checkAbort, TARGETED } from './core.js';
import { pageOperation } from './page.js';
import { syncPageMarks } from './marks.js';

function fatal(message) { return Object.assign(new Error(message), { fatal: true }); }
export function wait(ms, signal) {
  return new Promise((resolve, reject) => {
    checkAbort(signal);
    const abort = () => { clearTimeout(timer); reject(new DOMException('Run stopped.', 'AbortError')); };
    const timer = setTimeout(() => { signal?.removeEventListener('abort', abort); resolve(); }, ms);
    signal?.addEventListener('abort', abort, { once: true });
  });
}

export async function activeTab() {
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  if (!tab || !/^https?:\/\//i.test(tab.url || '')) throw new Error('Open a regular HTTP(S) webpage first. Chrome internal pages are unsupported.');
  return tab;
}

export function browserDriver(run, settings) {
  const target = { tabId: run.tabId };
  async function active() {
    let tab;
    try { tab = await chrome.tabs.get(run.tabId); } catch { throw fatal('The target tab was closed.'); }
    const window = await chrome.windows.get(run.windowId);
    if (!tab.active || tab.windowId !== run.windowId || !window.focused) throw fatal('The target tab or window is no longer active. Return to it and start a new run.');
    if (!/^https?:\/\//i.test(tab.url || '')) throw fatal('Navigation reached an unsupported page.');
    return tab;
  }
  async function inject(action, point, expected, documentId, context = {}) {
    const results = await chrome.scripting.executeScript({
      target: documentId ? { tabId: run.tabId, documentIds: [documentId] } : target,
      func: pageOperation, args: [action, point || null, expected || null, context]
    });
    if (!results[0]?.result) throw new Error('The page did not return an action result.');
    return { value: results[0].result, documentId: results[0].documentId };
  }
  async function syncMarks() {
    await chrome.scripting.executeScript({ target, func: syncPageMarks, args: [run.id, run.marks || []] });
  }
  async function metadata() {
    try { return await inject('META'); }
    catch { throw fatal('Cannot access this page. Chrome Web Store/internal pages and some embedded documents block extensions.'); }
  }
  function same(a, b) {
    return a.documentId === b.documentId && ['url', 'width', 'height', 'scrollX', 'scrollY', 'visualScale'].every(k => a.value[k] === b.value[k]);
  }
  async function assertCurrent(frame) {
    await active();
    if (!same({ value: frame.page, documentId: frame.documentId }, await metadata())) throw new Error('Page/viewport changed while Jev was answering. Taking a fresh screenshot on the next step.');
  }
  return {
    assertCurrent,
    async snapshot(signal) {
      const deadline = Date.now() + 20000;
      while ((await active()).status === 'loading') {
        if (Date.now() >= deadline) throw fatal('Page load exceeded 20 seconds. Wait for it to finish and rerun.');
        await wait(200, signal);
      }
      await wait(settings.settleMs, signal);
      await active();
      await syncMarks();
      checkAbort(signal);
      const before = await metadata();
      if (before.value.visualScale !== 1) throw fatal('Reset pinch zoom before running JevBot. Browser page zoom is supported.');
      const data = await chrome.tabs.captureVisibleTab(run.windowId, { format: 'png' });
      checkAbort(signal);
      await active();
      if (!same(before, await metadata())) throw new Error('Page changed during capture. Wait for the page to settle and rerun.');
      const bitmap = await createImageBitmap(await (await fetch(data)).blob());
      try {
        const layout = imageLayout(bitmap.width, bitmap.height);
        const canvas = document.createElement('canvas');
        canvas.width = layout.width; canvas.height = layout.height;
        const ctx = canvas.getContext('2d');
        ctx.fillStyle = '#ffffff'; ctx.fillRect(0, 0, canvas.width, canvas.height);
        ctx.drawImage(bitmap, 0, 0, layout.content.width, layout.content.height);
        return { page: before.value, documentId: before.documentId, image: layout, base64: canvas.toDataURL('image/png').split(',')[1] };
      } finally { bitmap.close(); }
    },
    async act(action, point, frame, signal, options = {}) {
      checkAbort(signal);
      await active();
      checkAbort(signal);
      if (action === 'GO_BACK') {
        try { await chrome.tabs.goBack(run.tabId); return { ok: true, message: 'Back navigation requested; verify the next screenshot.' }; }
        catch { return { ok: false, message: 'No accessible previous history entry.' }; }
      }
      if (!TARGETED.has(action)) return (await inject(action, point, frame.page, frame.documentId)).value;
      const context = { runId: run.id, step: run.actions.at(-1).number, action, allowRepeat: !!options.allowRepeat };
      const prepared = (await inject('PREPARE', point, frame.page, frame.documentId, context)).value;
      if (!prepared.ok) return prepared;
      if (action === 'CLICK') await options.beforeAction?.(prepared.mark);
      checkAbort(signal);
      await assertCurrent(frame);
      checkAbort(signal);
      try { return (await inject(action, point, frame.page, frame.documentId, { ...context, prepared: prepared.mark })).value; }
      catch (error) {
        // Navigating clicks can destroy the script context before the result returns.
        if (action === 'CLICK') throw fatal('Click outcome is uncertain because its page context was lost. Inspect the page before rerunning; the click will not be retried.');
        throw error;
      }
    }
  };
}

export async function clearPageMarks(runs) {
  for (const tabId of new Set(runs.map(run => run.tabId))) {
    try {
      await chrome.scripting.executeScript({ target: { tabId }, func: syncPageMarks, args: [null] });
    } catch { /* Closed/restricted tabs have no removable overlay. */ }
  }
}
