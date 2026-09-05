# Task 3 report: credential fields per provider

## What was implemented

`apps/desktop/src/components/settings/ProviderSection.tsx` gained three new field
blocks, rendered conditionally on `active`, in this order (after the existing
provider-choice/state blocks, before the error block):

1. **Codex login/status** (`active === "openai-codex"`) — a
   `provider-codex-login` button that calls `codex.login_start`, opens the
   returned `auth_url` via `openExternalUrl`, then polls `codex.login_status`
   every 1500ms via `window.setInterval`. The poll stops on **either**
   `logged_in` or `login_failed` (both call `stopPolling()` then `refresh()`).
   A `provider-codex-email` span shows `codexStatus.email`, falling back to
   `t("settings.providers.codex.signedIn")` when the id_token carries no email
   claim. A `provider-codex-logout` button calls `codex.logout()`. A
   `provider-codex-failed` alert shows on `login_failed`.
   - `pollRef` (a `useRef<number | null>`) holds the interval handle;
     `stopPolling` clears and nulls it; `useEffect(() => stopPolling, [stopPolling])`
     clears it on unmount.
   - Deviation from the brief's literal snippet: I added
     `setCodexStatus(null)` at the top of `startCodexLogin`, before
     `stopPolling()`. Reasoning: without it, retrying after a `login_failed`
     leaves the old failure banner on screen for the ~1.5s until the first
     fresh poll tick reports back — a stale "failed" message during an
     in-flight retry reads as if the new attempt already failed. Covered by
     a new test, "clears a stale failure banner as soon as a retry starts".

2. **API key field** (`active !== "openai-codex"`) — a `type="password"`
   input (`provider-key-input`) bound to a local `keyDraft` string that
   **never** gets pre-filled from `current?.configured` (settings.get never
   returns the secret, only `api_key_set`/`configured`). The placeholder
   alone communicates whether a key is stored
   (`settings.providers.apiKey.stored` vs `...placeholder`). A `provider-key-save`
   button is disabled unless `keyDraft.trim()` is non-empty, and sends
   `settings.set({ providers: { [active]: { api_key: keyDraft.trim() } } })`.
   A `provider-key-clear` button (shown only when `current?.configured`) sends
   `api_key: ""`, which the engine treats as a delete.

3. **Base URL field** (`active === "openai"` only) — `provider-base-url-input`,
   `type="url"`, saved `onBlur` via `settings.set({ providers: { openai: { base_url } } })`.

A `useEffect` keyed on `[active, list]` resets `keyDraft` to `""` and
`baseUrlDraft` to the newly-active provider's stored `base_url` (or `""`)
every time the active provider or the provider list changes — this is what
keeps an `openai` base URL from riding along into an `anthropic` patch when
the writer switches providers (the engine's `Set()` is all-or-nothing and
rejects `base_url` on any id but `openai`).

## i18n

Added under `settings.providers.*` in all three catalogues in
`apps/desktop/src/lib/i18n.tsx` (ko/en/ja): `codex.login`, `codex.logout`,
`codex.signedIn`, `codex.failed`, `apiKey`, `apiKey.stored`,
`apiKey.placeholder`, `apiKey.save`, `apiKey.clear`, `baseUrl`,
`baseUrl.hint`. `i18n.catalog.test.ts` (key-parity + placeholder-parity
across languages) passed unmodified.

## Commands run and output

### 1. Failing tests first (RED)

```
cd apps/desktop && pnpm test ProviderSection
```

Before implementing the component changes, I added the new mocks
(`codex.loginStart/loginStatus/logout`, `openExternalUrl`) and all 12 new
test cases (later grown to 13 after the retry-banner fix) to
`ProviderSection.test.tsx`. Result:

