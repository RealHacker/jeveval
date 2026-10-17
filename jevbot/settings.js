import { DEFAULTS } from './core.js';
export async function loadSettings() {
  const { settings } = await chrome.storage.local.get('settings');
  return { ...DEFAULTS, ...settings };
}
export async function saveSettings(settings) {
  const value = { apiKey: settings.apiKey.trim(), model: settings.model.trim(), settleMs: Number(settings.settleMs) };
  if (!value.model || value.model.length > 120) throw new Error('Enter a valid model name.');
  if (!Number.isInteger(value.settleMs) || value.settleMs < 500 || value.settleMs > 10000) throw new Error('Wait duration must be 500–10,000 milliseconds.');
  await chrome.storage.local.setAccessLevel({ accessLevel: 'TRUSTED_CONTEXTS' });
  await chrome.storage.local.set({ settings: value });
  return value;
}
