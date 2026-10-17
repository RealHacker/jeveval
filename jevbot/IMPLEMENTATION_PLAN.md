# JevBot implementation plan

## Visual action memory update

Keep the model's world screenshot-based. Add a run-scoped overlay in the isolated
content-script world: salient red outlines with step/action labels for CLICK,
SAVE_TEXT, SAVE_IMAGE, SAVE_LINK. Click marks mean dispatched, never that the
overall item is complete. Failed actions do not receive successful-action marks.
Overlays use a closed shadow root, pointer-events:none, fixed positioning, and
live element bounds; same-origin frame coordinates are mapped into the top page.
They must not affect document layout, hit testing, or extracted text.
Set styles directly via CSSOM so pages forbidding inline stylesheet blocks still
show marks; verify actual red pixels under `style-src 'none'`.

Persist lightweight internal mark records with the run, independently of model
state. Before navigation clicks, persist the intended target mark so returning
to a feed can restore it even if navigation destroys the action context. Commit
successful dispatches afterward. Restore only on the exact source URL and a
unique content match (image/link URL or exact text/control attributes); do not
restore marks from coordinates alone or guess when matching is ambiguous. Live
element references handle scrolling/resizing; fingerprints detect recycled DOM
nodes. Internal descriptors never become DOM world state sent to Jev. Clear old
run marks before a new run, and remove them when clearing history.

Before an operation, compare it against marks on the resolved target; block the
same operation on an unchanged target, allowing different operations and changed
controls. Report blocked repeats in history. A repeat may only proceed after a
separate explicit Choice question (choose another target vs retry), with at most
one retry per operation/target. Three blocked/failing attempts still stop the run.
Explain labels and this rule in every model request; do not add element IDs or a
target registry to model input. Sanitise internal descriptors out of action results
and history before API calls. Tests must verify actual screenshot marks, live
alignment, pointer transparency, navigation restoration, ambiguous/no-position
matching, reused nodes, operation-specific guards, retry bounds, fresh-run reset,
and unchanged Choice -> Box behavior.

## Product and interaction

Build an unpacked Chrome Manifest V3 extension (Chrome 116+) with a native side
panel, a separate options page, and no build step or remote executable code.
The panel has an instruction composer, step limit, live action timeline, collected
results, JSON export, and local request history. Use compact typography, warm
neutral surfaces, and teal status accents suitable for a narrow sidebar.

## API contract (verified 2026-10-17)

- POST `https://api.typesafe.ai/v1/systemone`, Bearer API key, JSON body, and
  `TypeSafe-Danger-Zone-Images: true` on every request.
- Default `jev-preview` because the Box documentation explicitly requires Jev
  1.14 (`jev-preview` or `jev-1.14.0`). Allow a model override in options.
- A fresh UUID in `assets` names a PNG screenshot, using `data_base64` without
  a data URL prefix. Reference it once in `state.screenshot`.
- State includes the instruction, viewport/page metadata, and every prior action
  with results. Choice asks for exactly one of CLICK, SCROLL_DOWN, SCROLL_UP,
  GO_BACK, SAVE_TEXT, SAVE_IMAGE, SAVE_LINK, DONE.
- CLICK and every SAVE action require a subsequent separate Box request using
  the same screenshot, state, and selected action. Box has no criteria. Validate
  normalized edges before mapping its center to CSS viewport coordinates.
- Fit captures within 280..1568 pixels on both axes. Preserve aspect ratio and
  pad extreme aspect ratios; map Box coordinates through content bounds so
  padding cannot become a click target. Device pixel ratio is not CSS size.

Sources: https://docs.typesafe.ai/images,
https://docs.typesafe.ai/primitives/box,
https://docs.typesafe.ai/primitives/choice,
https://developer.chrome.com/docs/extensions/reference/api/sidePanel,
https://developer.chrome.com/docs/extensions/reference/api/tabs.

## Execution and persistence

1. The side panel owns the asynchronous loop, avoiding MV3 worker lifetime
   dependence. The service worker only configures toolbar-to-panel opening.
2. Acquire an extension-origin Web Lock to allow one active runner. Bind it to
   the selected tab and window. Persist the request before the first API call.
3. Wait for tab load (bounded timeout), then the configured settle delay, then
   screenshot. Check the active tab before/after capture and before execution.
4. Record the chosen action, obtain Box if required, persist the intended action
   before applying it, then execute serially. Record failures as observations for
   Jev, with a consecutive-error ceiling. Never retry an uncertain click.
5. Execute a self-contained injected function using `document.elementFromPoint`;
   traverse open shadow roots and same-origin frames. Click the hit element (or
   actionable ancestor); extract text/currentSrc/closest anchor href for saves.
   Reject unsupported targets with actionable results. Scroll one viewport and
   use `chrome.tabs.goBack` for navigation. Results include source URL and Box.
6. Guard against changed document, URL, viewport, or scroll position while the
   model is answering. Stop on tab changes. Stop aborts requests; closing the
   panel interrupts execution. Interrupted runs are inspectable and rerunnable,
   never automatically replayed. DONE and the step limit have distinct statuses.
7. Store requests, actions, and results in extension-origin localStorage, bounded
   to 40 runs with text/result size bounds. Do not persist screenshots or API
   keys in run history. API key/settings live in chrome.storage.local restricted
   to trusted extension contexts. Render all page/model/user text as textContent.

## Permissions and boundaries

Use sidePanel, storage, scripting, tabs, and <all_urls> host access (required for
capture and scripting across click/back navigation). Restrict execution to HTTP(S)
pages; Chrome internal/store pages are unsupported. Do not inject the API key or
arbitrary model code. Screenshots and task history are sent to TypeSafe when Run
is pressed, explained in the UI/options. DOM clicks are synthetic; sites requiring
trusted input, cross-origin frames, closed shadow roots, canvas text, and CSS
background images are explicitly outside the initial extraction contract.

## Validation and delivery

- Dependency-free Node tests: exact API contract/header, Choice -> Box ordering,
  normalized/padded coordinate mapping, DONE, step cap, cancellation, stale
  viewport, failed actions, deduplication, persistence, and injected DOM actions.
- Serve a deterministic fixture page and test real browser clicks, scroll,
  navigation, and extraction where the available browser permits it. Use a mock
  API for repeatable automation without billing. Keep live API claims separate.
- Inspect panel/options at sidebar width, verify history and results workflows,
  syntax-check all modules, and package an unpacked-ready ZIP outside source.
- Document installation, configuration, behavior, privacy, limitations, and the
  exact verification performed in README.md.