```
 ❯ src/components/settings/ProviderSection.test.tsx (16 tests | 5 failed) 25248ms
   × ProviderSection > opens the browser and polls until Codex reports signed in ... Test timed out in 5000ms.
   × ProviderSection > shows a signed-in fallback when Codex reports no email claim ... Test timed out
   × ProviderSection > stops polling when Codex reports a failed login ... Test timed out
   × ProviderSection > clears the interval on unmount so an abandoned login stops calling the engine ... Test timed out
   × ProviderSection > logs out of Codex and returns to the sign-in button ... Test timed out

 Test Files  1 failed (1)
      Tests  5 failed | 11 passed (16)
```

The 11 non-Codex-polling tests failed for the expected reason at that point
(missing `provider-key-input`/`provider-base-url-input`/etc. elements —
confirmed via an earlier intermediate run before the RPC mocks were even
wired up, where the error was `Unable to find an element by:
[data-testid="provider-base-url-input"]` against the pre-Task-3 DOM). The 5
Codex tests timed out because the component had no `provider-codex-login`
button yet, so `findByTestId` never resolved. This confirmed the tests were
exercising code that did not exist yet.

### 2. Implementation, then GREEN

```
cd apps/desktop && pnpm test ProviderSection
```
```
 ✓ src/components/settings/ProviderSection.test.tsx (16 tests) 230ms
 Test Files  1 passed (1)
      Tests  16 passed (16)
```

### 3. Full verification

```
make test-desktop
```
```
$ pnpm lint    → ✖ 28 problems (0 errors, 28 warnings)   [pre-existing `any` warnings, unrelated to this change]
$ pnpm test    → Test Files  61 passed (61) / Tests 292 passed (292)
$ tsc -b && vite build → ✓ built in 1.27s
```

Then, after adding the retry-banner fix and its test:

```
cd apps/desktop && pnpm test ProviderSection
```
```
 ✓ src/components/settings/ProviderSection.test.tsx (17 tests) 225ms
```

```
make test-desktop
```
```
Test Files  61 passed (61)
     Tests  293 passed (293)
tsc -b && vite build → ✓ built in 1.29s
```

Per the task instructions, `go test ./...` was **not** run (this task
touches no Go code, and the brief warned it can hang on this machine's
locked keychain).

## Deviations from the brief

1. **`setCodexStatus(null)` added at the start of `startCodexLogin`** (see
   above) — not in the brief's literal snippet, added to avoid a stale
   failure banner during a retry. Low risk, covered by a new test.
2. **Test file structure**: the brief's illustrative test names ("never puts
   a stored key back into the input", "clears a key by sending an empty
   string", "offers base URL only for the openai-compatible provider",
   "drops the base URL draft when the provider changes", "opens the browser
   and polls until Codex reports signed in", "stops polling when Codex
   reports a failed login") are all present verbatim. I added several more
   for coverage the brief flagged as "easy to get wrong" or implied but
   didn't spell out as a test: placeholder-for-unconfigured-provider,
   save-button-disabled-until-typed/trimmed, no-clear-button-when-unconfigured,
   Codex email-fallback-when-absent, interval-cleared-on-unmount, and
   Codex logout. This is additive, not a change in behavior from the brief.
3. **Timer-test mechanics**: the brief says to use `vi.useFakeTimers()` +
   `vi.advanceTimersByTimeAsync`. I found two additional constraints not
   spelled out, needed to avoid hangs:
   - `vi.useFakeTimers()` must be called *after* the initial
     `screen.findByTestId(...)` used to locate the login button on first
     render, not before — calling it before the first `findByTestId` caused
     every such test to hang for the full 5s test timeout (confirmed by
     reproducing with a debug run). Likely cause: `@testing-library/dom`'s
     `waitFor`/`findBy*` polling here does not resolve purely from a
     synchronous initial check in this dependency version, so it can be
     starved when both the interval polling and MutationObserver retry path
     end up gated behind timers that are faked but never advanced.
   - After an explicit `vi.advanceTimersByTimeAsync(...)` flush inside
     `act()`, I used synchronous `screen.getByTestId(...)` rather than
     `await screen.findByTestId(...)` for the assertion — using `findByTestId`
     at that point also hung for the same reason, even though the element
     was already present in the DOM (verified via a debug
     `document.body.innerHTML` dump). This matches the existing project
     convention (`fireEvent` + `act` + synchronous assertions) used in
     `ContextPanel.test.tsx`'s fake-timer test, which I followed once I hit
     this.
   These are test-harness workarounds only; no application code was
   affected by this finding.

