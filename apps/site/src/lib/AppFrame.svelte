<script lang="ts">
  import type { Translation } from '$lib/content';
  let { t }: { t: Translation } = $props();
  const m = $derived(t.hero.mock);
</script>

<!--
  A drawing of the workspace, not a screenshot: it is built from the app's own
  tokens (see src/app.css) so it stays right when the shipped UI moves, and it
  never shows a panel the app no longer has.
-->
<figure class="frame" aria-label={m.window}>
  <div class="titlebar">
    <span class="light" style="background:#e0685c"></span>
    <span class="light" style="background:#d9a441"></span>
    <span class="light" style="background:#78ab63"></span>
    <span class="win">{m.window}</span>
  </div>

  <div class="topbar">
    <span class="crumbs">
      {#each m.breadcrumb as crumb, i}
        {#if i > 0}<span class="sep">›</span>{/if}<span class:last={i === m.breadcrumb.length - 1}>{crumb}</span>
      {/each}
    </span>
    <span class="keys">⌘K · ⌘F</span>
  </div>

  <div class="body">
    <aside class="rail">
      <div class="rail-label">{m.rail}</div>
      {#each m.railItems as item}
        <div class="node" class:on={item.active}>
          <span class="node-label">{item.label}</span>
          <span class="node-meta">{item.meta}</span>
        </div>
      {/each}
    </aside>

    <div class="page">
      <div class="kicker"><span>{m.kicker}</span><i></i></div>
      <h3 class="scene-title">{m.title}</h3>
      {#each m.prose as para, i}
        <p>{para}{#if i === m.prose.length - 1}<span class="caret"></span>{/if}</p>
      {/each}
    </div>
  </div>

  <div class="statusbar">{m.status}</div>

  <div class="badge">
    <span class="dot"></span>
    {m.badge}
  </div>
</figure>

<style>
  .frame {
    position: relative;
    margin: 0;
    border: 1px solid var(--color-line);
    border-radius: 10px;
    background: var(--color-surface);
    box-shadow:
      0 1px 0 color-mix(in srgb, var(--color-surface) 60%, white),
      0 24px 60px -30px rgb(var(--shade) / 0.45);
    overflow: hidden;
  }

  .titlebar {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 9px 12px;
    background: var(--color-surface-2);
    border-bottom: 1px solid var(--color-line);
  }
  .light { width: 9px; height: 9px; border-radius: 999px; opacity: 0.85; }
  .win {
    margin-left: 10px;
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--color-muted);
  }

  .topbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    padding: 10px 14px;
    border-bottom: 1px solid var(--color-line-soft);
    font-size: 12.5px;
    color: var(--color-muted);
  }
  .crumbs { display: flex; align-items: center; gap: 6px; min-width: 0; }
  .crumbs .sep { color: var(--color-muted-2); }
  .crumbs .last { color: var(--color-accent); font-style: italic; }
  :global(html:lang(ko)) .crumbs .last,
  :global(html:lang(ja)) .crumbs .last { font-style: normal; }
  .keys { font-family: var(--font-mono); font-size: 10.5px; color: var(--color-muted-2); }

  .body { display: grid; grid-template-columns: 8.5rem 1fr; min-height: 20rem; }

  .rail {
    border-right: 1px solid var(--color-line-soft);
    padding: 12px 10px;
    background: color-mix(in srgb, var(--color-surface-2) 45%, transparent);
  }
  .rail-label {
    font-family: var(--font-mono);
    font-size: 10px;
    letter-spacing: 0.14em;
    text-transform: uppercase;
    color: var(--color-muted-2);
    margin-bottom: 10px;
  }
  .node {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 6px 8px;
    border-radius: 4px;
    margin-bottom: 2px;
  }
  .node.on {
    background: var(--color-surface);
    box-shadow: inset 2px 0 0 var(--color-accent);
  }
  .node-label { font-size: 12.5px; color: var(--color-ink-soft); }
  .node-meta { font-family: var(--font-mono); font-size: 9.5px; color: var(--color-muted-2); }

  .page { padding: 22px 26px 26px; min-width: 0; }
  .kicker {
    display: flex;
    align-items: center;
    gap: 10px;
    font-family: var(--font-mono);
    font-size: 10px;
    letter-spacing: 0.16em;
    text-transform: uppercase;
    color: var(--color-muted-2);
  }
  .kicker i { flex: 1; height: 1px; background: var(--color-line); }

  .scene-title {
    font-size: clamp(1.6rem, 3vw, 2.1rem);
    margin: 10px 0 14px;
    letter-spacing: -0.02em;
  }
  .page p {
    margin: 0 0 0.85em;
    font-size: 14.5px;
    line-height: 1.85;
    color: var(--color-ink-soft);
  }

  .caret {
    display: inline-block;
    width: 1.5px;
    height: 1em;
    margin-left: 1px;
    vertical-align: -0.15em;
    background: var(--color-accent);
    animation: blink 1.15s steps(1, end) infinite;
  }
  @keyframes blink { 0%, 55% { opacity: 1; } 56%, 100% { opacity: 0; } }
  @media (prefers-reduced-motion: reduce) { .caret { animation: none; } }

  .statusbar {
    border-top: 1px solid var(--color-line-soft);
    padding: 9px 14px;
    font-family: var(--font-mono);
    font-size: 10.5px;
    color: var(--color-muted-2);
    background: color-mix(in srgb, var(--color-surface-2) 40%, transparent);
  }

  .badge {
    position: absolute;
    right: 12px;
    bottom: 40px;
    display: flex;
    align-items: center;
    gap: 7px;
    max-width: min(21rem, 78%);
    padding: 7px 11px;
    border-radius: 999px;
    border: 1px solid var(--color-line);
    background: var(--color-surface);
    box-shadow: 0 10px 24px -14px rgb(var(--shade) / 0.5);
    font-family: var(--font-mono);
    font-size: 10px;
    line-height: 1.35;
    color: var(--color-muted);
  }
  .badge .dot {
    width: 6px;
    height: 6px;
    flex: none;
    border-radius: 999px;
    background: var(--color-accent);
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-accent) 22%, transparent);
  }

  @media (max-width: 30rem) {
    .body { grid-template-columns: 1fr; }
    .rail { display: none; }
    .badge { position: static; margin: 0 12px 12px; max-width: none; }
  }
</style>
