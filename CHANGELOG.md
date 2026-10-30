# Changelog

## v1.14.0 - 2026-10-30

- **Share a published episode.** Once an episode is public on Linetta Works,
  the publish dialog shows Share buttons under its address. X, Threads and
  Naver Blog open their share page in your browser with the episode's title
  and link filled in. KakaoTalk and Instagram have no share page a desktop
  app can open, so those buttons copy the title and link for you to paste.
  Private, rejected and in-review episodes show no Share buttons.

## v1.13.1 - 2026-10-30

- Fixed: in the installed app, illustrations showed only their caption, both
  in the gallery and in the manuscript. They now show the picture again. The
  pictures themselves were never lost.

## v1.13.0 - 2026-10-26

- **Publish character sheets and a cover to Linetta Works.** Each sheet in
  Story World has a **Publish on Linetta Works** checkbox, shown once this
  device is connected to the site. It is off by default, and turning it on
  first reminds you to check for spoilers. A published sheet sends its name,
  aliases, role, summary, attributes, relationship names and its first
  reference image. Notes are never sent. Pick a cover from your gallery in
  the publish dialog's **World, characters and cover** section.
- Fixed: the text boxes in Story World and the character and storyline
  sheets could not be resized and used the manuscript's serif font. They now
  grow with their content, can be dragged taller, and use the UI font.
- Fixed: if undoing accepted outline suggestions failed twice in a row, the
  suggestions could be accepted again and create a duplicate scene. They now
  wait as "reverting · needs a check" until the app sorts them out from the
  outline itself, which it also does when it starts.

## v1.12.1 - 2026-10-25

- Fixed: the open-observations badge in the scene panel opened Story World
  with the continuity section collapsed, or not at all while the Fact Book
  was open. It now opens the continuity section on this scene's open
  observations.
- Fixed: the button that reopens a dismissed or resolved observation read
  like a status label. It now says **Reopen**.
- Fixed: undoing accepted outline suggestions could leave some of them stuck
  if a write failed partway. The undo now keeps them retryable.

## v1.12.0 - 2026-10-25

- **Where the story could go.** Ask the agent for possible branches of the
  next episode (the **Next development branches** chip does it in one
  click). It saves 2–5 continuations side by side: a premise, an
  episode-by-episode beat timeline and risk badges. Nothing is canon: the
  outline, the manuscript and Story World do not change. Open them from the
  command palette (**Open forecasts**) or the outline panel's header. Pick
  one and it goes into the agent's brief as a plan. To put it in the
  outline, ask the agent to propose it, then accept the scenes and beats in
  the suggestions inbox, where one click undoes them together. When the
  outline, the manuscript, Story World or your direction changes, a branch
  is marked stale and the sheet says what changed.
- **The agent can register a work on Linetta Works.** Ask it to publish an
  episode of a work that is not on the site yet. It writes or takes a blurb
  you approve, registers the work, then publishes the episode. It can also
  change the blurb or mark the work complete. Connecting your account is
  still yours to do in **Settings → Publishing**.
- Fixed: after the agent published an episode, the outline's publishing
  badges did not update until you reopened the work.

## v1.11.0 - 2026-10-25

- **Ask the agent about a passage.** Select text in a scene, right-click,
  and choose **Ask the agent**. The agent panel opens with the passage
  quoted above the message box. Ask about it, or say what to change. A
  change goes into that scene only. **Polish**, **Get feedback** and
  **Expand** fill in a request for you, and you still press send. The passage
  stays in the conversation as a quote, so a follow-up like "now make it
  shorter" knows what "it" is.
- **Writer's Direction.** The scene panel now starts with a **Current
  focus** for the work (what to concentrate on for the next few episodes)
  and **This scene's intent** (a one-line goal, what to keep, what to
  avoid). Only you write them. The built-in agent and connected MCP clients
  read them first and treat them as binding. Three written episodes after
  you set the focus, the app asks whether it still holds.
- **Continuity checks.** Ask the agent to check a scene's continuity (the
  **Check continuity** chip does it in one click). It holds the scene
  against Story World, your fact cards, earlier scenes and your direction,
  and records what it finds. Every finding quotes the scene exactly and
  points at something the work really has. The manuscript is not changed.
  Findings wait under **Story World → Continuity checks**, where you can
  dismiss or resolve them, and the scene panel shows how many are open.
- **Suggestions inbox.** Characters, places, relationships, fact cards and
  other world elements the agent draws from the manuscript now go to a
  suggestions inbox rather than straight into Story World. You accept,
  edit or reject them one at a time. A candidate that looks like someone
  you already have is flagged, and you can make it an alias instead of a
  duplicate. Small changes you ask for directly still apply right away.
- **Fill Story World after an import.** After you import a Markdown
  manuscript, Linetta offers to fill Story World from it. It shows the
  episode count and a rough token estimate before anything runs. The agent
  works episode by episode and sends what it finds to the suggestions inbox.
- **Settings → Tool budget** has a new **Check & propose** group. Turning it
  off removes the continuity and suggestion tools and their chips. Nothing
  already recorded is deleted.
- Publishing an episode to Linetta Works now uploads its illustrations as
  well. Readers see them in the same place as in the manuscript, and they
  are reviewed along with the text. An episode can carry up to 30 pictures.
  Pictures already on the site are not sent again.
- Fixed: opening a scene with @mentions or notes in Zen mode could save it
  as an empty scene. The editor now refuses to edit or save a scene it
  cannot fully read, and says so, instead of treating it as empty.
- Fixed: with **Codex** as the AI provider and no model chosen, requests
  failed on a ChatGPT account because the old default model is no longer
  offered. Linetta now uses the first model your account offers.
- Security: the engine is built with Go 1.27.3, which includes the October
  2026 standard-library security fixes.

## v1.10.0 - 2026-10-24