## Self-review findings

- Verified `settings.get`'s redaction contract is honored: `keyDraft` is
  local-only state, never initialized from any RPC response, and is reset to
  `""` after every save/clear and on every provider switch.
- Verified the all-or-nothing `Set()` contract: `baseUrlDraft` is scoped by
  the `[active, list]` effect and the base-url field/save path only exists
  when `active === "openai"`, so no other provider's patch can carry
  `base_url`. Confirmed with a test that switches `openai → anthropic`,
  types and saves a key, and asserts the resulting patch has no `base_url`
  key at all (not even `undefined` — `toHaveBeenLastCalledWith` requires an
  exact shape match).
  - Note: `settingsApi.set` in `saveKey`/`clearKey` sends only
    `{ providers: { [active]: { api_key } } }` — never spreads in
    `baseUrlDraft` — so this is enforced structurally, not just by the
    effect resetting the draft.
- Verified `login_failed` stops the poll (dedicated test, plus a follow-up
  assertion that advancing the fake timer further does not produce more
  `codex.login_status` calls).
- Verified the interval is cleared on unmount (dedicated test: unmount right
  after starting a login, then advance fake timers and assert
  `codex.login_status` was never called).
- Considered but did not change: `startCodexLogin`'s interval is not tied to
  `active`, so if a writer starts a Codex login and then switches to another
  provider before it resolves, the poll keeps running in the background
  (harmless — it only calls `setCodexStatus`/`refresh`, and stops itself on
  its own terminal condition or on unmount). The brief only requires
  clearing on unmount, which is satisfied; I did not add extra
  active-provider-aware cancellation since it wasn't asked for and the
  behavior is otherwise inert.
- Confirmed `Settings.tsx` does not reference `ProviderSection` (Task 6 is
  untouched) and no files outside `apps/desktop` were touched.
- Confirmed no reuse of the stale `.provider-test*` CSS classes — all new
  markup reuses the existing `modal-field`/`sd` classes already used
  elsewhere in this component and in `Settings.css`.

## Commit

```
feat(desktop): credential fields per provider — Codex login, API key, base URL (#94)
```

---

# Task 3 review round 2: fixing C1, I2–I4 and five minors

All four findings were real. Nothing was pushed back on. `apps/desktop` only;
no i18n keys added (every string the fixes touch already existed); `Settings.tsx`
still does not reference `ProviderSection` (Task 6 is untouched).

## C1 — a background refresh erased an unsaved API-key draft

The reset effect was keyed on `list` **object identity**, and `refresh()`
`setList`s a freshly-parsed array on every call. So a reload — a save's own
reload, an abandoned Codex login's poll tick — was indistinguishable from a
provider switch, and both ran `setKeyDraft("")`.

The fix separates the two. What retires a draft is the writer *changing
provider*, tracked explicitly:

```tsx
const draftProvider = useRef<ProviderID | null>(null);
useEffect(() => {
  if (draftProvider.current === active) return;
  draftProvider.current = active;
  setKeyDraft("");
  setBaseUrlDraft(list.find((r) => r.id === active)?.base_url ?? "");
}, [active, list]);
```

`list` stays in the deps because that is where the newly-active provider's
stored `base_url` is read from — the seed still has to happen on the commit
where `refresh()` delivers both the row and the new `active` (React batches
`setList`/`setActive`, so they land together). The guard is what makes the
effect ignore every *other* reason the list changed.

