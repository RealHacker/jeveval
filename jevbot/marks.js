// Runs in Chrome's isolated scripting world. Nothing here is sent as DOM state
// to Jev; the model sees the resulting red outlines in its screenshots.
export function syncPageMarks(runId, records = []) {
  const key = '__jevbotVisualMemoryV1';
  const old = globalThis[key];
  if (old && old.runId !== runId) { old.destroy(); delete globalThis[key]; }
  if (!runId) return { ok: true };
  let memory = globalThis[key];
  if (!memory) {
    const host = document.createElement('div');
    host.setAttribute('data-jevbot-marks', ''); host.setAttribute('aria-hidden', 'true');
    host.style.cssText = 'all:initial!important;position:fixed!important;inset:0!important;width:0!important;height:0!important;pointer-events:none!important;z-index:2147483647!important;';
    const shadow = host.attachShadow({ mode: 'closed' });
    // CSSOM properties also work on pages that disallow inline <style> blocks.
    // The closed shadow tree keeps the site's selectors away from these nodes.
    document.documentElement.append(host);
    const bindings = new Map();
    const onPageHide = () => { memory.destroy(); if (globalThis[key] === memory) delete globalThis[key]; };
    window.addEventListener('pagehide', onPageHide, { once: true });
    const text = el => (el.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 1000);
    const fingerprint = el => JSON.stringify({ tag: el.tagName, text: text(el), href: el.href || '',
      image: el.currentSrc || el.src || el.querySelector?.('img')?.currentSrc || el.querySelector?.('img')?.src || '',
      role: el.getAttribute('role') || '', aria: el.getAttribute('aria-label') || '',
      pressed: el.getAttribute('aria-pressed') || '', checked: el.checked ?? '', type: el.getAttribute('type') || '' });
    const frameUrl = el => el.ownerDocument.location.href;
    const eligible = record => record.pageUrl === location.href && ['attempted', 'dispatched', 'saved'].includes(record.state);
    function bounds(el) {
      let r = el.getBoundingClientRect();
      let rect = { left: r.left, top: r.top, width: r.width, height: r.height };
      let win = el.ownerDocument.defaultView;
      while (win && win !== window) {
        const frame = win.frameElement;
        if (!frame) return null;
        r = frame.getBoundingClientRect();
        const sx = r.width / frame.offsetWidth || 1, sy = r.height / frame.offsetHeight || 1;
        rect = { left: r.left + (frame.clientLeft + rect.left) * sx, top: r.top + (frame.clientTop + rect.top) * sy, width: rect.width * sx, height: rect.height * sy };
        win = frame.ownerDocument.defaultView;
      }
      return rect;
    }
    function candidates() {
      const result = [], roots = [document]; let count = 0;
      for (let i = 0; i < roots.length; i++) {
        for (const el of roots[i].querySelectorAll('*')) {
          if (el === host) continue;
          if (++count > 20000) return result;
          result.push(el);
          if (el.shadowRoot) roots.push(el.shadowRoot);
          if (el.tagName === 'IFRAME') { try { if (el.contentDocument) roots.push(el.contentDocument); } catch {} }
        }
      }
      return result;
    }
    function valid(binding) {
      try { return binding.element?.isConnected && frameUrl(binding.element) === binding.record.frameUrl && fingerprint(binding.element) === binding.record.fingerprint; }
      catch { return false; }
    }
    function rebind() {
      const pending = [...bindings.values()].filter(b => eligible(b.record) && !valid(b));
      if (!pending.length) return;
      const elements = candidates();
      for (const binding of pending) {
        const matches = elements.filter(el => el.tagName === binding.record.tag && frameUrl(el) === binding.record.frameUrl && fingerprint(el) === binding.record.fingerprint);
        // Never guess from a stale position or pick the first duplicate card.
        binding.element = matches.length === 1 ? matches[0] : null;
      }
    }
    const labels = { CLICK: 'Clicked', SAVE_TEXT: 'Text saved', SAVE_IMAGE: 'Image saved', SAVE_LINK: 'Link saved' };
    let animation;
    function draw() {
      const groups = new Map();
      for (const binding of bindings.values()) {
        const { element, record, box, label } = binding;
        if (!eligible(record) || !valid(binding)) { box.style.display = 'none'; continue; }
        const rect = bounds(element);
        if (!rect || rect.width <= 0 || rect.height <= 0 || rect.left >= innerWidth || rect.top >= innerHeight || rect.left + rect.width <= 0 || rect.top + rect.height <= 0) { box.style.display = 'none'; continue; }
        // Stack labels for different operations on the same element.
        const group = `${Math.round(rect.left)},${Math.round(rect.top)}`;
        const slot = groups.get(group) || 0; groups.set(group, slot + 1);
        const css = `position:fixed;pointer-events:none;box-sizing:border-box;border:3px solid #ef172d;border-radius:3px;background:transparent;left:${rect.left}px;top:${rect.top}px;width:${rect.width}px;height:${rect.height}px;`;
        if (box.dataset.geometry !== css) { box.style.cssText = css; box.dataset.geometry = css; }
        box.style.display = 'block';
        label.style.top = `${rect.top < (slot + 1) * 22 ? (slot + 1) * 22 : -slot * 22}px`;
        label.textContent = `${record.state === 'attempted' ? 'Click attempted' : labels[record.action]} · ${record.step}${record.retries ? ' · retry' : ''}`;
      }
    }
    function tick() { draw(); animation = requestAnimationFrame(tick); }
    function add(record, element = null) {
      let binding = bindings.get(record.id);
      if (!binding) {
        const box = document.createElement('div'); box.className = 'mark';
        const label = document.createElement('span'); label.className = 'label';
        label.style.cssText = 'position:absolute;pointer-events:none;box-sizing:border-box;left:-3px;top:0;transform:translateY(-100%);max-width:360px;padding:2px 5px;background:#c90019;color:white;font:700 12px/17px Arial,sans-serif;white-space:nowrap;border-radius:3px 3px 0 0;text-shadow:none;';
        box.append(label); shadow.append(box);
        binding = { record, element, box, label }; bindings.set(record.id, binding);
      } else { binding.record = record; if (element) binding.element = element; }
      return binding;
    }
    memory = {
      runId,
      sync(records) {
        const ids = new Set(records.map(r => r.id));
        for (const [id, binding] of bindings) if (!ids.has(id)) { binding.box.remove(); bindings.delete(id); }
        for (const record of records) add(record);
        if (!host.isConnected) document.documentElement.append(host);
        rebind(); draw();
      },
      prepare(element, action, step, allowRepeat) {
        const signature = fingerprint(element);
        const previous = [...bindings.values()].find(b => eligible(b.record) && b.record.action === action && valid(b)
          && (b.element === element || (['SAVE_TEXT', 'CLICK'].includes(action) && text(element) && text(b.element) === text(element)
            && (b.element.contains(element) || element.contains(b.element)))));
        if (previous && (!allowRepeat || previous.record.retries >= 1)) return {
          ok: false, blocked: true, retryAllowed: previous.record.retries < 1,
          message: previous.record.retries >= 1 ? 'Repeated operation blocked: the one permitted retry was already used. Choose another target or action.' : 'Repeated operation blocked: this unchanged target already has this action mark. Choose another target/action, or explicitly request the one permitted retry.'
        };
        return { ok: true, mark: { id: previous?.record.id || crypto.randomUUID(), pageUrl: location.href, frameUrl: frameUrl(element),
          fingerprint: signature, tag: element.tagName, action, step, retries: previous ? previous.record.retries + 1 : 0,
          state: action === 'CLICK' ? 'attempted' : 'pending' } };
      },
      matches(element, mark) { return mark.pageUrl === location.href && mark.frameUrl === frameUrl(element) && mark.fingerprint === fingerprint(element); },
      record(element, mark, state) {
        const record = { ...mark, state, fingerprint: fingerprint(element) };
        add(record, element); draw(); return record;
      },
      withoutOverlay(fn) {
        host.style.setProperty('display', 'none', 'important');
        try { return fn(); } finally { host.style.removeProperty('display'); }
      },
      destroy() { cancelAnimationFrame(animation); window.removeEventListener('pagehide', onPageHide); host.remove(); bindings.clear(); }
    };
    globalThis[key] = memory; tick();
  }
  memory.sync(records);
  return { ok: true };
}
