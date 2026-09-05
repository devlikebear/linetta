# App Store Description — Draft (1.2.0)

This is a draft for the Mac App Store listing, written ahead of the 1.2.0
release. It is not published anywhere and nothing here is live copy yet —
revise it at release time (in particular, verify Codex sign-in against a real
ChatGPT account first; see the plan's known gaps) and adapt it if it is reused
for a non-store channel. See "Scope notes" below for why it is written the way
it is.

---

## Description

Linetta is a local-first app for planning and writing long-form fiction: plan
your story, keep your world consistent, and write scene by scene. Everything
you write is stored in a local database on this device — Linetta needs no
account and no sign-in to use it as a writing tool on its own.

When you want to write alongside a model, connect one yourself: sign in with
your ChatGPT account, or add an API key for Anthropic, Google Gemini, or an
OpenAI-compatible endpoint. Linetta does not provide an AI service of its
own — it opens a panel where you write alongside the model you connected, on
your own manuscript. Every tool call it makes is recorded in an activity log,
and every change it makes to your manuscript text is saved as a version you
can restore. Agreeing to send your writing to a provider is a separate choice
for each one, and can be withdrawn at any time.

If you already run an AI agent of your own — Claude Code, Claude Desktop, or
another MCP client — you can connect it to Linetta instead of, or alongside,
the built-in one. Both reach your manuscript through the same set of tools, so
what one can read or change, the other can too.

Linetta has no user accounts, no analytics, and no advertising. The developer
runs no server for the app and does not collect what you write. Your writing
stays in a local database on your device, and it leaves that device only
along paths you turn on yourself: sending it to the AI provider you
connected, letting an agent you run read it, or copying it into a folder you
choose to sync into.

---

## Scope notes (not part of the listing text)

- **Why the Mac App Store, specifically.** This app also ships Windows and
  Linux builds, but those are installers from GitHub Releases, not listings
  under a store's own review — so I wrote this for the one channel that is
  actually store-reviewed. That choice affects the text: the Mac App Store
  build does not include GitHub Sync (`docs/privacy-policy.md` §3.3 says so
  explicitly — every other desktop build has it, the App Store build does
  not), so I left "pushing to a git remote" out of the "leaves this device"
  list. Folder Sync and MCP are both in the App Store build (§3.2, §3.4), so
  those stayed in. If this draft is reused for the Windows/Linux download
  page instead of a store listing, add GitHub Sync back in.

- **No numeral in the "leaves this device" paragraph.** That list is the one
  the privacy policy got wrong twice this week by stating an exhaustive
  count (two, then three, before landing on an open enumeration with no
  number at all — see `docs/privacy-policy.md` §3). I wrote this paragraph
  the same open-ended way on purpose, so a future path being added or a
  build variant changing what's included doesn't leave a stale count sitting
  in store copy that nobody remembers to fix.

- **The Linux secure-store gap is not mentioned.** On Linux there is no OS
  secure store, so the three API-key providers can't be configured there and
  only the ChatGPT sign-in works (`docs/privacy-policy.md` §2). That is real
  and worth documenting somewhere — the README already does — but it is a
  Linux-only limitation and this text is for the Mac App Store listing, where
  it doesn't apply and would only confuse an Apple reviewer or a macOS
  buyer. If this draft becomes general marketing copy shown to Linux users
  (a downloads page, a README-adjacent doc), add a line back in there
  instead of here.

- **"Restore" is scoped to manuscript text on purpose.** The agent's writes to
  scene prose are individually snapshotted and restorable
  (`engine/internal/mcphost/tools_write.go`,
  `apps/desktop/src/components/VersionSheet.tsx`); some other things it can
  change through `linetta_apply_story_ops` — individual fact-card, character,
  and relationship edits — are logged in the activity log but are not
  automatically undoable. The text above says "your manuscript text," not
  "every change," to avoid implying an undo path that doesn't exist for those.

- **No model names, prices, or screenshots**, per the brief for this task.