Two tests cover it, both of them the reviewer's own reproductions:
`keeps an unsaved key draft through a background providers.list reload` and
`does not erase a key typed on another provider when an abandoned Codex poll
returns`.

## I2 — the mocks made the suite blind

`rpc.providersList.mockResolvedValue([...])` handed back the same array
reference on every call, so `setList(sameRef)` made React bail out and the
`[active, list]` effect never re-ran in any test. Replaced with a small
stateful fake in `beforeEach` that behaves like the engine:

```tsx
rpc.providersList.mockImplementation(() => Promise.resolve(rows(activeId, rowExtras)));
rpc.settingsSet.mockImplementation((patch: { provider?: string }) => {
  if (patch.provider) activeId = patch.provider;
  return Promise.resolve({});
});
```

`rows()` builds four fresh row objects per call, so every `providers.list`
resolves a new array the way real parsed JSON does, and `settings.set({provider})`
actually moves the active row — which is what lets a test switch providers and
come back. Every per-test `mockResolvedValueOnce([...])` chain for the list is
gone; tests now say `activeId = "openai"` / `rowExtras = { openai: { base_url } }`
before rendering.

## I3 — an already-signed-in Codex account rendered as "not signed in"

`codex.login_status` was only ever called from inside the poll interval, so
opening settings with an account already signed in showed the sign-in button
while `provider-state` said `configured`, and the logout button was unreachable
without a whole fresh OAuth round trip. Added an effect keyed on `active`:

- when `active === "openai-codex"`, fetch `login_status` (with a `cancelled`
  flag so a fast provider switch cannot land a stale status);
- when `active` is anything else, `setCodexStatus(null)` — which also fixes the
  related lower finding, a stale `login_failed` banner reappearing when the
  writer returns to Codex.

Its `.catch` is deliberately silent, unlike the poll's (Minor 4). The
distinction: during a login attempt the writer is waiting on an answer, so a
dead poll needs to say so; on mount nobody asked, the sign-in button is already
the correct thing to render, and an error banner on every settings open would
say nothing actionable.

Tests: `shows an already signed-in Codex account without waiting for a fresh
login`, `drops a stale Codex failure when the writer leaves the provider and
comes back`. The logout test no longer has to fake a whole login first — it
renders signed-in and clicks, which is the path this finding unblocked.

## I4 — the poll's `void refresh()` had no `.catch`

Now `void refresh().catch(setError)`, matching the mount call site. Test:
`reports a reload that fails after a successful login`. Against the pre-fix
component that test also produces the `unhandledrejection` the finding
predicted (`Vitest caught 1 unhandled error during the test run`).

## Minors

1. **`:281` `saveBaseUrl` on every blur.** Now compares the trimmed draft
   against the row's stored `base_url` and returns before `guard` when nothing
   changed, so a focus-in/focus-out costs no round trip. `const current` moved
   above the callbacks so it can read the stored value. Test: `does not save the
   base URL on a blur that changed nothing` (asserts `settings.set` uncalled
   *and* `providers.list` still at one call).
2. **`setCodexStatus(null)` placement.** Moved above `await codexApi.loginStart()`.
   The test now proves the timing rather than the end state: `login_start` is
   held on a manually-released promise, and the banner is asserted gone while it
   is still pending.
3. **`??` → `||` for the email claim**, so `email: ""` renders "Signed in"
   rather than an empty span next to a Logout button. Test: `falls back to a
   signed-in label when the email claim is an empty string`.
4. **A rejecting `login_status` now signals.** `.catch(() => stopPolling())`
   became `.catch((e) => { stopPolling(); setError(e); })`. The contract is
   preserved — it still stops the poll — and it is no longer the one poll exit
   without a test: `stops the poll and says why when login_status itself fails`
   asserts both the rendered reason code and that no further tick fires.
