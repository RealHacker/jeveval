# JevBot

A Chrome extension that uses Jev's screenshot understanding to operate on a web
page. No build step, dependencies, backend, or injected model-generated code.

## Install and run

1. In Chrome 116+, open `chrome://extensions` and enable **Developer mode**.
2. Select **Load unpacked** and choose this `jevbot` directory (the directory
   containing `manifest.json`). Alternatively extract `.artifacts/jevbot.zip`
   into a folder and load that folder.
3. Pin JevBot in the extensions toolbar and click it to open the native side panel.
4. Open **Settings**, enter your TypeSafe API key, and save. Default model:
   **jev-preview**, with a **1,000 ms** wait before each screenshot.
5. Return to the webpage, enter a task, set a step limit, and select **Run on this
   page**. Keep its tab/window active and the side panel open.

Examples:

- `Like every comment on this page. Scroll through the entire page and do not unlike comments that are already liked.`
- `Find every cat image on this page and save its image URL. Continue scrolling until you have checked the whole page.`
- `Save the text and link URL of each visible article about databases.`

**Stop** aborts pending model requests and prevents subsequent actions. It cannot
undo actions already dispatched. Switching tabs/windows stops execution before
the next action. Closing/reloading the panel interrupts the run. Reopening
history restores the instruction, action trace, and results; pressing Run creates
a new run, never an automatic replay of interrupted clicks.

Use **Results** for collected text and URLs. Image saving collects the URL,
without downloading the binary. **Export JSON** exports the selected request,
actions, coordinates, and results. The history list keeps up to 40 recent runs;
older entries may be pruned earlier to stay within the storage budget. Clear
history removes all request records from this profile (settings remain).

## Visible action memory

After each click or successful save, JevBot draws a salient red outline around
the target, labeled with its action and step number: **Clicked · 3**, **Text
saved · 4**, **Image saved · 5**, or **Link saved · 6**. These marks appear on
the page and in the actual screenshots sent to Jev. Its decision instructions
explain that a mark records an operation, not completion of the whole item. For
example, clicking a food image still allows saving its description afterward.

The overlays are non-interactive and do not affect page layout, hit testing, or
saved text. They follow the live target during scrolling, resizing, and layout
changes. Different operations on the same target have separate labels.

JavaScript checks a selected target before dispatching the operation. Repeating
the same operation on the same unchanged target is blocked and recorded in the
action history. A separate Jev Choice asks it to **CHOOSE_ANOTHER** target/action
or explicitly **RETRY_ONCE** when the task and screenshot justify it. Only one
retry per unchanged target/operation is allowed. Further repeats are blocked;
three consecutive blocked/failed steps stop the run. A new run starts with fresh
marks. Clear history also removes overlays from the recorded tabs.

Mark records are stored locally with the request so returning from a detail page
can restore them. The implementation uses live DOM references and lightweight
content fingerprints internally, without exposing element IDs, a DOM tree, or
the fingerprints to Jev. Restoration requires the exact source URL and one unique
content match (text/control attributes or image/link URLs); it never guesses from
old screen coordinates. Removed or recycled nodes lose their old marks unless a
unique match is found again. Identical cards, changed content, changed URLs, or a
page over the 20,000-element restoration scan bound may prevent restoration.
There is no screenshot similarity matcher in this version.

Click intents are persisted before dispatch, because navigation can destroy the
action context. If dispatch cannot be confirmed, a restored **Click attempted**
mark means the outcome is uncertain. Known failed operations do not get success
marks. Inspect uncertain clicks before repeating them. Marks are run-scoped and
are not carried automatically into a new request.

## Decision/action contract

Each step waits for page loading (up to 20 seconds), then the configured render
delay (500–10,000 ms), then captures the visible page. It sends the user task,
page metadata, screenshot, prior actions and their results, and collected values
to `POST https://api.typesafe.ai/v1/systemone`.

The first question is `type: "choice"` with eight action keys:

| Action | Behavior |
| --- | --- |
| CLICK | Separate Box question, then DOM click on the hit control |
| SCROLL_DOWN | Scroll the main document down one viewport |
| SCROLL_UP | Scroll the main document up one viewport |
| GO_BACK | Navigate backward once within the starting tab |
| SAVE_TEXT | Separate Box question, then extract hit element's rendered text |
| SAVE_IMAGE | Separate Box question, then extract `img.currentSrc` / `src` |
| SAVE_LINK | Separate Box question, then extract nearest anchor's absolute URL |
| DONE | End the run immediately |

All targeted actions require a **subsequent Box call using the same screenshot**;
no speculative Box question is combined with Choice. Both requests include
`TypeSafe-Danger-Zone-Images: true`. PNG data is carried in `assets` as raw
`data_base64`, referenced by a fresh UUID in `state.screenshot`. Box edges are
validated as normalized fractions. Screenshot resize/padding is mapped back to
CSS viewport coordinates before calling `document.elementFromPoint(x, y)`.

