# Task 5 report: consent and the connection test it unlocks

## What was implemented

`apps/desktop/src/components/settings/ProviderSection.tsx` gained the
per-provider consent checkbox and the connection-test button it gates,
rendered between the model field and the section-level error, matching the
order the brief laid out (5-1 consent, 5-2 test):

- `nameKeyFor(id)` — a small helper looking up a provider's `labelKey` from
  the existing `PROVIDER_ORDER` table, so the consent sentence can name the
  company without a second source of truth for provider display names.
- `<input type="checkbox" data-testid="provider-consent">` bound to
  `current?.consented`, `disabled={busy}`. Its label is
  `t("settings.providers.consent", { provider: t(nameKeyFor(active)) })` —
  the sentence names the provider per design spec 5.4, via i18n
  interpolation rather than a generic "use AI features" string.
- `saveConsent(consented)` — guarded, writes exactly
  `settings.set({ providers: { [active]: { consented_at: consented ? Date.now() : 0 } } })`
  then refreshes. `0` is the revoke path; this field is
  `providers[id].consented_at`, never the dead
  `ai_data_sharing_consent_version` / `ai_data_sharing_consented_at` pair
  that lives elsewhere in settings and that nothing in the engine or the app
  reads.
- `<button data-testid="provider-test">`, `disabled={busy || !current?.consented || !current?.configured}`.
  Both gates are enforced, not just consent — `providers.test` goes through
  `Source.Client()` on the engine, which requires `Configured()` **and**
  `Consented()`, so a credential-less click would fail just as surely as a
  consent-less one. The button is unclickable rather than clickable-then-
  informative for either reason.
- `runTest()` — guarded, calls `providers.test(active)`. Success sets
  `testResult = "ok"` (rendered as `provider-test-ok`); failure sets
  `testError` (rendered as `provider-test-error`, `role="alert"`,
  `rpcErrorMessage(testError, t)`) and does **not** rethrow, so it never
  reaches the section-level `error`/`provider-error` — same reasoning as
  Task 4's model-list failure: a failed test is information about the
  provider, not a broken pane.
