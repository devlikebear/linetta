<div align="center">
  <img src="apps/desktop/src-tauri/icons/icon.png" width="112" alt="Linetta app icon" />

# Linetta

### A calm, local-first writing studio for long-form fiction

Plan your story, keep your world consistent, and write scene by scene. When you want to write alongside AI, use Linetta's built-in agent or connect one you already run over MCP — neither happens on its own.

[![Latest release](https://img.shields.io/github/v/release/devlikebear/linetta?style=flat-square)](https://github.com/devlikebear/linetta/releases/latest)
[![Build](https://img.shields.io/github/actions/workflow/status/devlikebear/linetta/ci.yml?branch=main&style=flat-square&label=build)](https://github.com/devlikebear/linetta/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/devlikebear/linetta?style=flat-square)](LICENSE)

[Website](https://linetta.marvin-42.com) · [Mac App Store](https://apps.apple.com/app/id6781664781) · [Download for Windows or Linux](https://github.com/devlikebear/linetta/releases/latest) · [Report an issue](https://github.com/devlikebear/linetta/issues)

English · 한국어 · 日本語
</div>

![Linetta editor](docs/assets/screenshots/workspace.png)

## One workspace for the whole story

Linetta is made for novelists and web-fiction writers who want their manuscript, outline, characters, places, relationships, and research close at hand — without turning writing into project management.

- **Write with focus.** Work scene by scene in a quiet editor with your outline always within reach.
- **Keep the story consistent.** Organize characters, places, relationships, storylines, beats, summaries, and memories beside the manuscript. Agents can check a scene against all of it, and what they find or propose waits for your decision.
- **Stay in control of AI.** Linetta calls a model only if you connect a provider — for the built-in agent, or your own agent over MCP. The activity log shows what changed and whether the built-in agent or an external client made the call, a recent structural change can be undone in one click while the app stays open, and the text an agent replaces is kept as a restorable version of the scene.
- **See your story.** Optionally draw illustrations with ChatGPT or Gemini from your scenes and character sheets. Earlier pictures go along so faces stay the same, and nothing is drawn until you press Generate.
- **Keep your work local.** Projects live in a local SQLite database with version snapshots and daily backups. No Linetta account or mandatory cloud is required.
- **Publish when you choose.** Optionally connect an account on [Linetta Works](https://works.marvin-42.com) and put an episode up for readers straight from the app. Only the episodes you publish — and the Story World sheets and cover you choose to make public — leave your device. Share a published episode to X, Threads, Naver Blog, KakaoTalk, or Instagram from the publish dialog.
- **Carry a work between devices.** If the site's operator has approved your account, the private vault keeps a work sealed under a recovery key only you hold, so you can pick it up on another computer. It is a free beta and never your only copy.
- **Move your manuscript freely.** Import and export Markdown, and optionally sync exported work through Git.

## See Linetta in action

| The work's own record | Research without leaving the scene |
| --- | --- |
| Story World lists the characters, places, items and concepts your work has registered — including the ones an agent created, whether the built-in agent or one connected over MCP. | Fact Book keeps source-backed notes next to the writing that needs them. |
| ![Linetta Story World](docs/assets/screenshots/story-world.png) | ![Linetta Fact Book](docs/assets/screenshots/fact-book.png) |

Your projects stay organized in a library built for multiple works:

![Linetta project library](docs/assets/screenshots/library.png)

## Get Linetta

### Website

Product overview and downloads: [linetta.marvin-42.com](https://linetta.marvin-42.com).

### Mac App Store

Linetta is free on the [Mac App Store](https://apps.apple.com/app/id6781664781).

### Homebrew on Apple Silicon

The direct macOS build is signed with an Apple Developer ID and notarized by Apple.

```sh
brew install --cask devlikebear/tap/linetta
```

Linetta updates itself from then on: choose **Check for Updates…** in the
Linetta menu or the tray menu. It also checks once after launch and asks before
downloading anything. `brew upgrade --cask linetta` keeps working too.

### Windows and Linux

Every [GitHub release](https://github.com/devlikebear/linetta/releases/latest) includes Windows NSIS and MSI installers plus Linux AppImage, `.deb`, and `.rpm` packages.

These builds update themselves the same way, from **Check for Updates…** in the
tray menu. The Mac App Store build is updated by the App Store instead.

Intel Mac users can [build from source](#build-from-source).

## Writing with the built-in agent (BYOK)

Linetta has a built-in writing agent, but it has nothing to call until you
bring your own connection to a model. Five providers are supported: ChatGPT
(Codex) by signing in with your ChatGPT account; Claude Code, which runs the
`claude` CLI you already have installed under your own Claude sign-in, with no
API key (Homebrew and direct-download builds only, not the Mac App Store
build); and Anthropic, Google Gemini, or an OpenAI-compatible endpoint —
OpenRouter, or a model running on your own machine — by API key.

Turn it on in **Settings → AI provider**. Consent is per provider, and it
gates even the connection test: that button is refused until you have given
that provider a credential and ticked its consent box. The one request that
runs on a credential alone is asking the provider which models it offers, so
the picker has something to list; that request carries no manuscript text.

An API key goes into your OS's secure credential store, never into
`settings.json`; signing in with Codex instead stores its tokens in Linetta's
own data directory, in a file only your account can read.

Linux has no secure credential store backend, so Anthropic, Google Gemini,
and the OpenAI-compatible endpoint cannot be configured there. Signing in
with a ChatGPT account still works, since that path stores tokens in
Linetta's data directory instead, and so does the Claude Code provider, which
needs no credential from Linetta at all.

Open the agent with the toolbar **Agent** button or `Cmd/Ctrl+J`; connect a
provider first, or the panel prompts you to set one up. Two chips under the
input box show the provider and model the next turn will use, and you can
switch either one there between turns. It reaches Linetta's tools the same way an
external MCP client does, and every call it makes is recorded in the MCP
activity log, which shows whether the built-in agent or an external client
made it. A structural change — outline restructuring and the like — gets an
Undo button on its own line in the agent panel, good for one click while the
app stays open; only the last eight are held, in memory, and none survive a
restart. A scene the agent wrote can be reverted the same way from its tool
line (the panel offers undo whenever a line carries a `batch_id` or a
`snapshot_id`). Linetta also snapshots the scene before every agent write and
keeps that version indefinitely, so you can put the old text back from the
scene's **Previous versions** sheet.

To ask about a passage, select it in the editor and choose **Ask the agent**
from the selection menu. The agent panel opens with the selected paragraphs
quoted above the input box; remove them with ✕, or send a different selection
to replace them. With a quote attached and the input empty, quick chips fill
in a request to polish, comment on or expand it, and you press send yourself.
The quote goes with your message and stays in the conversation, so a follow-up
such as "make it shorter" still knows which paragraph you meant. Asked only a
question, the agent changes nothing.

When only you can settle something, the agent asks on a card instead of
listing options in its reply. Each question shows its choices, with the one
the agent recommends marked, and room for your own answer; some let you pick
several. Answer them all and press **Send answers**, and they go out together
as your next message. The turn ends at the card, so nothing is waiting on you:
it stays open across restarts until you answer it or type something else.

To show the agent a picture, attach up to four PNG, JPEG, WebP, or GIF images
to a message with the button beside the input box, by pasting, or by dropping
them on it. Each can be up to 5 MB, and large photos are scaled down before
sending. The images go to the model with that message only; the conversation
history keeps a note of how many there were, not the pictures. The Claude Code
provider takes text only and says so. Ask the agent to keep an attached image
as a character's reference and it saves it to that character's visual sheet.

### The story ledger and parts

A long work needs more kept than a brief can quote. Each scene has a **plan**,
separate from its summary, so writing the scene does not erase what it was meant
to do. Under Story World → Story ledger, a work keeps what it owes the reader
(foreshadowing, running gags, open questions) and what is settled (decisions,
numbers, the timeline, who knows what). Agents record entries as they write, with
`linetta_edit_ledger`; a quote has to be in the scene it names, or the entry is
refused. You can add, drop or delete any of it. Each part can have a role, a goal
and a target tension, set there or with `linetta_set_part_arc`.

The brief carries the current part's job, how many scenes are left in it, what is
due now, what is still owed and what is settled. It never reads ahead: nothing
planted after the open scene appears. If a change to the outline deletes a
planned episode, its plan moves to the ledger's cut plans. The ledger and part
arcs are in backups and travel with a restored work.

### Reviewing finished episodes

The agent reads an episode as a new reader before handing it over. "Review this
episode" and "Critique this part" in the agent panel ask for it directly, and
findings land in the continuity list, which also takes motive, readability and
pacing. **Review finished episodes independently**, under Settings → Skills, adds
a separate provider call after a turn that wrote prose: it reads the last few
scenes with no memory of writing them, using read-only tools, and records what it
finds. It sends the manuscript again, so it is metered by your provider and is
off by default. It does not run with the Claude Code provider.

The style check also reports paired contrast and narrator explanation in Korean
prose, a sentence repeated across scenes, and consecutive scenes that end the same
way. It only reports them. You can name one scene as the style to match, and keep
a list of phrases to avoid in every work under Settings → Skills.

### Your direction

A work has a **current focus** — what to concentrate on for the next few
episodes — and any scene can have its own **intent**: a one-line goal, what
to keep, what to avoid. Both are yours to write; no agent tool can set
either. They sit at the very top of the story brief, ahead of everything
else, under a "Writer's Direction" heading the built-in agent treats as
binding — and the same brief is what a connected MCP client reads too, read-only
mode included. Set the focus and it is three episodes' worth of actually-
written scenes later, the app and the brief both flag it as possibly stale
and ask you to confirm it still holds.

If you have picked a branch in **Where the story could go** (below), it
appears right after the scene intent, labelled a plan — never canon — with
its title, premise, and beats. If the outline, manuscript, Story World, or
your own direction changed since the branch was drawn, the brief says so
and asks the agent to check with you before following it. Turning the
Writer's Direction switch off in the AI context checklist removes the plan
along with the rest of the section.

### What the agent remembers

Linetta keeps two short documents an agent reads: a **writer profile** — how
you work, up to 1,400 characters — and **work notes** for each work, up to
2,200. Both are pasted whole into the built-in agent's system prompt at the
start of every turn, with a line saying how much of the budget is used, and
the agent records into them with its `linetta_edit_memory` tool. You can read
and rewrite both yourself in **Settings → Memory**, one work at a time.

**The writer profile is not scoped to the work you have open.** It is global,
and it is read for every work. Something an agent records about you while
working on one book is in the system prompt of every other book, and in the
story brief a connected MCP client receives on any work. Work notes stay with
their work.

Both live in `library.db`, so the daily backup carries them, and so does
restoring: **Settings → Backup**, which brings a single work back from a
backup as a new project, brings that work's notes back with it. The writer
profile is global and stays with the library — a per-work restore never
copies or duplicates it. The startup recovery screen, which appears only when
the app cannot open the library and replaces the whole database file, brings
both documents back.

### What the agent learns

Beside the memories, an agent keeps **skills**: short how-to documents about
method, not about the story — how you want a fight scene paced, how a
recurring character speaks. Facts about the world belong in the Fact Book and
in memory; skills hold procedures.

Each one is a plain `SKILL.md` file with agentskills.io YAML frontmatter, in
its own folder under your data directory:

```text
<app data>/skills/<name>/SKILL.md                     every work
<app data>/skills/works/<project id>/<name>/SKILL.md  one work
```

That is deliberate. You can open a skill in any editor, keep the folder in
git, or point your own Claude Code at it and have it find the shape it
expects. At most 40 skills per scope, a description of 200 characters and a
body of 8,000.

Only the **names and descriptions** of enabled skills go into the built-in
agent's system prompt and into the story brief an MCP client reads; the body is
fetched on demand with `linetta_read_skill`. The story brief lists every one.
The system prompt holds that list to about 3,000 characters and, when they do
not all fit, says how many it is showing.

**A skill under `skills/<name>/` is global**, the same way the writer profile
is. It is offered on every work, not only the one you have open. A skill under
`skills/works/<project id>/` stays with its work.

**An agent writes a skill without asking you first.** There is no approval
gate: `linetta_edit_skill` creates, patches and deletes, and the change is
saved. What you get instead is on the record and in your hands — every skill
carries a badge saying whether you or an agent wrote it, every write is kept
as a version you can read and revert to, and one switch turns a skill off so
it leaves the prompt without being deleted. All of it is in **Settings →
Skills**, where you can also write and edit skills yourself.

**The self-review is an extra model call.** After a turn in which the built-in
agent executed eight or more tool calls — and after your reply has already been
sent — Linetta asks the same provider and the same model, in a separate
request, whether that turn taught it a technique worth recording or showed a
skill to be wrong. Most turns answer "nothing" in one round trip; a turn that
does write a skill costs up to five. It is on by default and it is
metered by your provider like any other call. Turn it off under **Settings →
Skills → Learning from its own work** (`agent_self_review_enabled` in
`settings.json`) and Linetta does not make that call at all. The Claude Code
provider never runs it, since that second turn would spend your plan's usage
without you asking for it.

**You can switch the memory, skills, review, and visual editing tools off
entirely.** Every tool's full description travels in every request, so
Linetta's forty-three tools are a standing cost on every turn — and the memory
tool, the two skills tools, and the three check-and-propose tools together are
a meaningful share of it. **Settings → Tool budget** says how many tools the
agent currently gets and what each group costs, and turns each group off for
both the built-in agent and any MCP client (`memory_tools_enabled`,
`skill_tools_enabled`, and `review_tools_enabled` in `settings.json`; all on
by default, and `visual_write_tools_disabled`, false by default). Switching a
group off also removes the instructions that name its tools — a prompt telling
an agent to use a tool it has not been given is worse than the tool's cost.
Each switch takes effect on your very next message; there is nothing to
restart. With the skills tools off the self-improvement pass cannot run at
all, and the Skills pane says so.

The memory, skills, and review groups lose different things. With the memory
tool off, what is already recorded is still read back to the agent — it just
cannot add to it, and you can still edit it yourself under Settings → Memory.
With the skills tools off, the skill **list** goes too: a list with no
`linetta_read_skill` to open a body is only names. With the review tools off,
the continuity tools, `linetta_propose_story_ops`, the inbox tools,
`linetta_save_forecast` and their starter chips all go — what is already
recorded stays exactly where it is. Nothing is deleted either way, and switching a group back on restores
it exactly.

**A turn stops after a set number of tool calls.** The built-in agent makes at
most 48 tool calls in one turn by default. Change it under **Settings → Tool
budget → Tool calls per turn** (`agent_max_tool_calls` in `settings.json`,
8–200); a higher limit lets a long request finish in one turn, and also lets a
turn cost more. When a turn reaches the limit, or the same tool fails three
times in a row, Linetta makes one more request with tools switched off, so the
agent says what it finished and what is left. Send "continue" to pick up from
there. A changed limit applies from the next turn, not to one already running.
External MCP clients are not affected.

**The daily backup does not carry the skills folder.** It is `VACUUM INTO` on
`library.db` and copies nothing else under your data directory. What it does
carry is the version history: every write and every delete lands a row holding
that skill's whole text in `library.db`, so the history survives even though
the files do not. A skill whose file is gone still appears in Settings →
Skills under "Skills that survive only in history," with the same history
panel and restore button every other skill uses. One consequence worth
knowing: a skill you have never edited through Linetta — one you hand-wrote
into the folder and never saved from Settings — has no row anywhere. If the
skills folder matters to you, copy it yourself, or keep it in git.

Character visual sheets hold appearance notes and up to eight reference images
per character. Add images in the sheet's own reference area: pick several at
once, drop them in, or take one from the work's gallery, such as the face an
episode 1 illustration got right. Set the illustration style in Story World;
sheets, images and style guides are included in library backups and restored
works.
`linetta_get_character_visuals` reads these designs and optional reference images;
`linetta_build_illustration_prompt` builds a single illustration or four-panel
prompt from them and your scene description.

A work's **style rules** are the part of your style a machine can check:
phrases to keep out and a sentence-length limit, kept under Story World →
Style rules and check. The check lists each place a scene breaks a rule — the
scene, the paragraph, the phrase — and changes nothing; what to do about it is
yours to decide. `linetta_analyze_style` runs the same check for an agent, so
one that drafted a scene can hold it against your rules before handing it
back. It does not judge tone: that stays with the style notes in the brief and
whoever reads them. Rules are in library backups and restored works.

You do not have to describe your style from memory. Ask an agent to work it
out from scenes you have already written: `linetta_analyze_style` also
measures them — sentence lengths, how much is dialogue, how your sentences
tend to end — and `linetta_propose_style_profile` saves what the agent makes
of that as a **draft**. A draft is the agent's reading, not your instruction,
so it stays out of your style notes and out of every brief until you approve
it under Story World → Style, where you can edit it first, add it after your
notes, replace them with it, or discard it. No tool approves a draft; only
you do.

Visual write tools (full mode): `linetta_set_character_visuals` (`project_id`, `name` or
`entity_id`, optional `age_range`, `build`, `hair`, `outfit`, `signature`, `palette`, `notes`),
`linetta_set_art_style` (`project_id`, optional `style`, `negative_prompt`),
`linetta_add_reference_image` (`project_id`, `name` or `entity_id`, then one of `data_base64` with `mime`, `attachment` for an image attached to the message being answered (built-in agent only), or `illustration_id` for a gallery picture; optional `caption`),
and `linetta_delete_reference_image` (`image_id`). Omitted patch fields are unchanged;
empty strings clear them. Images use PNG/JPEG/WebP/GIF, at most 5 MiB and 8 per character.
Each returns `undo_batch_id` for the agent undo button or `linetta_undo_last_change`
(`batch_id`); deleting an image retains its bytes and metadata in the bounded,
in-memory undo window (8 changes; cleared on restart). Set
`visual_write_tools_disabled` in settings to omit this optional group.

### Continuity checks

Ask the agent to check a scene's continuity — the "연속성 점검" starter chip
does this in one click — and it reads the scene with paragraph numbers, holds
it against Story World, your fact cards, earlier scenes, and your own
direction, and records what it finds with `linetta_report_continuity`. The
engine checks every quote and every reference before anything is stored: a
quote has to be an exact substring of the numbered paragraphs it cites, and
a reference has to name an entity, fact card, beat, or scene this work
actually has. A call that fails either check is refused with a sentence the
agent can act on, not silently accepted. An empty result — nothing wrong —
is recorded too, so "I checked and it's fine" is on the record the same way
a finding is.

**Nothing in the manuscript changes.** What you get is a list, under Story
World → Continuity checks: each finding's severity, category, summary, the
exact quote and paragraph it is about, a reference chip you can click to
open the entity, fact, or earlier scene it points at, and an optional
suggestion. You open the scene, mark it dismissed or resolved, delete it, or
leave it. If the list has grown too long to go through one by one, tell the
agent to clean it up: it reads the list with `linetta_list_continuity`, holds
what it needs against the scene, and settles the observations in bulk with
`linetta_update_continuity`. It does this only when you tell it to. Dismissing
and resolving can be reopened; a deleted observation is gone. If the scene's text has changed since the check, the group is flagged "본문이
바뀜 · 다시 점검" so you know the finding may be stale. A badge in the
right-hand scene panel shows how many open observations the current scene
carries, with a click to Story World's continuity section.

Continuity checks are a judgement call the agent makes when asked, not a
background pass. They are a different thing from the automatic consistency
re-check after a name or setting change (contextual edit): that one is
deterministic (it catches a name it just renamed and missed) and shows a
report immediately; continuity checks are the agent's own reading, held
until you act on it, and they do not replace each other.

Observations live in `library.db`, so they are in daily backups and travel
with a work restored from one — including the writer's own dismissed and
resolved judgements, which are work too valuable to lose on a restore.
References inside an observation are remapped to the restored work's own
ids; one that no longer resolves is dropped rather than left dangling. Turn
the group off under **Settings → Tool budget → Check & propose**
(`review_tools_enabled` in `settings.json`, on by default) to remove
the continuity tools and the starter chip for both the built-in agent
and any MCP client; observations already recorded stay exactly where they
are.

### Changing Story World through the agent

Story World is the agent's to change when you ask, through
`linetta_apply_story_ops`: it can edit and delete characters, places, items,
concepts, relationships, fact cards, storylines and beats, take an alias or an
attribute off a record, and close or reopen a storyline. Tell it to remove the
duplicates and it finds the ids with `linetta_list_characters`
(`include_relationships`, `include_threads`) and does it, rather than asking you
to.

Deleting an element takes its relationships, designs and reference images with
it. The prose keeps the name; only the link to the record goes. The agent says
what was removed, and the tool line in its panel has an undo that brings all of
it back, for as long as the change is among the last few. You can also delete an
element yourself from its sheet, which asks first and has no undo.

### Suggestions inbox

Ask the agent to create one character, place, or relationship directly and
it applies right away with `linetta_apply_story_ops` — the small-change
path, same as before. Anything pulled from the manuscript, or more than a
few world elements at once, goes through a different tool,
`linetta_propose_story_ops`: nothing is applied. Each candidate — a
character, place, item, concept, relationship, fact card, storyline, or
beat — lands in the **suggestions inbox** instead, grouped by kind, for you
to accept, edit, or reject one at a time. A badge on Story World's header
("제안함 N") shows how many are waiting; opening it, or running "제안함 열기"
from the command palette, shows the inbox as a sheet beside the editor.

Each candidate shows why the agent proposed it: the quote it cited from the
manuscript, with a link to open that scene, and any citations it offered
when asked for more than one. A new character or place whose name or alias
normalizes to the same text as one you already have — ignoring spacing,
case, and the middle dot — is flagged "같은 대상일 수 있음"; you can turn it
into an alias of the existing record with one click instead of creating a
duplicate. The one match the engine settles itself is the exact one: a work
cannot hold the same name twice, so accepting a candidate whose name and kind
are already there links it to that record, adding only what the record lacks
and never changing what it says, and anything depending on the candidate
follows. The same name on a different kind of element is refused until you
rename it. Anything looser stays a flag — the engine never merges those on
its own. A relationship that depends on a character candidate you have not
accepted yet is marked "생성이 먼저 필요"; selecting it selects that character
too, so accepting never leaves a dangling reference. An item that failed stays
in the inbox with the reason, ready to retry or reject. Rejecting an item
never cascades to whatever depended on it — that item just stays in the
inbox, marked as blocked, until you decide.

You can also hand the decision to the agent: say "accept these" or "reject
the rest" and it lists what is waiting with `linetta_list_proposals`, then
acts with `linetta_accept_proposals` or `linetta_reject_proposals`, and
reports what was applied, linked and failed. It does this only when you tell
it to.

Right after importing a Markdown manuscript, if a provider is connected,
Linetta offers to fill Story World from it: a dialog shows the episode
count, character count, and a rough input-token estimate (an estimate, not
a billed cost) before anything runs. Starting it opens the agent panel with
the extraction request already typed — you still press send, the same as a
starter chip. The agent works through the manuscript episode by episode,
checks what already exists first, and proposes with citations as it goes;
if it runs into its tool-call budget partway through, tell it to continue
and it picks up from the first episode it has not covered yet, using the
engine's own progress count rather than guessing.

Suggestions live in `library.db` alongside everything else, but they are
deliberately **not** carried over when you restore a single work from a
backup: a pending item's reference still names ids from the library it came
from, and a suggestion costs nothing to make again by asking the agent.
Turn the group off under **Settings → Tool budget → Check & propose**
(`review_tools_enabled` in `settings.json`, on by default) to remove
`linetta_propose_story_ops`, the inbox tools and the "Story World 채우기"
starter chip for both the built-in agent and any MCP client — items already in the inbox
stay exactly where they are.

### Where the story could go

Ask the agent "다음 회차 전개 갈래 3개를 만들어 줘" — or press the starter chip — and
it drafts 2–5 possible continuations from your last written episode and
saves them with `linetta_save_forecast`. **Nothing is canon.** The outline,
the manuscript, and Story World do not change; what you get is a sheet,
opened from the command palette ("갈래 예측 열기") or a button on the outline
panel's header, where every branch sits side by side: a title and premise,
an episode-by-episode beat timeline, risk badges you can click through to
the character, fact, or scene they are about, and — folded by default —
the character decisions, projected changes, open questions, and how the
branch fits your own direction.

**Picking a branch is a direct click, not a chat turn.** [이 갈래 고르기] and
[선택 해제] call the engine straight away; no agent runs, and your activity
log gains no new entry. The branch you pick appears in the next brief's
Writer's Direction section as a **plan** — see "Your direction" above — so
every later turn, built-in agent or MCP client, sees it without being told
again. If you want it in the outline, press [개요 반영 제안 요청]: it fills the
agent panel with a ready-to-send request, the agent proposes scenes and
beats to the suggestions inbox (never writes directly), and you apply them
there like any other suggestion. Accepting a forecast's structural
proposal gets its own one-click undo in the inbox, reverting the outline
change and anything hung on it together.

A forecast goes stale the moment canon it was drawn from changes —
anything in the outline, the manuscript (any episode's text, not only the
one you diverged from), Story World, or your own direction — and the sheet
says what moved, e.g. "정사가 바뀜: 인물 2개, 3화 본문". The check runs fresh
every time you open the sheet, so undoing the change that made a forecast
stale makes it fresh again; nothing here is a one-way flag. A stale
forecast can still be picked — the warning follows the choice into the
toast, not a refusal.

Forecasts live in `library.db` but are **not** carried over when you
restore a single work from a backup, for the same reason the suggestions
inbox is not: a branch's own references, and the episode it diverged from,
still name ids from the library it came from, and a forecast costs nothing
to draft again by asking the agent. Turn the group off under
**Settings → Tool budget → Check & propose** (`review_tools_enabled` in
`settings.json`, shared with continuity checks and the suggestions inbox,
on by default) to remove `linetta_save_forecast` and its starter chip for
both the built-in agent and any MCP client — forecasts already saved stay
exactly where they are, and you can still open the sheet and pick among
them.

## Illustrations

Linetta can draw illustrations for a scene with an image provider you choose.
It is off until you pick one in **Settings → Image generation**: ChatGPT or
Gemini, using the sign-in or key from the AI provider settings, with its own
consent box separate from the agent's. ChatGPT counts against your plan's
usage; Gemini is billed per image by Google AI.

Select a passage and choose **Draw this scene**, or use the command palette
or the gallery. Describe the scene, pick the characters (those mentioned in
the scene are preselected), choose a single picture or four panels, the
aspect, and whether to send up to four reference images from the character
visual sheets. Linetta builds the prompt from the scene, the characters'
looks and the work's art style; you can edit it, and it is sent exactly as
written. Nothing is drawn until you press **Generate**.

**Earlier pictures keep the faces the same.** A new illustration also goes out
with up to two earlier pictures from the work's gallery, after the characters'
reference images. Pictures of the same characters come first, then ones you
placed in a scene or chose to represent one, then the newest. The dialog and
the agent's request card show them as thumbnails; click one to leave it out.
When any go along, one paragraph asking the image model to match faces and
style without copying the composition is added to the end of the prompt.

Pictures are kept in the work's gallery (toolbar **Illustrations**), up to
200 per work and 5 MiB each, with the prompt, provider and model they came
from. **Insert into scene** places one in the text at the cursor; a picture
used in a scene shows **In use** and cannot be deleted until you take it out.
Illustrations are included in library backups, carried by Markdown export and
import (embedded in the single `.md` file) and by folder and Git sync (as
files in a `<work>.images/` folder beside the manuscript), and go up with an
episode you publish to Linetta Works.

A picture can also stand for a scene, an episode or a part. In the gallery,
open a picture and make it the representative of its scene or episode; it then
shows as a small image on that row of the outline and at the top of the scene,
and the gallery marks it **Representative**. A representative picture, like one
used as the Works cover, cannot be deleted until you unset it.

Once this device is connected to Linetta Works and the work is on the site, an
open picture also offers **Upload as Linetta Works cover**, which makes it the
work's cover right away. The gallery marks the chosen picture **Cover**.

**Agents ask; you approve.** An agent — the built-in one or one connected over
MCP — cannot draw. It can leave a request with `linetta_request_illustration`:
the scene, what the picture is for (insert at the end of a scene, represent a
scene, episode or part, or just keep in the gallery), and a prompt, or a scene
description Linetta turns into one from the visual sheets and art style. Each
request appears as a card under the agent's tool line and in the gallery's
**Agent requests** section, and the toolbar **Illustrations** button counts
the ones waiting. On the card you can edit the prompt, aspect, caption and
reference images, then **Generate** or reject it; nothing is drawn or sent to
the image provider before you press it. Requests from an MCP client are marked
as external, and a work holds at most 20 waiting requests.
`linetta_list_illustrations` lets the agent see the gallery and how its recent
requests were decided, and `linetta_set_representative_illustration` sets or
clears a representative from pictures already in the gallery, with an undo.
A request can name which earlier pictures go along
(`prior_illustration_ids`: up to two, or an empty list for none) or leave the
choice to the app.
The two write tools are hidden by the same switch as the other visual write
tools, and `read_only` mode keeps only the list.

## Writing with your own agent (MCP)

Connecting an outside agent over MCP is a different thing from connecting a
provider in Settings: a writer who only wants MCP never has to give Linetta a
credential at all.

When you want to write alongside AI, Linetta opens a local MCP endpoint and an
agent you already run — Claude Code, Claude Desktop, Codex CLI, or Gemini CLI —
connects to it. The subscription you already pay for does the work; Linetta
stays a writing tool.

Turn it on in **Settings → Connect an external agent (MCP)**. It is off until
you tick an explicit consent box. When those clients are detected, Settings
offers one-click connect; the pane still gives you a copyable command to paste
into your client.

Once connected, an agent can:

- read the outline, a scene, characters, fact cards, and a story brief;
- draft and revise scenes, write summaries, and restructure the outline;
- ask for an illustration, which waits for your approval before anything is drawn (`full` mode only);
- publish an episode to Linetta Works, once you have connected an account and published the work yourself (`full` mode only);
- record what it changed, so you can see it in the activity log.

The story brief also carries the writer profile and the work's notes described
above, in `read_only` and `full` alike. In `full` mode a client can write them with
`linetta_edit_memory`; in `read_only` that tool is not registered at all. A
client you have pinned to a single work can write only that work's notes — the
writer profile applies to every work, so it is outside the pin, and the tool
refuses it.

Skills work the same way. The brief lists every enabled skill's name and
description, in both modes, and `linetta_read_skill` opens one — that tool is a
read tool, registered in `read_only` as well. Writing is `linetta_edit_skill`,
which like every other write tool exists only in `full` mode. A pinned client
can write a skill for its own work and is refused on a global one, for the same
reason it is refused on the writer profile.

The writer keeps the last word. Every scene write is snapshotted before it
lands, and a recent outline restructuring or scene-prose rewrite can be
reversed in one click from the agent tool line — the last eight, until the
app is restarted; entity, storyline, beat, fact-card and memory edits are
logged but have to be reversed with another edit. `read_only` mode omits the writing tools
entirely, and a scene you are part way through editing is never replaced behind
your back — Linetta tells you the agent touched it and leaves your text alone
until you choose.

The endpoint binds `127.0.0.1` only, requires a token Linetta generates
locally, and stops the moment you turn it off.

## Publishing to Linetta Works

[Linetta Works](https://works.marvin-42.com) is a reading site for fiction
written in Linetta. Publishing to it is optional: nothing is sent to the site
until you connect an account, and after that only the episodes you choose to
publish.

**Connect.** Open **Settings → Publishing**, read what publishing sends, tick
the box, and press **Connect in browser**. The site opens in your browser with
a confirmation code; check it matches the one Linetta shows and press
**Allow**. Linetta then shows your pen name. This build of the feature is for
desktop; it is hidden on iOS.

**Publish.** Open an episode's menu in the outline — or a scene's, which
publishes the episode it belongs to — and choose **Publish this episode…**, or
use the command palette. The first time, Linetta asks for the blurb readers see
and whether the work is ongoing or complete; the title and genres come from
the work itself. The dialog shows the episode's address, with a copy and an
open button. Edit the episode later and the outline marks it **Edited**;
publish again and the same address shows the new text. Episode numbers follow
your outline, and an episode you move or insert renumbers the ones after it —
their old addresses redirect. You can make an episode private from the same
dialog. An agent you connected (built-in or over MCP) can publish too, once
you have connected your account. Ask it to publish an episode of a work that
is not on the site yet and it registers the work first with
`linetta_register_work`, using a blurb you wrote or approved, then publishes
with `linetta_publish_episode`. It can also change the blurb, mark the work
complete, or put a gallery picture up as the cover with
`linetta_publish_cover`. Only you can connect an account.

**Sheets and cover.** The quickest way to set the cover is from the gallery:
open a picture and press **Upload as Linetta Works cover**. It shows only when
this device is connected and the work is on the site, it replaces the cover on
the site, and it sends no sheets. You can follow its review on the same
picture.

The publish dialog's **World, characters and cover** section, at the bottom of
the dialog you open with **Publish this episode…** in an episode's outline
menu, can put character and world sheets and a cover up together. A sheet goes
only if you turn on **Show on Linetta Works** in its panel — off by default,
and shown only when this device is connected — so
check it for spoilers first. The sheet's name, aliases, role, summary,
attributes and the names of its relationships go, with its first reference
image as the sheet's picture; relationship notes, visual notes and sheets you
have not made public are not sent. Pick a cover from the work's gallery, then
press **Upload sheets and cover**. They go up separately from episodes, so a
rejected picture or a limit on sheets never holds back an episode.

**Share.** Once an episode is public, the publish dialog shows **Share**
buttons under its address. X, Threads and Naver Blog open their share page in
your browser with the episode's title and link filled in. KakaoTalk and
Instagram have no share page a desktop app can open, so those buttons copy the
title and link for you to paste. Private, rejected and in-review episodes show
no Share buttons.

**What is sent, and where.** The published episode's title and text, the
illustrations placed in it, its place in the order, and the work's title,
genres, blurb, serial status and address go to the Linetta Works server,
which is run by Linetta's developer, and are public for anyone to read; so do
the sheets and cover you chose above. The site sends published text and
illustrations to the Anthropic API for content review. Unpublished episodes,
notes, fact cards, agent memories and sheets you have not made public are not
sent when you publish. See the
[privacy policy](docs/privacy-policy.md), section 3.5.

**Where the connection is kept.** The publish token is kept in the macOS
Keychain or the Windows Credential Manager. Linux has no secure store Linetta
can use, so there it is kept in `publish-token` in Linetta's data folder,
readable only by your user. Your pen name and the site address are kept in
`settings.json`.

**Disconnect.** **Settings → Publishing → Disconnect** forgets the connection
on this device. To remove the device from your device list on the site,
disconnect it from your profile there. Episodes you published stay up until
you make them private or delete them.

## Private vault

The private vault carries a work between your devices through Linetta Works
without the site being able to read it. It is a free beta for accounts the
operator has approved, and the desktop app is the only place to use it.

**Turn it on.** Connect your account under **Settings → Publishing**, then open
**Settings → Private Vault**. It takes you to the site to read the agreement
and send a request. Once approved, come back, agree to what is sent, and make a
recovery key, or enter the one you saved on your first device.

**Save the recovery key.** It is shown once, and it is the only way to open what
you upload. The operator cannot recover it. Linetta keeps a new key only after
you confirm you saved it somewhere off this device.

**Upload and download.** Open a work and use the vault button in the top bar or
**Open Private Vault** in the command palette. Nothing moves by itself: Linetta
shows that a newer version is waiting, and you decide. A work that is not on
this device yet is downloaded from **Settings → Private Vault**.

**When two devices disagree.** If a work changed in both places, choose which to
carry on from. Either way both manuscripts survive: uploading this device's
version leaves the other as a restore point, and downloading the vault's
version first copies this device's version into a new work. Downloading backs up
your library first.

**What is sent, and where.** A work's title, manuscript, outline, story data,
version snapshots, pictures, and its own skills and memory are sealed on this
device and stored through the Linetta Works server in Cloudflare R2. The server
keeps ciphertext, its size and times, and random identifiers, and nobody there
can read the content or the title. Activity logs, your global writer profile and
skills, and provider settings and API keys stay here. The full list is in the
[privacy policy](docs/privacy-policy.md), section 3.7.

**Delete.** **Settings → Private Vault → Delete from server** removes one work
and its restore points for good, and leaves the manuscript on this device.
Closing the vault on the site deletes everything there. Disconnecting from the
site deletes nothing on the server and keeps your recovery key.

Data in the vault can be lost, and the site makes no promise to keep or restore
it. Keep your own backups.

## Data and safety

Linetta stores writing data on your device:

```text
macOS    ~/Library/Application Support/com.devlikebear.linetta
Linux    ${XDG_DATA_HOME:-~/.local/share}/com.devlikebear.linetta
Windows  %APPDATA%\com.devlikebear.linetta
```

Important data includes:

- `library.db`: projects, scenes, story data, version snapshots, the writer profile and work notes an agent reads, the version history of every skill, and the retired built-in companion's transcripts;
- `backups/YYYY-MM-DD/`: daily backups of `library.db`, kept for 14 days — the database only, nothing else on this list — plus the copy of the library made before each private vault download (`library-pre-vault-*`);
- `skills/`: the `SKILL.md` documents an agent writes and reads, one folder per skill;
- `<project id>/memory/experiences.jsonl`: facts an agent has been told to remember;
- `companion/`: where facts remembered before 1.0 were left. Linetta moves them
  up beside the others at startup, then removes this folder if nothing is left
  in it — so anything at all it did not move, down to a hidden `.DS_Store`,
  keeps the folder around after every project has already moved out of it. A
  folder still here is not a sign that memories were left behind; see
  [Moving to Linetta 1.0](docs/migrating-to-1.0.md) for what it declines to
  move and what to do about each case;
- `settings.json`: app preferences, and the Linetta Works account this device is connected to (pen name and site address — not the token);
- `publish-token`: on Linux only, the Linetta Works publish token, readable only by your user (elsewhere it is in the OS secure store);
- `vault-key`: on Linux only, the private vault recovery key, readable only by your user (elsewhere it is in the OS secure store);
- `vault-incoming/`: sealed pieces of a private vault download in progress, removed when it finishes.

Manual and agent-write snapshots are retained indefinitely. Autosave snapshots are thinned over time, from every save during the first day to daily snapshots after 30 days.

## Frequently asked questions

### Does Linetta require an account or subscription?

No. Writing, organization, import/export, snapshots, and backups work without a Linetta account or subscription.

### Do I have to use AI?

No. Linetta is a complete writing app on its own. It does not contact a model
until you set one up: connect a provider and give it your consent, for the
built-in agent, or connect your own agent over MCP, which needs no provider
credential at all.

### Can I bring an existing manuscript?

Yes. Linetta supports Markdown import and export so your writing is not locked into the app.

### Is my work uploaded to a Linetta cloud?

Not unless you choose to. Linetta needs no cloud, and nothing is uploaded by default. The MCP endpoint is local to your machine, and Git sync talks only to the remote you configure. There are two exceptions, both your choice: when you publish an episode to Linetta Works, that episode goes to the site so readers can read it — as do any Story World sheets and cover you choose to make public — and when you upload a work to the [private vault](#private-vault), it goes to the same operator's server sealed, so that nobody there can read it.

## For contributors

Linetta uses a Tauri 2 Rust shell, React 18, Vite, TypeScript, an embedded Go engine, and SQLite. The shell and engine communicate through JSON-RPC envelopes across a C ABI.

See the [development guide](docs/DEVELOPMENT.md) for architecture notes, versioning, mobile build targets, CI workflows, and troubleshooting.

### Build from source

Install the desktop dependencies once:

```sh
cd apps/desktop
pnpm install
```

Start the desktop app:

```sh
make dev
```

Build a desktop release for the current operating system:

```sh
make build-desktop
```

Build the standalone JSON-RPC engine for debugging:

```sh
make build-engine
```

### Verification

Run the full local gate:

```sh
make test
```

Useful focused checks:

```sh
make test-go
make test-desktop
make test-tauri
make test-mobile-engine
```

`make test` runs the Go test suite, frontend Vitest tests, the Vite production build, and the Tauri shell's `cargo check` and `cargo test`.

### Mobile engine development

The repository also builds the embedded engine for iOS and Android. These targets are intended for contributors working on the mobile runtime:

```sh
make build-mobile-engine-ios
make build-mobile-engine-android
```

The iOS target requires Xcode's iOS SDK. The Android target requires `ANDROID_NDK_HOME`.

## Contributing and feedback

Linetta is in active development. Bug reports, writing-workflow feedback, localization fixes, and focused pull requests are welcome through [GitHub Issues](https://github.com/devlikebear/linetta/issues).

If you are a novelist trying Linetta on a real project, feedback about where the app helps or interrupts your flow is especially valuable.

## License

Linetta is licensed under the GNU Affero General Public License version 3 only (`AGPL-3.0-only`). See [LICENSE](LICENSE) and [LICENSE-NOTICE.md](LICENSE-NOTICE.md) for details, including commercial licensing options.
