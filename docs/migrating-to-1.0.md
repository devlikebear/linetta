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

Remembered facts were never in the database — they live in
`<app data>/companion/<project>/memory/experiences.jsonl` and keep working:
an agent connected over MCP can still record and recall them.

## What happened to your API keys

They are untouched. Linetta no longer reads them, but it does not delete
credentials you own.

To remove them yourself:

| Platform | Where |
| --- | --- |
| macOS | Keychain Access → search `linetta` |
| Windows | Credential Manager → Windows Credentials → `linetta` entries |
| Linux | your Secret Service keyring (`seahorse`, `kwalletmanager`), or `~/.local/share/com.devlikebear.linetta/secrets` |

Your provider selection and model choices stay in `settings.json`. Nothing reads
them; a future release may clean them up.

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