5. **The mis-named base-URL test.** Renamed to `never lets an openai base URL
   ride into another provider's key patch`, which is what its
   `toHaveBeenLastCalledWith` exact-shape assertion actually guarantees. The
   drop it claimed to observe is now a separate test that does observe it:
   `drops an unsaved base URL draft when the writer changes provider` types a
   base URL, leaves for Anthropic, comes back, and asserts the input is empty.

## Contracts re-checked, not weakened

- The poll still stops on `logged_in`, on `login_failed`, on a rejected
  `login_status`, and on unmount — one dedicated test each, all four still
  present and each still asserting that advancing the timer further produces no
  additional call.
- Save is still disabled on an empty draft; clearing is still its own button
  sending `api_key: ""`.
- No `base_url` can reach another provider's patch: the field only renders for
  `openai`, and `saveKey`/`clearKey` never spread the base URL draft — the
  structural guarantee, with the exact-shape test above.
- No stored key reaches the input: `keyDraft` is still initialized only from
  typing. The C1 fix makes it survive *longer*, so `never puts a stored key back
  into the input` was re-checked against a `configured: true` row and still
  passes.

## Verification

### New tests fail against the pre-fix component

`git show 19c680b:.../ProviderSection.tsx` was written over the component (the
fixed copy parked in a scratch dir outside the repo, restored afterwards) and
the new suite run against it:

```
   × ProviderSection > keeps an unsaved key draft through a background providers.list reload 9ms
   × ProviderSection > does not save the base URL on a blur that changed nothing 5ms
   × ProviderSection > does not erase a key typed on another provider when an abandoned Codex poll returns 6ms
   × ProviderSection > shows an already signed-in Codex account without waiting for a fresh login 1007ms
   × ProviderSection > falls back to a signed-in label when the email claim is an empty string 1007ms
   × ProviderSection > drops a stale Codex failure when the writer leaves the provider and comes back 11ms
   × ProviderSection > stops the poll and says why when login_status itself fails 4ms
   × ProviderSection > reports a reload that fails after a successful login 4ms
   × ProviderSection > clears a stale failure banner before the retry's first round trip 1005ms
   × ProviderSection > logs out of Codex and returns to the sign-in button 1006ms
 Test Files  1 failed (1)
      Tests  10 failed | 16 passed (26)
```

The assertions, one per finding:

```
FAIL > keeps an unsaved key draft through a background providers.list reload
AssertionError: expected '' to be 'sk-live-typed' // Object.is equality

FAIL > does not erase a key typed on another provider when an abandoned Codex poll returns
AssertionError: expected '' to be 'sk-ant-typed' // Object.is equality

FAIL > shows an already signed-in Codex account without waiting for a fresh login
TestingLibraryElementError: Unable to find an element by: [data-testid="provider-codex-email"]

FAIL > reports a reload that fails after a successful login
TestingLibraryElementError: Unable to find an element by: [data-testid="provider-error"]

FAIL > stops the poll and says why when login_status itself fails
TestingLibraryElementError: Unable to find an element by: [data-testid="provider-error"]

FAIL > drops a stale Codex failure when the writer leaves the provider and comes back
AssertionError: expected <p class="sd" role="alert" …(1)></p> to be null
+ Received: <p class="sd" data-testid="provider-codex-failed" role="alert">

FAIL > does not save the base URL on a blur that changed nothing
AssertionError: expected "spy" to not be called at all, but actually been called 1 times
  1st spy call: [ { providers: { openai: { base_url: "https://openrouter.ai/api/v1" } } } ]

⎯⎯⎯⎯⎯⎯ Unhandled Errors ⎯⎯⎯⎯⎯⎯
Vitest caught 1 unhandled error during the test run.
⎯⎯⎯⎯ Unhandled Rejection ⎯⎯⎯⎯⎯
Error: x
 ❯ src/components/settings/ProviderSection.tsx:67:37
 ❯ src/components/settings/ProviderSection.tsx:128:20    ← the unguarded `void refresh()`
```

