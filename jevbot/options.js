import { loadSettings, saveSettings } from './settings.js';
const $ = id => document.getElementById(id);
function error(message) { $('notice').textContent = message; $('notice').hidden = !message; }
try {
  const settings = await loadSettings();
  $('api-key').value = settings.apiKey; $('model').value = settings.model; $('settle-ms').value = settings.settleMs;
} catch (e) { error(e.message); }
$('settings-form').addEventListener('submit', async event => {
  event.preventDefault(); error(''); $('saved').textContent = '';
  try {
    await saveSettings({ apiKey: $('api-key').value, model: $('model').value, settleMs: $('settle-ms').value });
    $('saved').textContent = 'Saved on this device.';
  } catch (e) { error(e.message); }
});
