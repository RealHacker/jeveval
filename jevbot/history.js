export const HISTORY_KEY = 'jevbot.history.v1';
export const MAX_RUNS = 40;
export function loadHistory(storage = localStorage) {
  const raw = storage.getItem(HISTORY_KEY);
  if (!raw) return [];
  let value;
  try { value = JSON.parse(raw); } catch { throw new Error('Saved history is unreadable. Export or clear local extension storage before continuing.'); }
  if (!Array.isArray(value) || value.some(r => !r || typeof r.id !== 'string' || !Array.isArray(r.actions) || !Array.isArray(r.results))) throw new Error('Saved history has an invalid format.');
  return value;
}
export function saveRun(run, storage = localStorage) {
  const history = loadHistory(storage).filter(r => r.id !== run.id);
  history.unshift(run);
  // Prune oldest entries before hitting the browser's localStorage quota.
  while (history.length > MAX_RUNS || (JSON.stringify(history).length > 1800000 && history.length > 1)) history.pop();
  try { storage.setItem(HISTORY_KEY, JSON.stringify(history)); }
  catch { throw new Error('Cannot save request history: local storage is full or unavailable. Export/delete older runs before continuing.'); }
}
export function createRun(task, maxSteps, tab, model) {
  const text = task.trim();
  if (!text || text.length > 8000) throw new Error('Enter an instruction of 1–8,000 characters.');
  if (!Number.isInteger(maxSteps) || maxSteps < 1 || maxSteps > 200) throw new Error('Step limit must be between 1 and 200.');
  return { id: crypto.randomUUID(), task: text, maxSteps, model, tabId: tab.id, windowId: tab.windowId, startUrl: tab.url,
    createdAt: Date.now(), updatedAt: Date.now(), status: 'running', actions: [], results: [], marks: [] };
}