### After the fix

```
cd apps/desktop && pnpm test ProviderSection
```
```
 ✓ src/components/settings/ProviderSection.test.tsx (26 tests) 290ms

 Test Files  1 passed (1)
      Tests  26 passed (26)
```

```
make test-desktop
```
```
$ eslint src --max-warnings 28
✖ 28 problems (0 errors, 28 warnings)      [the same pre-existing `any` warnings]

 Test Files  61 passed (61)
      Tests  302 passed (302)

$ tsc -b && vite build
✓ 1928 modules transformed.
✓ built in 1.29s
```

`go test ./...` not run, per the instructions: no Go changed, and this Mac's
locked keychain hangs the engine fixtures.

## One thing left alone, deliberately

C1's first path had a second mechanism the reviewer noted: `busy` disables the
Save button between mousedown and mouseup, so the click that follows a blur-save
is swallowed. That is now a cosmetic re-click rather than a destroyed key — the
draft survives, Save re-enables, and the second click saves — and Minor 1 removes
the common case (a blur that changed nothing no longer sets `busy` at all). A
general fix would mean rethinking `busy` as a single section-wide flag, which is
wider than this task and would touch the disabled states the other tests pin.
Flagging it rather than half-doing it.

The abandoned Codex poll still runs after the writer switches providers; it is
inert (it sets a `codexStatus` nothing renders, and stops on its own terminal
condition or on unmount), and its `refresh()` no longer disturbs a draft.

## Round 2: the orphaned Codex poll resurrecting a stale failure banner

**Finding (Important).** The mount effect added last round (I3) re-fetches
`codex.login_status` fresh whenever `active` becomes `"openai-codex"`, gated
by its own `cancelled` flag. But the poll started by `startCodexLogin` never
checked `active` before calling `setCodexStatus(s)`. Reproduction: start a
Codex login, abandon the browser, switch to Anthropic, switch back to Codex.
The mount effect's fresh fetch resolves first with `{logged_in:false}` — no
banner. A tick from the poll that was never stopped by the two switches then
resolves with `{logged_in:false, login_failed:true}` and overwrites it: a
failed-login banner on a settings visit that made no login attempt.

**Why a live `active === "openai-codex"` check does not fix it.** By the time
the orphaned tick's promise resolves, the writer has already switched back —
`active` again reads `"openai-codex"`. A gate that reads current `active` at
resolve time would see a match and let the stale write through. What actually
needs remembering is not "is the writer on Codex right now" but "has the
writer left since this particular poll started" — a one-way fact, not a live
mirror of `active`.

**Fix.** Added `pollValidRef` (`ProviderSection.tsx:56-64`): `true` from the
moment `startCodexLogin` creates the interval, flipped to `false` for good the
moment the mount effect's `[active]` effect sees the writer has left
`"openai-codex"` (`:143`) — never reset back to `true` by merely returning,
only by a fresh `startCodexLogin()`. The poll's tick checks
`pollValidRef.current` before `setCodexStatus(s)` and before the
logged_in/login_failed terminal branch (`:187`), so a late write from an
abandoned session is silently dropped and the mount effect's fresh fetch is
left standing. Chose gating the write over stopping the poll on the
provider-leave transition: stopping the interval would still not cancel an
already-in-flight `loginStatus()` call, so it does not by itself close the
race, and it would have changed the call-count assertion in the existing
`does not erase a key typed on another provider when an abandoned Codex poll
returns` test (which relies on the poll still ticking once after the writer
leaves). Gating the write keeps that test's behavior — and the RPC call count
— unchanged.

**Also fixed.** The mount effect's `.catch(() => {})` on `login_status` swallowed
every rejection, including an unreachable engine, leaving Settings silent with
just a sign-in button. Changed to `.catch((e) => { if (!cancelled) setError(e); })`,
mirroring the poll's own rejection handler and the same `cancelled` guard the
effect's `.then` already used.

