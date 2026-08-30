# linetta-site

Marketing site for [Linetta](https://github.com/devlikebear/linetta) — a local-first
writing studio for long-form fiction.

Deployed at **https://linetta.marvin-42.com** via Cloudflare Pages.

## Stack

- SvelteKit 2 + Svelte 5 (runes)
- TypeScript
- Tailwind CSS v4
- `@sveltejs/adapter-static` (fully prerendered, no server at runtime)

## Develop

```sh
cd apps/site
pnpm install
pnpm dev      # http://localhost:5173
pnpm build    # output → ./build
pnpm preview  # serve the built output
pnpm check    # svelte-check + tsc
```

From the repository root, `make site-dev`, `make site-build` and `make site-check`
do the same thing.

## Languages

Three prerendered pages share one component tree and one content file:

| Route  | Locale | Content export        |
| ------ | ------ | --------------------- |
| `/`    | en     | `en` in `content.ts`  |
| `/ko`  | ko     | `ko` in `content.ts`  |
| `/ja`  | ja     | `ja` in `content.ts`  |

All copy lives in `src/lib/content.ts`, typed by `Translation` — adding a field to
the type breaks every locale that has not been updated, which is the point. The
`<html lang>` attribute is stamped per route by `src/hooks.server.ts`, because
`svelte:head` cannot reach the root element.

## Design tokens

`src/app.css` mirrors the desktop app's default **hanji** palette
(`apps/desktop/src/App.css`) — celadon `#2d6f64` on sage-cast paper, Newsreader
and IBM Plex Mono — including that palette's dark set, which the site uses for
`prefers-color-scheme: dark`. Keep the two in step when the app's palette moves.

The product visuals in `src/lib/AppFrame.svelte` and `src/lib/PanelMock.svelte`
are drawings built from those tokens, not screenshots. That is deliberate: a
screenshot goes stale the moment a panel is added or removed, and the repository's
existing captures still show a companion the app no longer has.

## Crawlers

`static/robots.txt` points at `/sitemap.xml`, which is prerendered by
`src/routes/sitemap.xml/+server.ts` from the same `translations` catalogue the
pages use. Adding a locale therefore updates the sitemap, the `hreflang` links
and the pages together.

The deployed origin lives in one place — `SITE` in `src/lib/content.ts`. Canonical
tags, `hreflang` links and the sitemap all derive from it, so moving the site to
another domain means changing that constant, the `Sitemap:` line in `robots.txt`,
and the absolute `og:image` URLs in `src/app.html`.

## Deploy

Cloudflare Pages builds from `main`:

- Root directory: `apps/site`
- Build command: `pnpm build`
- Output directory: `build`
- Node version: 22 or later

Everything is prerendered, so the deployment is static files only.

## Social card

`static/og-image.png` (1200×630) is generated from `scripts/og-image.html`:

```sh
chromium --headless --window-size=1200,630 \
  --screenshot=static/og-image.png scripts/og-image.html
```

Regenerate it when the headline or the palette changes.

## Content accuracy

The site describes the app at `main`, which since the MCP-first pivot has **no
built-in AI**: no provider settings, no API key field, no in-app companion. AI
collaboration happens through the optional local MCP server.

Claims in `content.ts` are taken from the engine rather than from the docs — the
fifteen `linetta_*` tool names, the nine/six read-write split, the access modes,
the 7391 default port, the per-minute call limit. When one of them changes, the
engine is the thing to check.

`VERSION` in `content.ts` is the one number nothing derives: bump it by hand when
a release ships.