- **Illustrations.** Choose ChatGPT or Gemini under **Settings → Image
  generation** to draw pictures for your scenes. It is off until you choose
  one, and it uses the sign-in or key you already set up for that AI
  provider, with its own consent. A picture is drawn only when you press
  generate.
- **Draw this scene** in the editor's selection menu, **Draw an
  illustration** in the command palette, or the gallery opens the drawing
  dialog. Describe the scene and pick the characters (the ones mentioned in
  the scene are already picked). Then choose a single picture or four panels,
  the aspect, and whether to send reference images. **Build prompt** shows
  exactly what will be sent; you can edit it, and what you edited is what
  goes.
- Each work keeps its pictures in a gallery (the **Illustrations** button,
  or **Open illustration gallery**), up to 200 per work. The gallery shows
  the prompt each picture was drawn from and which model drew it. You can
  edit captions and delete pictures there.
- **Insert into scene** puts a picture into the manuscript at the cursor,
  where you can edit its caption. The gallery marks pictures **In use** and
  names the scene, and will not delete a picture a scene still uses.
  Character counts, search and replace ignore pictures.
- Markdown export carries the pictures inside the one `.md` file, and
  importing that file brings them back into the new work's gallery. Folder
  and Git sync write them as image files in a `<work>.images` folder beside
  the manuscript. Plain-text export and publishing to Linetta Works leave
  pictures out for now; the publish dialog says how many.
- An agent reading a scene sees each picture as an `[[illustration:…]]`
  line. Writing the line back keeps the picture where it was; dropping the
  line takes it out of the scene but leaves it in the gallery.
- A scene summary the built-in agent wrote is no longer replaced by the
  scene's first few sentences after the scene is edited. The next episode's
  context still gets that summary, marked as describing the earlier draft,
  until the agent writes a new one.
- The built-in agent now reads the work's context — your style notes
  included — before it writes or revises a scene in a turn, so a one-line
  fix follows your style notes too.
- With **Claude Code** as the AI provider, the model list now offers full
  model ids such as `claude-opus-5-5` after the `opus`, `sonnet` and `haiku`
  aliases, so you can pin an exact model. The list comes from the models your
  Claude Code has cached, a built-in list, and — if you also set up an
  Anthropic API key — Anthropic's own current list. The aliases still come
  first and still follow the newest model of each tier.
- Security: built with Go 1.26.10 and updated `rustls-webpki` and
  `webpki-root-certs`, for the advisories about a compromised intermediate
  certificate.

## v1.9.1 - 2026-10-19

- **Settings → Publishing** has a **Browse Linetta Works** button that opens
  the site in your browser, connected or not. It only opens the page; the app
  sends nothing to the site.

## v1.9.0 - 2026-10-18

- Publish episodes to [Linetta Works](https://works.marvin-42.com), a reading
  site for fiction written in Linetta, straight from the app. It is optional
  and off until you connect: under **Settings → Publishing**, read what
  publishing sends, agree, and connect your account in the browser by
  matching a confirmation code. Nothing is sent to the site before that, and
  afterwards only the episodes you publish.
- **Publish this episode…** in an episode's (or a scene's) outline menu, and
  in the command palette, opens the publish dialog. The first time a work goes
  up it asks for the blurb readers see and whether the work is ongoing or
  complete; title and genres come from the work. The dialog gives the
  episode's address to copy or open, and can make it private again.
- Once a work is on the site, the outline marks each episode **Public**,
  **Edited** (changed since you published it), **In review**, **Rejected** or
  **Private**. Publishing an edited episode again updates the same address.
  Episode numbers follow your outline; moving or inserting an episode
  renumbers the ones after it, and their old addresses redirect.
- If publishing fails — no connection, an episode too long for the site, the
  device disconnected on the web — your manuscript is left exactly as it was,
  the dialog says why, and you can try again.
- An agent you connected can see what is published
  (`linetta_publish_status`) and publish an episode when you ask
  (`linetta_publish_episode`), after you have connected and published the work
  once yourself. Every publish is in the activity log.
- The publish token is kept in the macOS Keychain or the Windows Credential
  Manager, or on Linux in a `publish-token` file only your user can read. The
  [privacy policy](docs/privacy-policy.md) now describes what publishing sends
  (section 3.5).

## v1.8.2 - 2026-10-17

