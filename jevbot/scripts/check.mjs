import { readdir, readFile } from 'node:fs/promises';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
const root = new URL('../', import.meta.url);
for (const file of (await readdir(root)).filter(f => f.endsWith('.js'))) {
  const result = spawnSync(process.execPath, ['--check', fileURLToPath(new URL(file, root))], { encoding: 'utf8' });
  if (result.status) { process.stderr.write(result.stderr); process.exit(1); }
}
const manifest = JSON.parse(await readFile(new URL('manifest.json', root)));
for (const path of [manifest.background.service_worker, manifest.side_panel.default_path, manifest.options_ui.page]) await readFile(new URL(path, root));
console.log('Module syntax and manifest entry points passed.');