The default model is `jev-preview` because the [Box documentation](https://docs.typesafe.ai/primitives/box)
specifies Jev 1.14 (`jev-preview` or `jev-1.14.0`). The [image documentation](https://docs.typesafe.ai/images)
defines image assets, dimensions, and the required opt-in header. These are
experimental features; model accuracy and API formats may change.

The action trace records each intent before applying it, and records dispatch
results afterward. Click success means dispatch succeeded; Jev must inspect the
next screenshot to determine the site's actual response. A lost click context
stops the run as uncertain rather than retrying. Duplicate collected values are
marked in history and stored once per type. Three consecutive action failures
stop the run. DONE, user Stop, errors, interruption, and step limit have distinct
statuses. Limits: 1–200 steps, 8,000 instruction characters, 10,000 characters per
result, 200,000 collected characters per run.

## Permissions, privacy, and limitations

- `sidePanel`: native sidebar. `tabs`: bind to and inspect the target tab;
  screenshot and back navigation. `scripting`: run the fixed DOM executor.
  `storage`: local API key/settings. `<all_urls>`: capture and operate after
  navigation across HTTP(S) origins without repeated permission prompts.
- The API key lives in `chrome.storage.local`, restricted to trusted extension
  contexts. It is never passed into page scripts or exported in request history.
  Local browser storage is not application-encrypted.
- Task history uses **extension-origin localStorage**, not the webpage's storage.
  It includes extracted values and source URLs. Screenshots remain in memory;
  they are sent to TypeSafe but are not stored in history. No analytics.
  Local mark records additionally contain target content fingerprints for
  restoration; these can include text and URLs but stay out of model requests.
- Only TypeSafe receives remote API calls. The CSP allows that API origin and
  local `data:` decoding for screenshots; scripts are bundled, with no eval.
- Chrome internal pages, the Chrome Web Store, file URLs, and some built-in
  document viewers cannot be automated. Open shadow roots and same-origin
  iframes are supported; cross-origin frames and closed shadow roots are not.
- The Box center chooses one hit element. Canvas text, background-image URLs,
  OCR, typing, keyboard commands, drag gestures, downloads, and OS dialogs are
  outside the available action set. Text saves extract the hit element, not an
  arbitrary multi-element region. Non-HTTP(S) image/link URLs are rejected.
- DOM `.click()` generates synthetic input; some sites require trusted events.
  Scroll operates on the main document, not nested scroll containers. Navigation
  stays bound to the original tab; a newly opened tab is not followed.
- Document identity, URL, scrolling, and viewport dimensions are checked before
  execution. Dynamic changes inside an otherwise unchanged viewport can still
  invalidate a target. Use a longer settle delay on slow pages. Avoid changing
  page state while the model is answering. Reset pinch zoom before running.
- The model may misidentify targets or stop before completing the task. Review
  the page and trace, especially before repeating an interrupted run.

## Development and validation

The implementation design is in `IMPLEMENTATION_PLAN.md`. Responsibilities:
`core.js` owns the loop, `api.js` the API contract, `browser.js` Chrome capture and
navigation, `marks.js` visual memory/restoration and repeat guards, `page.js` fixed DOM actions, `history.js` persistence, `panel.js` the
sidebar, and `options.js` settings. The MV3 worker only configures the panel and
storage access; no long-running task depends on its lifetime.

Node 20+:

```powershell
cd E:\Toy\JevEval\jevbot
npm test
npm run check
powershell -ExecutionPolicy Bypass -File scripts/package.ps1
```

Optional Chromium integration (Playwright installed separately):

```powershell
# If Playwright is not installed in node_modules, point at its index.mjs.
$env:PLAYWRIGHT_MODULE = 'C:\path\to\playwright\index.mjs'
# Optional; otherwise uses Playwright's Chromium channel.
$env:CHROMIUM_EXECUTABLE = 'C:\path\to\chrome.exe'
node tests/browser.mjs
```

The browser test creates its own temporary profile, loads this actual extension,
serves `tests/fixture.html` locally, intercepts TypeSafe requests with deterministic
Choice/Box answers, and exercises real Chrome capture/scripting/navigation. The
temporary test profile is removed afterward. Screenshots and JSON export are
retained in ignored `.artifacts/`. No live API key or paid request is used.

Verified on 2026-10-17:

- All 22 Node unit tests and all module syntax / manifest entry points.
- Chromium DOM clicks, text/image/link saves, shadow DOM, same-origin iframe,
  stale viewport rejection, and scrolling.
- Actual unpacked-extension integration: all eight actions, 14 mocked Jev calls,
  screenshot encoding, required headers, navigation/back, options persistence,
  history restore, result views, and JSON export.
- Actual panel step limit, Stop during a pending Box call, and interrupted-intent
  recovery without replay.
- Rendered sidebar at 390 px and settings screenshot visually inspected.
- Visual-memory browser checks: actual red screenshot pixels, scroll/resize/layout
  alignment, pointer transparency, nested text guard, operation-specific blocking,
  bounded retry, navigation/re-created node restoration, recycled-node and
  ambiguous-match safety, and fresh-run reset. The marked page screenshot was
  visually inspected (`.artifacts/visual-marks.png`).
- Red mark pixels also verified with `style-src 'none'`; the renderer uses direct
  CSSOM styles inside its closed shadow tree instead of inline stylesheet blocks.
- Actual extension API interception confirms red marks are in the screenshot sent
  to Jev (`.artifacts/model-screenshot.png`) and fingerprints stay local. A full
  repeated-image sequence executes one explicit retry and blocks further repeats.

The browser integration opens the sidebar HTML in an extension tab with the
fixture kept active. Native toolbar/panel docking and a live Jev completion on a
third-party site (including Pinterest) have **not** been verified. A mock run verifies execution and
API structure; it does not establish model accuracy.
