// Serialized by chrome.scripting.executeScript: keep this function self-contained.
export function pageOperation(action, point, expected, context = {}) {
  const meta = () => ({ url: location.href, title: document.title, width: innerWidth, height: innerHeight,
    scrollX, scrollY, pageHeight: document.documentElement.scrollHeight,
    visualScale: window.visualViewport?.scale || 1 });
  const current = meta();
  const memory = globalThis.__jevbotVisualMemoryV1;
  if (action === 'META') return current;
  if (expected && ['url', 'width', 'height', 'scrollX', 'scrollY', 'visualScale'].some(k => expected[k] !== current[k])) {
    return { ok: false, message: 'Page moved or changed while Jev was answering; target was not used.' };
  }
  if (action === 'SCROLL_DOWN' || action === 'SCROLL_UP') {
    const before = scrollY;
    window.scrollBy({ top: (action === 'SCROLL_DOWN' ? 1 : -1) * innerHeight, behavior: 'instant' });
    return { ok: true, message: scrollY === before ? 'Page boundary reached; no further scroll.' : `Scrolled from ${before} to ${scrollY}.`, scrollY };
  }
  if (!point || point.x < 0 || point.y < 0 || point.x >= innerWidth || point.y >= innerHeight) return { ok: false, message: 'Target lies outside the viewport.' };
  let doc = document, x = point.x, y = point.y;
  let element = doc.elementFromPoint(x, y);
  // Resolve the actual hit through open shadow trees and same-origin frames.
  for (let depth = 0; element && depth < 20; depth++) {
    if (element.shadowRoot?.elementFromPoint) {
      const child = element.shadowRoot.elementFromPoint(x, y);
      if (child && child !== element) { element = child; continue; }
    }
    if (element.tagName === 'IFRAME' || element.tagName === 'FRAME') {
      try {
        const frameDoc = element.contentDocument;
        if (!frameDoc) return { ok: false, message: 'Cross-origin frame target is unsupported.' };
        const rect = element.getBoundingClientRect();
        // Include frame borders and CSS scaling when mapping into its viewport.
        const scaleX = rect.width / element.offsetWidth || 1;
        const scaleY = rect.height / element.offsetHeight || 1;
        x = (x - rect.left) / scaleX - element.clientLeft;
        y = (y - rect.top) / scaleY - element.clientTop;
        doc = frameDoc; element = doc.elementFromPoint(x, y); continue;
      } catch { return { ok: false, message: 'Cross-origin frame target is unsupported.' }; }
    }
    break;
  }
  if (!element) return { ok: false, message: 'No element at the Box center.' };
  const closest = (el, selector) => {
    while (el) {
      const found = el.closest?.(selector);
      if (found) return found;
      el = el.getRootNode?.().host;
    }
    return null;
  };
  const operation = action === 'PREPARE' ? context.action : action;
  let target = element;
  if (operation === 'CLICK') target = closest(element, 'button,a,input,select,textarea,[role="button"],[role="link"],[onclick]') || element;
  if (operation === 'SAVE_IMAGE') target = closest(element, 'img') || element.querySelector?.('img');
  if (operation === 'SAVE_LINK') target = closest(element, 'a[href]');
  if (!target) return { ok: false, message: `No ${operation === 'SAVE_IMAGE' ? 'HTML image' : 'hyperlink'} at the Box center.` };
  if (operation === 'CLICK') {
    if (target.disabled || target.getAttribute('aria-disabled') === 'true') return { ok: false, message: 'Target is disabled.' };
    if (typeof target.click !== 'function') return { ok: false, message: 'Target does not support DOM click.' };
  }
  if (action === 'PREPARE') {
    if (!memory || memory.runId !== context.runId) return { ok: false, message: 'Visual memory is not ready; take a fresh screenshot.' };
    return memory.prepare(target, operation, context.step, context.allowRepeat);
  }
  if (context.prepared) {
    if (!memory || memory.runId !== context.runId || !memory.matches(target, context.prepared)) return { ok: false, message: 'Target content changed before execution. Take a fresh screenshot.' };
    const guard = memory.prepare(target, operation, context.step, context.allowRepeat);
    if (!guard.ok) return guard;
  }
  const mark = state => context.prepared ? memory.record(target, context.prepared, state) : undefined;
  const label = (element.innerText || element.getAttribute?.('aria-label') || element.getAttribute?.('alt') || element.tagName).trim().slice(0, 240);
  if (action === 'CLICK') {
    mark('attempted');
    target.click();
    return { ok: true, label, mark: mark('dispatched'), message: `Click dispatched to ${label || target.tagName}; verify its effect in the next screenshot.` };
  }
  let value;
  if (action === 'SAVE_TEXT') {
    const extract = () => (element.innerText || element.textContent || '').trim();
    value = memory ? memory.withoutOverlay(extract) : extract();
  }
  if (action === 'SAVE_IMAGE') {
    const image = closest(element, 'img') || element.querySelector?.('img');
    if (!image) return { ok: false, message: 'No HTML image at the Box center (background/canvas images are unsupported).' };
    value = image.currentSrc || image.src;
  }
  if (action === 'SAVE_LINK') {
    const link = closest(element, 'a[href]');
    if (!link) return { ok: false, message: 'No hyperlink at the Box center.' };
    value = link.href;
  }
  if (!value) return { ok: false, message: 'The target has no extractable value.' };
  if (value.length > 10000) return { ok: false, message: 'Target exceeds the 10,000-character result limit; select a smaller element.' };
  if (action !== 'SAVE_TEXT' && !/^https?:\/\//i.test(value)) return { ok: false, message: 'Only HTTP(S) image and link URLs can be saved.' };
  return { ok: true, value, label, mark: mark('saved'), message: `Saved ${action.slice(5).toLowerCase()}.` };
}
