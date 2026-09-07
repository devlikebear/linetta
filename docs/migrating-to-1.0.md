# Moving to Linetta 1.0

Linetta 1.0 removes the built-in AI companion. The app is a writing tool now; AI
collaboration happens in a client you already run, connected over MCP.

If you never used the companion, nothing about your writing changes and you can
skip this page.

## What happened to your conversations

They are still there. Linetta does not drop the table.

Before or after upgrading, open **Settings → Backup → Export companion record**.
It writes one markdown file containing every project's transcript and the facts
the companion was told to remember, readable without Linetta installed. The row
only appears if your library actually holds a transcript.

Remembered facts were never in the database — they live in a per-project
`memory/experiences.jsonl` file, and an agent connected over MCP records and
recalls them the same way the companion did.

**Facts remembered before 1.0 needed rescuing, and Linetta now does it for
you.** 1.0 moved where that file is looked for — it was
`<app data>/companion/<project id>/memory/`, and it is now
`<app data>/<project id>/memory/` — but nothing moved the file, so for a while
an agent no longer recalled what the companion had remembered. Nothing was ever
deleted.

Since 1.2, Linetta moves those directories up one level when it starts, and
removes the `companion` folder once it is empty. There is nothing to click:
open the app and the facts are back where an agent reads them
([#114](https://github.com/devlikebear/linetta/issues/114)).

Two cases it deliberately leaves alone, because guessing would risk your data:

- **A project that already has memories in the new place.** If both
  `<app data>/companion/<project id>/` and `<app data>/<project id>/` exist,
  Linetta touches neither — merging two `experiences.jsonl` files is not
  something it will do behind your back. The file is one JSON object per line,
  so you can append the old file's lines to the new one yourself.
- **A `companion` folder holding anything other than project memory.** Only a
  directory containing `memory/experiences.jsonl` is treated as memory and
  moved; anything else stays, and so does the folder around it. A symlink is
  not followed either.

So a `<app data>/companion/` still there after an upgrade is holding something
Linetta would not move without you — or, rarely, something it could not move,
such as a folder it lacks permission to write to. Nothing in it was changed,
and the manual fix is the same in every case: move the project directories up
one level into `<app data>/` yourself, or merge the lines by hand where both
files exist.

## What happened to your API keys, and what changes in 1.2

They were untouched in 1.0: Linetta stopped reading them, but did not delete
credentials you own.

**In 1.2, the built-in agent reads them again** — but only for a provider you
explicitly consent to. Being left alone in 1.0 is not consent: each provider
needs its own yes, given again in **Settings → AI provider**, and Linetta
refuses to build a request to any provider until that box is ticked. A key
sitting untouched in your OS credential store does not quietly start being
used.

If you would still rather have a credential gone than consent to it, here is
where to go and delete it:

| Platform | Where |
| --- | --- |
| macOS | Keychain Access → search `linetta` |
| Windows | Credential Manager → Windows Credentials → `linetta` entries |
| Linux | there is no secure secret store here, so a key you enter today cannot be saved and only the ChatGPT (Codex) sign-in works, storing its token in `<app data>/codex/auth.json`. **A key you entered before 3 June 2026 is a different matter**: those builds wrote it in plain text into `<app data>/settings.json`, under `providers`, and on Linux nothing has moved or cleared it since. Settings → AI provider now says so and offers to delete it for you; you can also open the file and delete the `api_key` value yourself |

On Linux `<app data>` is `$XDG_DATA_HOME/com.devlikebear.linetta`, or
`~/.local/share/com.devlikebear.linetta` when `XDG_DATA_HOME` is unset.

Your provider selection and model choices stay in `settings.json`, and in 1.2
they mean something again: it is what the built-in agent reads to decide which
provider to call.

## Setting up an agent instead

1. **Settings → Connect an external agent (MCP)**, tick the consent box, choose
   `read_only` or `full`, and press Enable.
2. Copy the `claude mcp add …` line the pane shows and run it. That is the whole
   setup for Claude Code.
3. For Claude Desktop, paste the `claude_desktop_config.json` snippet instead —
   it points at the bridge binary shipped inside the app.

Restrict access to one work with the project field if you want an agent nowhere
near the rest of your library.

## What is different in practice

**You keep the last word.** Every scene an agent writes is snapshotted first and
listed in the activity log, and `linetta_undo_last_change` reverses it. If you
are part way through editing a scene when an agent rewrites it, Linetta does not
replace your text — it tells you and waits.

**Summaries no longer come from a model.** A short scene is its own summary; a
longer one keeps its opening lines. An agent replaces either with a real summary
through `linetta_write_summary`, and Linetta will not overwrite what it wrote.

**Some things are gone.** The synopsis "rewrite" button needed a model, so it
went; the field is still typed into by hand or written by an agent. The Fact
Book no longer researches for you — you record a claim and its source, and
Linetta fetches the page. Web search settings are gone with the companion that
used them.

**References are read-only.** Material you attached before still reaches an
agent's story brief, but there is no longer a way to add more. A proper place to
manage them is planned.