- A new `useEffect` keyed on `[active]` (not `[active, list]`, and not a
  dependency of `refresh()`'s output) clears `testResult`/`testError` on
  every provider change, so a passing tick from one provider cannot survive
  onto another's screen.

`apps/desktop/src/lib/i18n.tsx` gained three keys in all three catalogues
(`ko`, `en`, `ja`) under `settings.providers.*`:
`settings.providers.consent` (carries a `{provider}` placeholder),
`settings.providers.test`, `settings.providers.test.ok`. The existing
`errors.provider*` reason-code mappings (`providerAuthFailed`,
`providerConsentRequired`, `providerRateLimited`, `providerUnreachable`,
`providerNotConfigured`) were already present from earlier tasks and needed
no changes — `rpcErrorMessage` covers `provider_consent_required` even
though the UI now makes that particular reason unreachable by design (the
button being disabled pre-empts it).

`apps/desktop/src/components/settings/ProviderSection.test.tsx` gained:
- A `providersTest: vi.fn()` mock wired into the `providers` mock object,
  with a default `mockImplementation(() => Promise.resolve({ ok: true }))`
  in `beforeEach`.
- A test for the Task 2 deferred finding: `provider-state`'s
  `notConfigured`/`notConsented` branches, previously exercised nowhere —
  every existing test that touched that testid used the true/true override.
- A `describe("consent and connection test", ...)` block with the six cases
  the brief named plus one more (`enables the connection test once both
  consent and a credential are present`, the positive case complementing the
  two disabled-state tests) and one pinning the exact interpolated label
  text (`names the provider in the consent label`).
- `afterEach` now also calls `vi.restoreAllMocks()` (kept alongside the
  existing `vi.useRealTimers()`) — the consent-patch test spies on
  `Date.now()` to pin the exact timestamp in the patch, and without
  restoring it that mocked "now" would otherwise leak into every later test
  in the file.

## Deviation from the brief

None in substance. Two additions beyond the brief's six listed test names,
both purely additive coverage (see above) — not a deviation, since the
brief's list was described as "the tests" the feature needs, and the extra
two close gaps in that list rather than replacing anything in it. Also
kept `runTest()` free of a trailing `refresh()` call exactly as spec'd —
a connection test does not change engine-side provider state, so there is
nothing for a reload to pick up.

## Commands run

### Failing tests first (before implementation)

```
$ pnpm test ProviderSection -- --run
...
 ❯ src/components/settings/ProviderSection.test.tsx:927:42
    925|       render(<ProviderSection />);
    926|
    927|       await userEvent.click(await screen.findByTestId("provider-test")…
       |                                          ^
    928|
    929|       const error = await screen.findByTestId("provider-test-error");

 Test Files  1 failed (1)
      Tests  8 failed | 38 passed (46)
```

All 8 new tests failed (missing `provider-consent`/`provider-test`
testids); the 38 pre-existing tests were untouched and stayed green,
confirming the failure was localized to the new behavior.

### After implementation

```
$ cd apps/desktop && pnpm test ProviderSection -- --run
 ✓ src/components/settings/ProviderSection.test.tsx (46 tests) 472ms

 Test Files  1 passed (1)
      Tests  46 passed (46)
```

### Full verification (from repo root)

```
$ make test-desktop
...
 Test Files  61 passed (61)
      Tests  322 passed (322)
...
$ tsc -b && vite build
✓ 1928 modules transformed.
...
✓ built in 1.34s
```

`pnpm lint` (part of `make test-desktop`) reported 0 errors, 28 pre-existing
`no-explicit-any` warnings in unrelated editor files (same count as before
this change — verified none are new).

Per the task instructions, `go test ./...` was **not** run (no Go touched;
this Mac's locked keychain hangs pre-existing engine fixtures).

## Self-review findings

- Checked the consent patch shape is pinned exactly (`providers[id].consented_at`,
  never `ai_data_sharing_consent_*`) via an assertion on the literal object
  passed to `settingsSet`, using a `Date.now()` spy so the timestamp itself
  is asserted rather than just its presence/shape.
- Checked the revoke path writes `0`, not `undefined`/omitted — the engine
  overwrites `consented_at` wholesale, so an omitted field would leave a
  stale prior consent standing.
- Checked the test button's two independent disable conditions
  (consent-only-missing, credential-only-missing) as separate cases, plus
  the positive case where both are present — confirms neither gate was
  dropped nor inverted.
- Checked the failed-test rendering uses `rpcErrorMessage` (translated),
  carries `role="alert"`, and does not touch `provider-error`.
- Checked the provider-change effect clears `testResult` specifically
  (rather than only `testError`) by asserting `provider-test-ok` disappears
  after switching from a passing Anthropic run to Gemini.
- Added the `provider-state` `notConfigured`/`notConsented` coverage named
  as a deferred Task 2 finding in the brief; ran it standalone first to
  confirm it would have passed even against a hardcoded happy-path string
  is not something I can prove without a hostile diff, but the assertion
  checks both halves of the string, which a hardcoded `configured`/
  `consented` string would fail.
- No changes were made under `engine/` or `apps/desktop/src-tauri/`, and
  `ProviderSection` was not wired into `Settings.tsx` (that is Task 6).
- Verified `pnpm lint`'s 28 warnings are pre-existing (all in
  `FocusExtension.ts`, `MentionExtension.ts`, `SearchHighlightExtension.ts`,
  `Tiptap.tsx`, `autoMention.ts`/`.test.ts` — none in files this task
  touched).

---

## Fix round: review findings F1–F9

The shipped behaviour of the consent gate was correct, but the review found
the test net had holes wide enough for a specific wrong implementation, plus
two user-visible defects in the consent sentence itself. Nine findings, all
addressed here.

### What changed in the product

**F9 — the sentence now names the destination, not the protocol.**
`settings.providers.name.openai` is "OpenAI 호환" / "OpenAI-compatible" /
"OpenAI 互換", so the label read "…will be sent to OpenAI-compatible": broken
English, and silent about where the scenes actually go. For that provider the
destination is the writer's own `base_url` — OpenRouter, or a local Ollama
that never leaves the machine. `ProviderSection.tsx` now derives the
interpolated value:

```tsx
const consentDestination =
  active === "openai"
    ? current?.base_url?.trim() || t("settings.providers.consent.customEndpoint")
    : t(nameKeyFor(active));
```

The *saved* `base_url` is used, not `baseUrlDraft`: a half-typed URL must not
appear in the sentence the writer is consenting to. The new
`settings.providers.consent.customEndpoint` key covers the not-yet-configured
case in all three catalogues ("직접 설정한 OpenAI 호환 엔드포인트" / "the
OpenAI-compatible endpoint set above" / "上で設定した OpenAI 互換エンドポイント").

**F8 — the Korean particle is gone rather than made conditional.**
`{provider}으로` agrees with a consonant-final syllable; "Google Gemini"
reads 제미나이, so "Google Gemini으로" is ungrammatical — and after F9 the
placeholder can hold a URL, where no particle rule would be reliable at all.
The sentence now uses `에`, which has no allomorph:
`"씬 원문이 {provider}에 전송되는 데 동의합니다"`. English and Japanese were
already destination-agnostic ("sent to {provider}" / "{provider}に送信") and
needed no rewording; they only gained the new key.

**F3 — a passing test result no longer outlives what it depends on.**
The clear-effect keyed on `[active]` alone, so after a green run on Anthropic,
unticking consent (or clearing the key) left "Connected" on screen. The button
correctly disabled, which made the stale tick worse rather than better: the
writer reasonably reads it as "the connection is still live". It now keys on
the facts:

```tsx
const consented = Boolean(current?.consented);
const configured = Boolean(current?.configured);
useEffect(() => { setTestResult(null); setTestError(null); }, [active, consented, configured]);
```

Deliberately three primitives and an id, never `list` — see F5 below. The
checkbox and test button were switched to the same two consts so there is one
reading of consent state in the component rather than three.

### What changed in the test net

| Finding | Test added |
| --- | --- |
| F1 | `it.each` over all five destinations (four ids + `openai` with a base URL) |
| F2 | mocked `t` now echoes `name=value`; new test runs the **real** `translate()` |
| F3 | "clears a passing test result when consent is revoked" / "…when the key is cleared" |
| F4 | "drops a stale result before showing the next one" (both directions) |
| F5 | "keeps a passing test result across an unrelated reload" |
| F6 | "keeps the checkbox ticked after consent is saved" |
| F7 | "locks the consent box and the test button while a call is in flight" |
| F8 | "keeps the Korean sentence grammatical after a vowel-final name" |
| F9 | "names the configured endpoint, not the protocol, for openai" |

A `persistWrites()` helper makes `settings.set` do the engine's half of a
consent or key write, so the next `providers.list` reflects it. Without that,
F3 and F6 cannot be observed at all: both the tick and the disable state are
derived from `list`, which is exactly why `await refresh()` has to be there.

**How F2 was closed — both halves, deliberately.** The finding offered two
options; neither alone covers both mutants, so both were done:

- The mocked `t` joined `Object.values(vars)`, discarding the variable *name*.
  It now emits `${key}:${name}=${value}`. That pins the **component's** half:
  renaming its interpolation key to `{ name: ... }` now fails five tests.
- The mock cannot see the catalogue at all, and the parity test only compares
  ko↔en↔ja, so renaming the placeholder to `{name}` in all three catalogues
  was invisible. A new test pulls the **real** `translate` via
  `vi.importActual` and asserts, for each language, that the substitution
  actually happens and no `{` survives. Verified: under that mutation the
  catalogue parity test still passes and this one fails.

### Verification — every finding fails against HEAD or its named mutant

Run in a copy of `apps/desktop` **outside the repo**
(`…/scratchpad/harness/desktop`, sources copied, `node_modules` symlinked),
with the new test file held constant and the sources swapped underneath it.

**Baseline: the pre-fix `bc4a2d6` sources + new tests** — F3, F8, F9:

```
   × … > names openai's destination in the consent label 5ms
   × … > names openai's destination in the consent label 2ms
   × … > names the configured endpoint, not the protocol, for openai 4ms
   × … > keeps the Korean sentence grammatical after a vowel-final name 1ms
   × … > clears a passing test result when consent is revoked 1026ms
   × … > clears a passing test result when the key is cleared 1032ms
      Tests  6 failed | 53 passed (59)
     → Google Gemini: expected '씬 원문이 Google Gemini으로 전송되는 데 동의합니다' not to contain 'Google Gemini으로'
```

**F1 — the reviewer's mutant** (`consentDestination`'s non-`openai` branch
replaced with a literal `t("settings.providers.name.anthropic")`), which used
to pass 46/46:

```
⎯⎯⎯⎯⎯⎯⎯ Failed Tests 2 ⎯⎯⎯⎯⎯⎯⎯
 FAIL  … > names openai-codex's destination in the consent label
 FAIL  … > names gemini-native's destination in the consent label
AssertionError: expected 'settings.providers.consent:provider=s…' to be 'settings.providers.consent:provider=s…'
Expected: "settings.providers.consent:provider=settings.providers.name.openai-codex"
Received: "settings.providers.consent:provider=settings.providers.name.anthropic"
      Tests  2 failed | 57 passed (59)
```

**F2a — component interpolates `{ name: … }`, catalogues unchanged:**

```
   × … > names openai-codex's destination in the consent label 4ms
   × … > names anthropic's destination in the consent label 1ms
   × … > names gemini-native's destination in the consent label 3ms
   × … > names openai's destination in the consent label 1ms
   × … > names openai's destination in the consent label 3ms
      Tests  5 failed | 54 passed (59)
```

**F2b — placeholder renamed to `{name}` in all three catalogues, component
unchanged.** Note the catalogue parity test is in this run and passes, which
is the point:

```
   × … > substitutes the destination into the real consent sentence 8ms
     → ko: expected '씬 원문이 {name}에 전송되는 데 동의합니다' to contain 'Acme Models'
   × … > keeps the Korean sentence grammatical after a vowel-final name 1ms
     → Google Gemini: expected '씬 원문이 {name}에 전송되는 데 동의합니다' to contain 'Google Gemini'
      Tests  2 failed | 59 passed (61)
```

**F4 — `setTestResult(null); setTestError(null);` deleted from `runTest`:**

```
   × … > drops a stale result before showing the next one 24ms
     → expected <span role="alert" …(1)></span> to be null
      Tests  1 failed | 58 passed (59)
```

**F5 — clear-effect deps changed to `[active, list]`:**

```
   × … > keeps a passing test result across an unrelated reload 48ms
     → Unable to find an element by: [data-testid="provider-test-ok"]
      Tests  1 failed | 58 passed (59)
```

**F6 — `await refresh()` deleted from `saveConsent`:**

```
   × … > clears a passing test result when consent is revoked 1027ms
     → expected <span …(1)></span> to be null
   × … > keeps the checkbox ticked after consent is saved 1021ms
     → expect(element).toBeChecked()
      Tests  2 failed | 57 passed (59)
```

**F7 — `busy` removed from the checkbox** (and, separately, from the test
button); both mutants fail the same test:

```
   × … > locks the consent box and the test button while a call is in flight 12ms
     → expect(element).toBeDisabled()
      Tests  1 failed | 58 passed (59)
```

### Full verification (HEAD of this branch)

```
$ cd apps/desktop && pnpm test ProviderSection
 ✓ src/components/settings/ProviderSection.test.tsx (59 tests) 657ms

 Test Files  1 passed (1)
      Tests  59 passed (59)
```

```
$ make test-desktop
✖ 28 problems (0 errors, 28 warnings)
 Test Files  61 passed (61)
      Tests  335 passed (335)
✓ built in 1.40s
```

46 → 59 tests in this file; 322 → 335 across the desktop suite. `pnpm lint`
warnings are unchanged at 28 (all pre-existing `no-explicit-any` in editor
files). Per the task instructions `go test ./...` was **not** run.

### Contracts confirmed intact

All were re-checked against the passing suite, none were touched:

- Drafts reset when the provider changes, never on a `list` re-fetch
  (`if (draftProvider.current === active) return;`).
- The Codex poll stays gated by the monotonic `pollGenRef`.
- Consent is still written to `providers[id].consented_at` (epoch-ms, `0`
  revokes) — never `ai_data_sharing_consented_at`.
- The test button still requires consent AND a credential (a server contract
  from `Source.Client()`), now read through the `consented`/`configured`
  consts rather than three separate `current?.` reads.
- Save still disabled on an empty key draft; empty-string `api_key` still
  deletes the secret; `base_url` still never rides into a non-`openai` patch;
  no stored key reaches the input.

### Residual concerns

- The consent sentence for `openai` now renders a raw `base_url`. It is React
  text content (escaped), and it is the truthful destination, but a very long
  URL will wrap inside the checkbox label. Left as-is: truncating the address
  the scenes are being sent to would defeat the sentence's purpose.
- If the writer types a new base URL and ticks consent without blurring the
  field first, the sentence still names the previously saved endpoint. That is
  the honest reading — consent applies to what the engine has stored — but it
  is a seam worth watching once Task 6 wires this into `Settings.tsx`.

## Round 2: the F9 fix outlived a moving destination

The re-review found one Important issue in the round-1 diff itself: F9 made
the `openai` consent sentence name the live `base_url`, but the clear-effect
(F3/F5) was still keyed on `[active, consented, configured]` — an id and two
booleans that do not move when `base_url` changes. So editing the base URL
(or rotating an API key while one was already stored) left a "Connected"
badge standing next to a destination or credential it was never run against.

### The fix

`ProviderSection.tsx:174-199`: the effect now also depends on `baseUrl`
(`current?.base_url ?? ""`, directly on `ProviderStatus`) and on a new
`credentialEpoch` counter:

```tsx
const consented = Boolean(current?.consented);
const configured = Boolean(current?.configured);
const baseUrl = current?.base_url ?? "";
useEffect(() => {
  setTestResult(null);
  setTestError(null);
}, [active, consented, configured, baseUrl, credentialEpoch]);
```

`baseUrl` closes the endpoint-swap case directly — it is server state, so
any refresh that reports a different endpoint retires the result.

The API key has no equivalent: `settings.get`/`providers.list` expose only
`configured` (`api_key_set`), never the key's value, so there is no key
*identity* this effect could ever compare — `configured` stays `true` across
a rotation because a key is stored both before and after. Rather than invent
a proxy off server state that would silently miss the same case `configured`
does, `credentialEpoch` is a local counter bumped only inside `saveKey` and
`clearKey` — the two places this pane itself changes the key — after the
write succeeds:

```tsx
setCredentialEpoch((n) => n + 1);
```

This closes the case that matters (the writer rotates the key through this
screen) and is honest about what it does not close: a key changed some other
way while this pane stays mounted (hand-editing the settings file, a second
window) is invisible to it, because nothing on `ProviderStatus` would let any
dependency array see that either. The comment above the effect spells this
limitation out rather than implying `configured` (or anything else) covers
it.

### Tests added (fail against the pre-fix component)

Two tests were added to the "consent and connection test" block and verified
to fail against `git show 5583d8b:.../ProviderSection.tsx` swapped into a
scratch copy outside the repo (a fresh `pnpm install` was required there —
straight `cp -R` of `node_modules` produced spurious `@testing-library/
jest-dom` matcher failures unrelated to this change, confirmed by reproducing
the same failure against the current, already-passing HEAD copy):

```
 FAIL  … > clears a passing test result when the base URL changes
   AssertionError: expected <span …></span> to be null
 FAIL  … > clears a passing test result when the key is rotated to a different one
   AssertionError: expected <span …></span> to be null
 Tests  2 failed | 59 passed (61)
```

`persistWrites()` was extended to also apply a `base_url` patch to
`rowExtras`, so the base-URL test can drive a real `saveBaseUrl` → refresh
round trip the same way the existing key/consent tests do.

### Full verification (HEAD of this branch)

```
$ cd apps/desktop && pnpm test ProviderSection
 ✓ src/components/settings/ProviderSection.test.tsx (61 tests) 795ms
 Test Files  1 passed (1)
      Tests  61 passed (61)
```

All six round-1 tests named in the fix-round-2 brief were confirmed passing
by name (`--reporter=verbose`): "clears a passing test result when consent
is revoked", "…when the key is cleared", "keeps a passing test result across
an unrelated reload", "drops a stale result before showing the next one",
"keeps the checkbox ticked after consent is saved", "locks the consent box
and the test button while a call is in flight".

```
$ make test-desktop
 Test Files  61 passed (61)
      Tests  337 passed (337)
✓ built in 1.38s
```

59 → 61 tests in this file; 335 → 337 across the desktop suite. Per the task
instructions, `go test ./...` was not run.

### Contracts reconfirmed intact

Same list as round 1, re-checked against the passing suite: drafts reset
only on a provider change; the Codex poll stays gated by `pollGenRef`;
consent still writes `providers[id].consented_at` (never the dead
`ai_data_sharing_consented_at`); the test button still requires consent AND
a credential; the consent sentence still names the real destination (both
the mocked-`t` and real-`translate()` tests pass); save/clear/base_url
draft-isolation behavior is unchanged.

### Residual concerns

- `credentialEpoch` only sees a key change made through this pane. A key
  edited outside it (settings file, a second window) while this screen stays
  mounted would still show a stale "Connected" — the same class of gap the
  original finding describes, just one step further out. Closing that fully
  would need the engine to expose some key identity (e.g. a fingerprint) on
  `ProviderStatus`, which is out of scope here.
