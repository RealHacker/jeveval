import { runLoop } from './core.js';
import { askJev } from './api.js';
import { activeTab, browserDriver, clearPageMarks } from './browser.js';
import { createRun, loadHistory, saveRun, HISTORY_KEY } from './history.js';
import { loadSettings } from './settings.js';

const $ = id => document.getElementById(id);
let current = null, controller = null, busy = false, phase = '';
const names = { running: 'Running', done: 'Done', limit: 'Step limit', stopped: 'Stopped', error: 'Error', interrupted: 'Interrupted' };
const node = (tag, text, className) => {
  const el = document.createElement(tag); el.textContent = text;
  if (className) el.className = className;
  return el;
};
function notice(message = '') { $('notice').textContent = message; $('notice').hidden = !message; }
function controls() {
  $('run').disabled = busy; $('stop').hidden = !busy;
  $('task').disabled = busy; $('max-steps').disabled = busy;
  $('clear-history').disabled = busy;
}
function renderHistory() {
  const list = $('history'); list.replaceChildren();
  for (const run of loadHistory()) {
    const li = node('li', ''); const button = node('button', ''); button.type = 'button'; button.disabled = busy;
    button.setAttribute('aria-current', String(run.id === current?.id));
    button.append(node('span', run.task, 'history-task'), node('small', `${new Date(run.createdAt).toLocaleString()} · ${names[run.status] || run.status} · ${run.results.length} results`));
    button.addEventListener('click', () => { current = run; $('task').value = run.task; $('max-steps').value = run.maxSteps; phase = ''; notice(run.error || ''); render(); });
    li.append(button); list.append(li);
  }
  if (!list.children.length) list.append(node('li', 'Your requests stay here, on this device.', 'hint'));
}
function render() {
  controls();
  $('status').textContent = current ? names[current.status] || current.status : 'Ready';
  $('status').dataset.status = current?.status || 'ready';
  $('phase').textContent = phase || (current ? `${current.actions.length} / ${current.maxSteps} steps · ${current.results.length} collected` : 'Give Jev a task to get started.');
  $('action-count').textContent = current?.actions.length || 0;
  $('result-count').textContent = current?.results.length || 0;
  $('actions').replaceChildren(); $('results').replaceChildren();
  for (const step of current?.actions || []) {
    const li = node('li', ''); li.dataset.state = step.status;
    li.append(node('span', step.number, 'step-number'));
    const title = node('div', '', 'step-title'); title.append(node('span', step.action.replaceAll('_', ' ')), node('span', step.status, 'step-state'));
    li.append(title, node('p', `${step.result?.message || (step.box ? 'Target located.' : 'Waiting for target…')}${step.result?.duplicate ? ' Already collected.' : ''}`, 'step-message'));
    if (step.guard) li.append(node('p', `Repeat guard: ${step.guard.decision === 'RETRY_ONCE' ? 'Jev explicitly chose one retry.' : step.guard.decision === 'CHOOSE_ANOTHER' ? 'Jev chose to move on.' : 'checking repeat request.'}`, 'step-message'));
    if (step.box && step.point) {
      const detail = node('details', ''); detail.append(node('summary', 'Target coordinates', 'hint'));
      detail.append(node('p', `Box ${JSON.stringify(step.box)} · CSS point (${step.point.x.toFixed(1)}, ${step.point.y.toFixed(1)})`, 'step-message')); li.append(detail);
    }
    $('actions').append(li);
  }
  for (const result of current?.results || []) {
    const li = node('li', ''); li.append(node('span', result.type, 'result-type'));
    if (result.type !== 'text' && /^https?:\/\//i.test(result.value)) {
      const link = node('a', result.value, 'result-value'); link.href = result.value; link.target = '_blank'; link.rel = 'noopener noreferrer'; li.append(link);
    } else li.append(node('span', result.value, 'result-value'));
    li.append(node('div', `Step ${result.step} · ${result.sourceUrl}`, 'result-source'));
    $('results').append(li);
  }
  $('actions-empty').hidden = !!current?.actions.length;
  $('results-empty').hidden = !!current?.results.length;
  $('export').disabled = !current || busy;
  renderHistory();
}
function switchView(view) {
  for (const key of ['actions', 'results']) {
    const selected = key === view;
    $(`${key}-view`).hidden = !selected;
    $(`${key}-tab`).classList.toggle('selected', selected);
    $(`${key}-tab`).setAttribute('aria-pressed', String(selected));
  }
}
$('actions-tab').addEventListener('click', () => switchView('actions'));
$('results-tab').addEventListener('click', () => switchView('results'));
$('settings').addEventListener('click', () => chrome.runtime.openOptionsPage().catch(e => notice(e.message)));
$('stop').addEventListener('click', () => { controller?.abort(); phase = 'Stopping…'; render(); });
window.addEventListener('pagehide', () => controller?.abort());

$('task-form').addEventListener('submit', async event => {
  event.preventDefault(); if (busy) return;
  busy = true; controls(); notice('');
  try {
    await navigator.locks.request('jevbot.runner', { ifAvailable: true }, async lock => {
      if (!lock) throw new Error('JevBot is running in another sidebar. Stop that run first.');
      const settings = await loadSettings();
      if (!settings.apiKey) throw new Error('Add your TypeSafe API key in Settings before running a task.');
      const tab = await activeTab();
      current = createRun($('task').value, Number($('max-steps').value), tab, settings.model);
      controller = new AbortController();
      switchView('actions');
      const driver = browserDriver(current, settings);
      await runLoop(current, {
        ...driver,
        ask: (body, signal) => askJev(body, settings.apiKey, signal),
        persist: run => { saveRun(run); render(); },
        phase: value => { phase = value; render(); }
      }, controller.signal);
      notice(current.error || (current.status === 'limit' ? 'Step limit reached. Review the page and actions before running again.' : ''));
    });
  } catch (error) { notice(error.message); }
  finally { controller = null; busy = false; phase = ''; render(); }
});
$('clear-history').addEventListener('click', async () => {
  if (busy) return;
  try {
    await navigator.locks.request('jevbot.runner', { ifAvailable: true }, async lock => {
      if (!lock) throw new Error('Stop the run in the other sidebar before clearing history.');
      await clearPageMarks(loadHistory());
      localStorage.removeItem(HISTORY_KEY); current = null; notice(''); render();
    });
  }
  catch (error) { notice(error.message); }
});
$('export').addEventListener('click', () => {
  if (!current) return;
  const url = URL.createObjectURL(new Blob([JSON.stringify(current, null, 2)], { type: 'application/json' }));
  const link = node('a', ''); link.href = url; link.download = `jevbot-${current.id}.json`; link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
});
window.addEventListener('storage', event => {
  if (event.key === HISTORY_KEY && !busy) {
    try { current = loadHistory().find(r => r.id === current?.id) || loadHistory()[0] || null; render(); }
    catch (error) { notice(error.message); }
  }
});
try {
  // A lock is released automatically when its panel closes. Never resume/replay.
  await navigator.locks.request('jevbot.runner', { ifAvailable: true }, lock => {
    if (!lock) return;
    for (const run of loadHistory().filter(r => r.status === 'running')) {
      run.status = 'interrupted'; run.error = 'Sidebar closed or reloaded. Review previous actions before rerunning.';
      const pending = run.actions.at(-1);
      if (pending && ['chosen', 'executing'].includes(pending.status)) pending.status = 'interrupted';
      saveRun(run);
    }
  });
  current = loadHistory()[0] || null;
  if (current?.error) notice(current.error);
  render();
} catch (error) { notice(error.message); }