- With Gemini, the built-in agent no longer stops when the model calls more
  than one tool at once — reading two scenes together, say. Gemini rejected
  the next step because each tool's result went back as a separate message;
  the results of one step now go back together
  ([#200](https://github.com/devlikebear/linetta/pull/200)).
- With Gemini and no model chosen, the agent now uses `gemini-3.8-flash`.

## v1.8.1 - 2026-10-17

v1.8.0 was never published as a release, so this is the first release with
the v1.8.0 changes below.

- Long conversations with the built-in agent cost less. Once a conversation
  outgrew the history the agent replays, the oldest message dropped out on
  every turn, so the AI service could not reuse anything it had cached from
  the turn before. The replayed history now keeps its starting point for as
  long as it fits
  ([#197](https://github.com/devlikebear/linetta/pull/197)).
- With Anthropic, a turn that calls tools many times — reading scenes,
  searching the manuscript — no longer resends everything read so far at full
  price on every step. Each step now marks the newest tool result for caching,
  so the next step reuses it
  ([#198](https://github.com/devlikebear/linetta/pull/198)).
- Updated rustls to 0.23.46 to pick up its security fixes
  ([#196](https://github.com/devlikebear/linetta/pull/196)).

## v1.8.0 - 2026-10-14

- Claude Code is now an AI provider. If Claude Code is installed and you are
  signed in to it, the built-in agent can run on your own Claude plan, with no
  API key: pick **Claude Code** under **Settings → AI provider**, where
  Linetta shows where it found the `claude` command (or that it did not), and
  use the connection test to check you are signed in. Linetta runs it with
  its own file, shell and web tools switched off, and with your own hooks,
  plugins and other MCP servers left out, so the agent works on your
  manuscript only through Linetta's tools — and what it sends goes to
  Anthropic. The model list offers Claude Code's own `haiku`, `opus` and
  `sonnet`. Not available in the Mac App Store or mobile builds, which cannot
  start another program
  ([#194](https://github.com/devlikebear/linetta/pull/194)).
- The agent panel shows which provider and model your next message goes to,
  under the message box, and you can change either one there without opening
  Settings. Only providers that are set up and agreed to can be picked; the
  others say they need setup. The choice is locked while a reply is running,
  since a change only applies from the next message
  ([#190](https://github.com/devlikebear/linetta/pull/190)).
- Fixed Google Gemini stopping with "Could not reach this AI service" when a
  reply took more than 30 seconds to begin — which is normal when the model
  writes a whole scene in one go. Gemini replies now run as long as they need,
  a reply that is cut off partway is reported instead of silently arriving
  empty, and two tool calls in one Gemini response no longer lose the second
  one ([#194](https://github.com/devlikebear/linetta/pull/194)).
- Error messages from AI providers now say what actually went wrong. A reply
  that took too long, a request the service refused, and a problem on the
  service's side each have their own message; until now all of them said to
  check your internet connection. The cause of a failed turn is also written
  to `logs/agent.log` in Linetta's data folder, so it can be looked into
  afterwards ([#192](https://github.com/devlikebear/linetta/pull/192)).
- Saving AI provider settings is now all or nothing. When the system keychain
  refused to store an API key, the rest of that save — the model, the address,
  your consent — used to be kept anyway; now nothing in it is
  ([#191](https://github.com/devlikebear/linetta/pull/191)).

## v1.7.0 - 2026-10-13

- The agent panel now knows when a turn is still running. A turn keeps going
  in the background when you close the panel or leave the work, but coming
  back used to show only the saved conversation and a guess ("may still be in
  progress") beside a send button that would be refused. The panel now picks
  the running turn back up — stop button, the reply streamed so far, and
  everything after — and the toolbar's agent button shows a dot while a turn
  is running, panel open or not
  ([#185](https://github.com/devlikebear/linetta/pull/185)).
- A running turn says so. The last line of the conversation shows what the
  agent is doing and for how long ("Thinking… 0:42"), so the minutes it can
  spend writing a whole scene into a single tool call no longer look like it
  has stopped. The conversation also follows new lines as they arrive, unless
  you have scrolled up to read.
- A turn that ends without an answer now tells you why. When the model
  returned nothing, or its response was cut off at the provider's output
  limit, the turn used to end silently: a column of "read" lines and nothing
  after. Both now leave a notice, as does a turn cut short because the app
  was closed on it, and sending "continue" picks the work up.
- What the agent says after using a tool now appears below that tool's line,
  where it happened, instead of being appended to its earlier message above.
- Raised the output limit for Anthropic models from 16,000 to 64,000 tokens.
  Rewriting several scenes in one response ran past the old limit and was cut
  off partway through, with nothing written. This applies to the current
  Claude families (Opus 4.6 and later, Sonnet 4.6 and later, Haiku 4.5, and
  the 5 series); other models and providers keep their own defaults. Long
  responses that used to be cut off now run to the end, so such a turn can
  use more output tokens
  ([#186](https://github.com/devlikebear/linetta/pull/186)).

## v1.6.0 - 2026-10-11

- The built-in agent no longer stops mid-job without a word. When a turn runs
  into its tool-call limit — or the same tool fails three times running — the
  agent now gets one last reply, with tools forbidden, to say what it finished
  and what is left, so the notice that points you at "the reply" has a reply
  to point at and sending "continue" picks the work up from there. The limit
  itself went from 24 tool calls a turn to 48: summarising every scene costs a
  read and a write per scene, and 24 cut a twelve-scene work off partway
  ([#173](https://github.com/devlikebear/linetta/pull/173)).
- You can set that limit yourself: **Settings → Tool budget → Tool calls per
  turn** (`agent_max_tool_calls` in `settings.json`), from 8 to 200, default
  48. Raise it to hand a long work over in one go; a higher limit also lets a
  single turn cost more. It applies to the built-in agent only — an external
  MCP client runs its own loop.
- Fixed the iOS app being killed at launch when built with Xcode 27: it now
  adopts the UIScene lifecycle (Tauri 2.12). Also on iOS, the white bands in
  the status-bar and home-indicator areas are gone, focusing a small input no
  longer leaves the page zoomed in, and a work card opens on the first tap
  ([#172](https://github.com/devlikebear/linetta/pull/172)).
- Built with Go 1.26.9, which fixes nine standard-library vulnerabilities in
  `net/http`, `net/textproto` and `crypto/tls`
  ([#174](https://github.com/devlikebear/linetta/pull/174)).

## v1.5.0 - 2026-10-06

- Linetta can update itself. **Check for Updates…** is in the tray menu and,
  on macOS, in the Linetta menu: it looks up the latest GitHub release, asks
  before downloading, and asks again before it installs and restarts, so
  nothing is replaced under your hands. It also checks once shortly after
  launch and offers each new release a single time. Every installer is signed
  with an update key and the app refuses a download whose signature does not
  match. Mac App Store installs are left to the App Store, and a build that
  cannot replace itself points you to the release page instead. This starts
  with the first release after v1.4.0 — v1.4.0 and earlier have no updater, so
  that one upgrade is still by hand.

## v1.4.0 - 2026-10-06

- Fixed Markdown export silently dropping formatting the editor lets you
  apply. Bullet and numbered lists (nested ones included), code blocks, inline
  code, strikethrough, underline and links were written as bare text, so an
  exported file — and every folder-sync and Git-sync copy — lost them, and
  importing it back gave you paragraphs. They are now written as Markdown and
  read back as the same nodes; blank lines inside a code block are kept, and a
  `# comment` in code is no longer taken for a chapter heading on import.
  Prose that merely looks like syntax (`- 그는 웃었다`, `1. 첫째`, a literal
  `*` or `_`) is backslash-escaped on export so it comes back as prose. The
  plain-text copy also dropped the text of code blocks; it keeps it now. A
  test holds the exporter's node list against the editor's real schema, so an
  editor extension added later cannot be forgotten the same way
  ([#160](https://github.com/devlikebear/linetta/issues/160)).
- `linetta_read_scene` takes an optional `format`. Leave it out (or pass
  `plain`) and the read is byte-for-byte what it was. Pass `markdown` and the
  scene comes back with the writer's headings, emphasis, lists, links, quotes
  and code intact, rendered by the same converter the Markdown export uses, so
  an agent carrying a scene out of Linetta no longer has to guess the
  formatting back in. It is a read format only: the write tools still take
  plain text, and `content_version` is the same for both
  ([#161](https://github.com/devlikebear/linetta/issues/161)).
- Added style rules and a style check. Style notes have always told an agent
  how your prose should read; nothing told you whether the prose it handed
  back did. Under Story World → Style rules and check you can now list
  phrases to keep out of a work and set a sentence-length limit, then check
  the open scene or the whole work. The check names each place a rule is
  broken — scene, paragraph, the phrase or the sentence's length, with an
  excerpt you can click through to — and never edits anything. It runs in the
  engine without a model, so the same scene and rules always give the same
  report, and with no rules saved it says nothing was checked rather than
  reporting a clean result. A new read tool, `linetta_analyze_style`, gives a
  connected or built-in agent the same check; judging tone stays with the
  agent and the style notes. Rules travel with library backups and restored
  works ([#162](https://github.com/devlikebear/linetta/issues/162)).
- An agent can now work out your style from what you have already written,
  so the style notes no longer have to start from a blank box.
  `linetta_analyze_style` also returns measurements of the scenes it reads —
  sentence lengths, how many sentences a paragraph holds, the share that is
  dialogue, the commonest sentence endings — counted in the engine without a
  model. A new write tool, `linetta_propose_style_profile`, saves the agent's
  description as a draft of at most 2,200 characters. A draft is the agent's
  reading and not your wish, so it is kept apart: it is in neither your style
  notes nor any story brief until you approve it. Story World → Style shows a
  waiting draft, lets you edit it, and then adds it after your notes, replaces
  them with it, or discards it; the same section now lets you edit the style
  notes themselves, which the app had no place for before. No MCP tool
  approves a draft, in any mode
  ([#163](https://github.com/devlikebear/linetta/issues/163)).
- Story World now scrolls as a whole under its header. Its sections above the
  list could be opened taller than the panel and were cut off.

## v1.3.0 - 2026-10-05

- Added character visual sheets, up to eight reference images per character,
  and a work-wide illustration style guide. The images travel with library
  backups and restored works. Two read tools expose the designs and build
  single-illustration or four-panel prompts from the writer's own descriptions
  ([#154](https://github.com/devlikebear/linetta/issues/154)).
- Fixed missing outline controls and inspector positioning on full-width iPads,
  and aligned outline behavior with the compact layout on landscape iPhones.
- Aligned the app and embedded engine on iOS/iPadOS 15.0, and made local and CI
  iOS builds use the SwiftPM builder required by the current Swift bridge when
  running Xcode 27. Native input and signed distribution QA remain pending.

## v1.2.2 - 2026-09-10

- Fixed five ways the editor could lose or overwrite what you typed, all in
  the same save path. Toggling focus mode rebuilt the editor from the scene
  as it was on load, discarding every edit since; entering ZEN did the same.
  The last few keystrokes were dropped when you went back to the library,
  jumped to another work from search, opened Threads or Settings from the
  palette, or pressed `Cmd/Ctrl+R`, because the pending autosave was
  cancelled rather than flushed. A failed save replaced the whole screen
  with an error and unmounted the editor with your unsaved text in it; it is
  now a line beside the editor, and a version conflict says so in its own
  words. A save that finished after you had typed again marked the scene
  clean, so an external change could reload over the newer text; the
  editor now tracks an edit revision and only clears the flag for the
  revision it actually saved (ZEN never set the flag at all — it does now).
  And batch replace checked the scene's version before writing but wrote
  unconditionally, so a save landing in between was overwritten; the write
  is now version-checked too
  ([#103](https://github.com/devlikebear/linetta/issues/103),
  [#104](https://github.com/devlikebear/linetta/issues/104),
  [#105](https://github.com/devlikebear/linetta/issues/105),
  [#106](https://github.com/devlikebear/linetta/issues/106),
  [#107](https://github.com/devlikebear/linetta/issues/107)).
- Trimmed the descriptions of three MCP tools — `linetta_apply_story_ops`,
  `linetta_edit_skill` and `linetta_read_skill` — that had grown to restate
  their own JSON schema (field lists, an apology for having no list tool) on
  top of the workflow guidance an agent actually needs. Measured by
  `MeasureToolBudget`, the three together dropped from 8,282B to 7,120B, and
  the full 19-tool set a built-in or external agent is handed dropped from
  27,687B to 26,525B. The workflow sentences that only live in these
  descriptions — outline-structural batches are all-or-nothing and return
  `undo_batch_id`, entity/thread/beat/fact/memory changes are not
  batch-undoable, scene prose goes through `linetta_write_scene` instead —
  are unchanged. Part of [#90](https://github.com/devlikebear/linetta/issues/90)
  ([#132](https://github.com/devlikebear/linetta/issues/132)).
- A scene the agent wrote can now be reverted from its tool line, the same
  way an outline batch already could. The agent panel only offered an undo
  button when a tool call returned `batch_id`, but `linetta_write_scene` and
  `linetta_revise_scene` — the agent's most common action — return
  `snapshot_id` instead, so the "click undo on the line you just watched
  change" flow the design doc promises never fired for prose — tracked as
  [#112](https://github.com/devlikebear/linetta/issues/112). The panel now
  offers undo whenever a line carries either.
- Restoring a single work from a backup now brings its work notes back with
  it. 1.2.0 added per-work **work notes** (`agent_memory`, scope
  `work_notes`) but `MergeProject`, the per-work restore path, copied only the
  work's own tables and left memory out, so a restored work came back with
  its notes silently empty — tracked as
  [#116](https://github.com/devlikebear/linetta/issues/116). The startup
  recovery screen, which replaces the whole library file, was never affected.
  The global **writer profile** stays with the library and is never copied or
  duplicated by a per-work restore.
- Fixed a skill whose file is gone from `<app data>/skills/` being
  unreachable in Settings → Skills — no row, and no way to see it ever
  existed. The daily backup still does not carry that folder, only
  `library.db` (`VACUUM INTO`, nothing else under the data directory), so
  that has not changed; what changed is that skill_snapshots' own row for the
  name — landed by every write and delete, whether or not its file survives —
  is now something the pane can reach. Settings → Skills lists any such name
  under "Skills that survive only in history," and opens the same history
  panel and the same restore button every other skill uses: `skills.restore`
  could already recreate a skill from a version row, it just had no name to
  restore ([#119](https://github.com/devlikebear/linetta/issues/119)).

## v1.2.1 - 2026-09-09

- Added an **Agent** button to the workspace toolbar, beside Contextual edit,
  Story World and Fact Book. 1.2.0 opened the built-in agent's panel only from
  `Cmd/Ctrl+J` and the command palette, so a writer who installed it saw no
  sign of the feature the release was about. The button shows only in builds
  that carry the agent, like the panel itself.

## v1.2.0 - 2026-09-09

- Added a built-in AI writing agent: connect a provider — sign in with a
  ChatGPT account, or add an API key for Anthropic, Google Gemini, or an
  OpenAI-compatible endpoint — and open the panel with `Cmd/Ctrl+J`. Consent to
  send your writing to a provider is given separately for each one.
- Added ChatGPT (Codex) sign-in inside the app, so connecting it no longer
  needs the Codex CLI installed. Tokens are kept at
  `<app data>/codex/auth.json` with owner-only permissions (0600), in the same
  file format the Codex CLI uses for its own login, so the two can share one
  sign-in — except in the Mac App Store build, which is sandboxed and cannot
  read `~/.codex`, and so needs its own sign-in.
- The MCP activity log now records whether the built-in agent or an external
  client made each tool call. Activity entries and the `mcp.changed`
  notification both carry a new `source` field for it.
- The built-in agent has no tool surface of its own: it is a client of
  Linetta's own MCP server, reaching the same tools external clients use,
  rather than a second set built for it.
- The consent text you tick before connecting a provider now spells out what
  the agent's tools can actually read — any scene in any work, not only the one
  you have open, plus summaries, the outline, manuscript-wide search, the
  character, plot and fact cards, remembered facts, and the list of your works.
  The behaviour did not change; the sentence you agree to now matches it.
- An agent now keeps two short, curated memories: a **writer profile** (1,400
  characters) for how you work, and **work notes** (2,200 characters) for each
  work. Both are pasted whole into the built-in agent's system prompt at the
  start of every turn with a line saying how much of the budget is used, and
  both appear in the story brief an external MCP client reads. A new
  `linetta_edit_memory` tool records into them with `add`, `replace` and
  `remove`; like every other write tool it is registered only in full-access
  mode, and a client pinned to a single work is refused on the writer profile.
  Settings → Memory shows both as textareas you can rewrite yourself.
- The writer profile is **global**: it is not scoped to the work you have open.
  Something an agent records there while working on one book is in the system
  prompt of every other book, and in the story brief a connected client
  receives on any work. Work notes stay with their work. The consent text for a
  provider and for MCP now says so.
- Both memories live in `library.db`, so the daily backup carries them. The
  per-work restore below does not yet: bringing a single work back from a
  backup returns it without its work notes, which is tracked as
  [#116](https://github.com/devlikebear/linetta/issues/116). The startup
  recovery screen, which replaces the whole library file, does bring both back.
  The unbounded `memory/experiences.jsonl` recall is unchanged and still
  appears in the story brief — alongside both curated documents for an external
  client, and on its own for the built-in agent, which already carries them in
  its system prompt.
- An agent now also keeps **skills**: short how-to documents about method
  rather than about the story. Each is a plain `SKILL.md` file with
  agentskills.io YAML frontmatter in its own folder — `<app data>/skills/<name>/`
  for a skill that applies to every work, `<app data>/skills/works/<project
  id>/<name>/` for one work — so you can edit them in any editor, keep the
  folder in git, or point your own Claude Code at it. At most 40 per scope,
  200 characters of description and 8,000 of body.
- Only the names and descriptions of enabled skills go into the built-in
  agent's system prompt and into the story brief an MCP client reads; the body
  is fetched on demand. Two new tools: `linetta_read_skill`, a read tool
  registered in read-only mode as well, and `linetta_edit_skill`, a write tool
  registered only in full-access mode. A client pinned to a single work can
  write a skill for that work and is refused on a global one, exactly as it is
  on the writer profile. That makes ten read tools and nine write tools.
- **A skill under `skills/<name>/` is global**, like the writer profile: it is
  offered on every work, not only the one you have open. The consent text for
  a provider and for MCP now says so.
- **An agent writes a skill without asking.** There is no approval gate. What
  you get instead is a badge on every skill saying whether you or an agent
  wrote it, a version of every write you can read and revert to, and one
  switch that takes a skill out of the prompt without deleting it — all under
  Settings → Skills, where you can also write and edit skills yourself.
- Added a self-review that costs an extra model call. After a turn in which
  the built-in agent executed eight or more tool calls, and after your reply
  has already been sent, Linetta asks the same provider and the same model, in
  a separate request, whether the turn taught it a technique worth recording
  or showed a skill to be wrong. Most turns answer "nothing" in one round
  trip; one that writes a skill costs up to five. It is on by default and your
  provider meters it like any other call — Settings → Skills → "Learning from
  its own work" (`agent_self_review_enabled`) turns it off, and off means the
  call is not made.
- Added a **tool budget** you can spend: every tool's full description travels
  in every request, and the memory tool plus the two skills tools are about a
  fifth of Linetta's nineteen. Settings → Tool budget says how many tools the
  agent currently gets and what each group costs, and switches either group
  off — for the built-in agent and for any MCP client alike
  (`memory_tools_enabled`, `skill_tools_enabled`; both on by default, so
  nothing changes unless you open the pane). Switching a group off also
  removes the instructions that name its tools. With the memory tool off the
  recorded documents are still read back to the agent, which simply cannot add
  to them; with the skills tools off the skill list goes too, because a list
  with no way to open a body is only names. Nothing is deleted either way, and
  either switch takes effect on your next message rather than at the next
  restart.
- Skills live in files, and **the daily backup does not carry them**: it is
  `VACUUM INTO` on `library.db` and copies nothing else under the data
  directory. It does carry their history — every write and delete lands a row
  holding that skill's whole text in `library.db`. So a skill you only ever
  hand-wrote into the folder has no row anywhere, and a skill whose file is
  gone is not listed in Settings → Skills, which is also the only way to reach
  its history. Copy the folder yourself or keep it in git; tracked as
  [#119](https://github.com/devlikebear/linetta/issues/119).
- Fixed facts remembered before 1.0 being unreachable. 1.0 changed where
  `memory/experiences.jsonl` is looked for without moving the file, so every
  fact the retired companion had been told to remember sat in
  `<app data>/companion/<project id>/` where nothing read it, and the story
  brief's `## Memories` section was silently empty. Linetta now moves those
  directories into place at startup and removes the emptied `companion` folder.
  Nothing is merged: a project that already has memories in the new location is
  left untouched, along with its old directory
  ([#114](https://github.com/devlikebear/linetta/issues/114)).
- Fixed Linetta refusing to open its settings on Linux for anyone who had
  entered an API key before 3 June 2026. Those builds wrote the key in plain
  text into `settings.json`; the move into secure storage has nowhere to go on
  a platform with no credential store, and that failure came back out of the
  load, so every other preference in the file was unreachable too. The load now
  always completes and the key is left exactly as it was found — neither moved
  nor quietly deleted. Settings → AI provider says the key is still there, says
  where the file is, and offers to delete it; it also says plainly that a key
  left in that file does nothing, because Linetta reads provider keys only from
  the OS credential store
  ([#113](https://github.com/devlikebear/linetta/issues/113)).
- Fixed the model list for a ChatGPT (Codex) sign-in never loading, and the
  failure saying to sign in again when the sign-in was fine. The list was
  asked of the wrong host with the wrong caller version — a request that
  either got a 401, which then spent a perfectly good refresh token, or a 400.
  It now comes from the same backend the chat goes to and lists what that
  account can use, `gpt-6-astra` first. A model list that still will not load
  now says so, and says what to do — type a model name you know, or leave the
  field empty for the provider default — with the reason after it rather than
  as the headline ([#126](https://github.com/devlikebear/linetta/issues/126)).
- Settings panes added since 1.1 — AI provider, Memory, MCP, Sync, Backup —
  now read like the rest: help lines, counters and errors at the same small
  muted size, buttons on the same style, and the provider chips on the same
  segmented control.

## v1.1.0 - 2026-08-31

- Export now carries the whole work: synopsis, plot threads and beats, fact
  cards, margin notes, style notes, and each scene's status and summary ride in
  the markdown frontmatter, and import restores all of it. Git and folder sync
  write the same complete files.
- Restore no longer means replacing the library. Settings → Backup lists every
  backup, shows the works inside one, and brings a chosen work back as a new
  project — nothing you are writing now is touched, and a safety snapshot is
  taken first.
- Linetta can stay running in the system tray after the window closes, so a
  connected agent never loses its MCP connection, and can start hidden at
  login. Both are opt-in toggles under Settings → Run in background.
- Settings got a sidebar: App / Writing / Connections / Data categories replace
  the single long page.

## v1.0.0 - 2026-08-29

Linetta is a writing tool. It no longer talks to a language model.

- Removed the built-in AI companion, the provider settings, and every path that
  sent a manuscript to a third party. Linetta stores no API keys and calls no
  models.
- Added an MCP server hosted inside the app, so the agent you already run —
  Claude Code, Claude Desktop — can read and write the work directly. It is off
  until you consent, binds `127.0.0.1` only, and needs a token generated on your
  machine. `read_only` mode omits the writing tools entirely.
- Every change an agent makes is snapshotted before it lands and recorded in an
  activity log. A restructuring of the outline can be undone as one batch while
  the app stays open; a rewritten scene is restored from its version history.
  A scene you are part way through editing is never replaced behind your back.
- Scene summaries no longer need a model: a short scene is its own summary, a
  longer one keeps its opening, and an agent can replace either with a real one.
- The Fact Book is now a place to record what you checked and where, rather than
  to ask. Cards are readable by a connected agent.
- Existing companion conversations are preserved. Settings → Backup can export
  every project's transcript and remembered facts as one markdown file.
- Provider API keys already in your OS keychain are left untouched; remove them
  there if you want them gone.
- `Cmd/Ctrl+J` and `Cmd/Ctrl+I` are unbound.
- Added a Story World panel: the characters, places, items and concepts your
  work has registered, with per-kind lists, search across names and aliases, and
  a click through to the record. When an agent creates three characters, there
  is now somewhere to go and look.
- Registered names appearing in your prose are noticed as you write, instead of
  waiting for you to press a scan button. They are counted, not linked — a
  homonym would be linked to the wrong record, so applying stays your call.
- Engine failure and import messages now speak the reader's language. They were
  Korean sentences written in the engine, which an English or Japanese writer
  saw verbatim. So do the section headings in an exported manuscript, and the
  companion archive. Import reads all three, so a file exported in one language
  still opens after you switch.
- Fixed a character marked Protagonist counting for nothing unless you write in
  Korean. The engine matched core story roles against Korean labels only, so a
  connected agent asking for story context got a work with no cast.
- Markdown export says where the file landed, and names the sync folder when it
  landed in one.

## v0.9.6 - 2026-08-22

- Fixed folder sync and backup restore on Windows, which failed on every write because the atomic-write flush ran on a read-only file handle.
- Fixed the OpenRouter sign-in flow, which never opened a browser on Windows and left the writer to follow the fallback link by hand.
- Ran the verification gate on Windows in CI, and included the Tauri shell's Rust unit tests in it, so platform regressions are caught before a release.

## v0.9.5 - 2026-08-22

- Made Windows a working development and runtime target: LF checkouts, a debug-build fix for the embedded engine DLL, and Claude Code CLI detection that no longer relies on Unix permission bits.
- Added a Windows Credential Manager backend so API keys can be stored outside macOS; key-storage copy now names the platform's own store.
- Fixed the OpenRouter model list, which returned nothing once the API began reporting structured pricing values.

## v0.9.4 - 2026-08-03

- Updated React Router to address current client-routing security advisories and added a guard that rejects the RSC-only audit exception if RSC APIs are introduced.

## v0.9.3 - 2026-08-03

- Fixed AI provider connection tests to honor explicit data-sharing consent for the active provider without reusing consent across providers.

## v0.9.2 - 2026-07-14

- Prevented stale autosaves from overwriting another scene by serializing per-scene writes and enforcing optimistic content-version checks.
- Added verified daily and pre-migration backups, full-library `.linetta` recovery snapshots, startup restore controls, and atomic sync/import writes with partial-failure reporting.
- Required explicit, revocable consent before manuscript content is sent to a selected AI provider, and aligned the in-app disclosure and Korean/English/Japanese privacy policy.
- Narrowed renderer IPC and file-opening access, preserved structured RPC errors, aligned localization catalogs, and added lint and dependency-audit gates to CI.

## v0.9.1 - 2026-07-05

- Localized every AI surface for the selected app language (Korean/English/Japanese): the companion, inline editor AI, contextual edit, and Fact Book now build prompts and reply in the app language, including tool status text, apply results, and query outputs.
- Translated canonical outline node labels (부/장/씬, 권/화) in AI prompts and the workspace UI, so an English or Japanese UI shows Arc 1/Episode 1 or 第1巻/第1話 instead of raw Korean labels.
- Added English and Japanese keywords to the companion's intent detection so requests like "rewrite this scene" or 「書き直して」 apply changes directly, matching the Korean behavior.
- Added an English (U.S.) App Store listing.

## v0.9.0 - 2026-06-24

- Promoted desktop distribution to a public release: the macOS app is Developer ID signed and notarized through the Homebrew tap, with Windows (NSIS/MSI) and Linux (AppImage/deb/rpm) prebuilt installers published on every GitHub Release.
- Updated the README install guide to document the notarized macOS path and removed the obsolete quarantine workaround.

## v0.8.5 - 2026-06-23

- Improved the companion action picker with clearer current-scene and whole-work action scopes, plus persistent curated actions after a choice is used.
- Refined the OpenRouter beginner path with smarter model presets, friendlier credit/key-limit errors, and connection tests that avoid leaking raw provider JSON.
- Fixed Tauri dev/build invalidation so embedded Go engine changes are rebuilt when internal engine files change.

## v0.8.4 - 2026-06-22

- Added inline OpenRouter API key entry, model selection, model refresh, and connection testing to the beginner AI setup wizard.
- Loaded available OpenRouter models from the OpenRouter API while keeping `openrouter/auto` as the safe default.
- Compacted the companion AI setup dialog with tabbed setup choices, collapsed steps, and bounded modal height so it no longer gets cut off.

## v0.8.3 - 2026-06-21

- Replaced raw companion AI setup failures with a guided recovery card that preserves the blocked prompt for retry.
- Added a reusable AI setup surface with a beginner OpenRouter path, subscription guidance, and advanced direct-key options.
- Added OpenRouter provider support with OAuth PKCE connection, Keychain-backed key storage, model defaults, and key limit/credit status checks.

## v0.8.1 - 2026-06-21

- Restored Windows desktop CI packaging with NSIS and MSI installers.
- Build the embedded Go engine as `linetta_engine_ffi.dll` on Windows and load it at runtime to avoid MSVC c-archive link failures.
- Documented the Windows embedded-engine packaging path and added distribution validation coverage for the DLL bundle.

## v0.8.0 - 2026-06-21

- Embedded the Go engine in-process through a C ABI for desktop and mobile builds, removing the packaged desktop sidecar path.
- Added iOS xcframework and Android shared-library build scripts, mobile engine CI, and manual mobile release workflows.
- Added responsive Workspace coverage for mobile-sized screens and release smoke tooling for iOS simulator and Android APK/AAB packaging.

## v0.4.17 - 2026-06-14

- Made companion scene-writing and scene-edit requests transactional, so current-scene text changes require verified `set_scene_text` application instead of accepting model-only success claims.
- Added explicit companion scene intents, apply readback metadata, and current-editor refresh from verified changed-node events.
- Added retry handling for failed companion scene edits and kept failed proposal applications from refreshing as successful.

## v0.4.16 - 2026-06-14

- Kept long companion responses attached to the project session when the companion panel is closed or remounted, so completed replies and apply proposals are still visible when the writer returns.
- Added project-aware companion stream events so background companion runs can be routed safely across panel and screen changes.
- Serialized SQLite access through one connection and closed Fact Book cursors before nested source queries to avoid `SQLITE_BUSY` and single-connection deadlocks.

## v0.4.15 - 2026-06-14

- Turned the companion empty state into a writer action palette for continuing a scene, smoothing dialogue, raising tension, checking continuity, shaping the next-episode hook, and finishing an episode.
- Fixed companion proposal handling so `set_scene_text` scene-replacement ops from proofread/rewrite flows are preserved and labeled as current-scene text replacement instead of exposing raw op names.
- Replaced raw companion tool capability chips with writer-facing labels while keeping search, URL reading, apply-ops, context, and image attachment controls available.

## v0.4.14 - 2026-06-12

- Fixed the "이번 화" character count in the editor footer and ZEN mode summing the whole 권 when writing in a leaf episode created directly under an arc.
- Showed the episode character gauge on leaf episodes in the outline rail, matching container episodes.
- Seeded new webnovel projects with a `1권 > 1화` outline instead of a root `씬 1` so the first keystroke lands in an episode.

## v0.4.12 - 2026-06-07

- Fixed the Windows desktop build that broke in v0.4.11: bumped the bundled `tars` engine to v0.34.0, whose `claude-code-cli` process-teardown used unix-only syscalls. Updated to tars v0.34.1, which builds for Windows again. macOS and Linux were unaffected.

## v0.4.11 - 2026-06-07

- Upgraded the bundled `tars` LLM engine to v0.34.0: `claude-code-cli` runs are now bounded by a timeout (`CLAUDE_CODE_CLI_TIMEOUT`, default 5m) and a hung run no longer blocks until cancelled, descendant processes are killed as a group, and per-call startup is ~1s faster.

## v0.4.10 - 2026-06-07

- Recovered leaked `linetta_apply_ops` JSON into applyable Fact Book proposal cards instead of showing raw tool payloads.
- Wrapped long Fact Book assistant text, source URLs, and proposal cards so feedback stays inside the side panel.
- Added a Fact Book retry action for failed source URL saves so writers can search for an alternative source without restarting the review.

## v0.4.9 - 2026-06-07

- Added story-element presets for items, skills, magic, abilities, and relationship labels in the Entity Sheet.
- Extended companion story-state tools so worldbuilding elements can store structured attributes such as effects, costs, triggers, limits, and weaknesses.
- Renamed entity and AI context surfaces toward story/worldbuilding elements so items and concepts appear alongside characters and places.

## v0.4.7 - 2026-06-06

- Added companion context inspection, per-message copy controls, and image attachments from file upload or clipboard paste.
- Added outline inspection and repair tools that preserve node content while moving, restoring, renumbering, and undoing structure changes.
- Improved outline editing controls for parts, chapters, and scenes, including scrollable repair issue lists for long diagnostics.

## v0.4.6 - 2026-06-06

- Improved Fact Book source saving with direct URL fallback, clearer in-panel failure feedback, and web search API key connection testing.
- Added editor selection actions for fact-checking selected blocks and replacing selected text with AI.
- Added Linetta frontmatter metadata to markdown exports so imported markdown restores characters, places, and relationships instead of treating metadata appendices as scenes.

## v0.4.5 - 2026-06-05

- Added a Fact Book persistence layer with sourced fact cards for project-wide and scene-linked claims.
- Added companion support for creating sourced fact cards and reusing saved Fact Book context in later responses.
- Added a Workspace Fact Book panel with current-scene review prompts, source links, deletion, and localized UI copy.

## v0.4.4 - 2026-06-05

- Added a Homebrew cask post-install step that clears the macOS quarantine attribute for hobby releases that are not notarized yet.
- Kept optional Developer ID signing and notarization support in the release workflow for a future public distribution path.

## v0.4.3 - 2026-06-04

- Added written draft excerpts to companion context so character, relationship, and scene analysis can use the actual manuscript text.
- Saved the current editor document immediately before companion sends to avoid stale or missing draft context.
- Added an outline-building companion prompt example and made the companion composer grow for multi-line prompts.

## v0.4.2 - 2026-06-04

- Added companion empty-state prompt examples and a built-in tool help toggle.
- Added companion help copy for `web_search`, `web_fetch`, and `linetta_apply_ops` in Korean, English, and Japanese.
- Fixed OpenAI Codex ChatGPT-account connections by defaulting to `gpt-5.3-codex-spark` and replacing the unsupported legacy `gpt-5.3-codex` setting.

## v0.4.1 - 2026-06-04

- Added Windows and Linux release packaging for NSIS, MSI, AppImage, deb, and rpm bundles.
- Added winget manifest rendering and Flathub submission starter metadata.
- Added distribution metadata validation for release automation.

## v0.4.0 - 2026-06-03

- Added a guided onboarding tour for first-run users across the library and workspace.
- Added Settings controls to enable or disable automatic tours and replay the tour manually.
- Added persisted tour state so completed or skipped tours stay dismissed until the tour version changes.
- Localized onboarding tour copy in Korean, English, and Japanese.

## v0.3.0 - 2026-06-03

- Added Korean, English, and Japanese app UI language support with Korean as the default.
- Localized workspace, settings, search, AI generation, companion, entity, relationship, thread, version, import, and engine-gate surfaces.
- Added language-aware display for default scene and chapter labels without mutating stored manuscript data.

## v0.2.16 - 2026-06-03

- Added a beginner AI setup wizard with official setup links for OpenAI Codex, OpenAI API, Claude API, and Gemini API.
- Added a Settings connection test for the active AI provider.
- Moved provider and web-search API keys out of `settings.json` into macOS Keychain with redacted Settings responses.