### New test

`does not resurrect a stale Codex failure when the writer returns before the
orphaned poll resolves` — starts a login, switches to Anthropic, switches back
to Codex (asserting no banner from the fresh fetch), then advances the fake
timer by 1500ms so the poll's tick resolves with a stale `login_failed: true`
and asserts the banner still does not appear.

Run against `git show 686fcb4:.../ProviderSection.tsx` swapped into a scratch
copy outside the repo (`node_modules` symlinked in, `./node_modules/.bin/vitest
run ProviderSection` run directly there):

```
 ❯ src/components/settings/ProviderSection.test.tsx (27 tests | 1 failed) 298ms
   × ProviderSection > does not resurrect a stale Codex failure when the writer returns before the orphaned poll resolves 6ms
     → expected <p class="sd" role="alert" …(1)></p> to be null

 Test Files  1 failed (1)
      Tests  1 failed | 26 passed (27)
```

Only the new test fails against the pre-fix component; all 26 pre-existing
tests still pass against it, confirming the new test is isolated to this
finding.

### Mutation re-check (scratch copies outside the repo, `pnpm`'s node_modules
symlinked in, `./node_modules/.bin/vitest run ProviderSection` run directly
to skip pnpm's install-reconciliation)

1. Removing `s.logged_in` from the stop condition (`if (s.logged_in ||
   s.login_failed)` → `if (s.login_failed)`):
   ```
      × ProviderSection > opens the browser and polls until Codex reports signed in
      × ProviderSection > reports a reload that fails after a successful login
      Tests  2 failed | 25 passed (27)
   ```
   Two tests fail, not one. Re-ran the identical mutation against the
   unmodified `686fcb4` component (before any of this round's changes) and got
   the same two failures — this is pre-existing shared coverage between those
   two tests, not something introduced by this fix. The finding's four
   mutations describe only the `stops polling when Codex reports a failed
   login` line item as depending on `s.logged_in`; `reports a reload that
   fails after a successful login` also happens to depend on it (a `logged_in`
   response is what that test uses to reach its `refresh()`-rejects-with-error
   path). Reporting the actual result rather than the expected one.
2. Removing `s.login_failed` (`if (s.logged_in || s.login_failed)` →
   `if (s.logged_in)`):
   ```
      × ProviderSection > stops polling when Codex reports a failed login
      Tests  1 failed | 26 passed (27)
   ```
   Exactly the predicted test, alone.
3. Removing `stopPolling()` from the `login_status` rejection handler:
   ```
      × ProviderSection > stops the poll and says why when login_status itself fails
      Tests  1 failed | 26 passed (27)
   ```
   Exactly the predicted test, alone.
4. Neutering the unmount cleanup effect (`useEffect(() => stopPolling, ...)` →
   `useEffect(() => () => {}, ...)`):
   ```
      × ProviderSection > clears the interval on unmount so an abandoned login stops calling the engine
      Tests  1 failed | 26 passed (27)
   ```
   Exactly the predicted test, alone.

File restored to the fixed state after each mutation and re-verified green
(27/27) before moving to the next.

### Verification

```
cd apps/desktop && pnpm test ProviderSection
```
```
 ✓ src/components/settings/ProviderSection.test.tsx (27 tests) 306ms

 Test Files  1 passed (1)
      Tests  27 passed (27)
```

```
make test-desktop
```
```
$ eslint src --max-warnings 28
✖ 28 problems (0 errors, 28 warnings)      [same pre-existing `any` warnings]

 Test Files  61 passed (61)
      Tests  303 passed (303)

$ tsc -b && vite build
✓ 1928 modules transformed.
✓ built in 1.32s
```

`go test ./...` not run, per the instructions: no Go changed, and this Mac's
locked keychain hangs the engine fixtures.
