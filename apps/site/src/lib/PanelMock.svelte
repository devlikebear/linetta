<script lang="ts">
  import type { PanelMockId, Translation } from '$lib/content';

  let { id, t }: { id: PanelMockId; t: Translation } = $props();

  const mock = $derived(t.hero.mock);
  const parent = $derived(mock.breadcrumb[1] ?? mock.breadcrumb[0]);
</script>

<!--
  Structural sketches, deliberately text-light: they show the shape of each
  panel without inventing UI copy that would then drift from the real app.
-->
<div class="mock" role="img" aria-label={t.workspace.panels.find((p) => p.id === id)?.title ?? ''}>
  {#if id === 'outline'}
    <div class="tree">
      <div class="branch">▾ {parent}</div>
      {#each mock.railItems as item, i}
        <div class="leaf" class:on={i === 0}>
          <span class="dot" class:filled={i < 2}></span>
          <span class="leaf-label">{item.label}</span>
          <span class="bar"><i style="width:{[64, 20, 0][i] ?? 0}%"></i></span>
          <span class="num">{item.meta}</span>
        </div>
      {/each}
      <div class="leaf ghost"><span class="dot"></span><span class="leaf-label">+</span></div>
    </div>
  {:else if id === 'editor'}
    <div class="prose">
      <p>{mock.prose[0]}</p>
      <p class="dim">
        <span class="mention">{mock.breadcrumb[0]}</span> — <span class="skel w-70"></span>
        <span class="skel w-45"></span>
      </p>
      <div class="note">
        <span class="note-mark">✎</span>
        <span class="skel w-60"></span>
        <span class="skel w-35"></span>
      </div>
    </div>
  {:else if id === 'factbook'}
    <div class="cards">
      {#each [0, 1] as card}
        <div class="fact">
          <div class="src">https://<span class="skel inline w-30"></span></div>
          <div class="skel w-90"></div>
          <div class="skel w-65"></div>
        </div>
      {/each}
    </div>
  {:else if id === 'contextual'}
    <div class="rows">
      <div class="swap">
        <span class="old">A</span>
        <span class="arrow">→</span>
        <span class="new">B</span>
      </div>
      {#each mock.railItems as item, i}
        <div class="row">
          <span class="box" class:checked={i < 2}></span>
          <span class="row-label">{item.label}</span>
          <span class="skel flex-1"></span>
        </div>
      {/each}
    </div>
  {:else}
    <div class="spine">
      {#each [0, 1, 2] as line}
        <div class="thread">
          <span class="thread-line"></span>
          {#each [0, 1, 2, 3] as beat}
            <span
              class="beat"
              class:open={(line + beat) % 3 === 0}
              style="left:{12 + beat * 27}%"
            ></span>
          {/each}
        </div>
      {/each}
      <div class="axis">
        {#each mock.railItems as item}<span>{item.label}</span>{/each}
      </div>
    </div>
  {/if}
</div>

<style>
  .mock {
    border: 1px solid var(--color-line-soft);
    border-radius: 6px;
    background: color-mix(in srgb, var(--color-surface) 70%, transparent);
    padding: 1.15rem 1.25rem;
    min-height: 13.5rem;
    display: flex;
    flex-direction: column;
    justify-content: center;
  }

  .skel {
    display: block;
    height: 7px;
    border-radius: 3px;
    background: var(--color-line);
    margin: 7px 0;
  }
  .skel.inline { display: inline-block; vertical-align: middle; margin: 0; }
  .w-90 { width: 90%; }
  .w-70 { width: 70%; }
  .w-65 { width: 65%; }
  .w-60 { width: 60%; }
  .w-45 { width: 45%; }
  .w-35 { width: 35%; }
  .w-30 { width: 30%; }
  .flex-1 { flex: 1; }

  /* outline */
  .branch {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--color-muted);
    margin-bottom: 8px;
  }
  .leaf {
    display: flex;
    align-items: center;
    gap: 9px;
    padding: 7px 9px;
    border-radius: 4px;
    margin-left: 12px;
  }
  .leaf.on { background: var(--color-surface); box-shadow: inset 2px 0 0 var(--color-accent); }
  .leaf.ghost { opacity: 0.45; }
  .leaf .dot {
    width: 6px; height: 6px; border-radius: 999px;
    border: 1px solid var(--color-muted-2);
  }
  .leaf .dot.filled { background: var(--color-accent); border-color: var(--color-accent); }
  .leaf-label { font-size: 13px; color: var(--color-ink-soft); min-width: 3.5rem; }
  .bar { flex: 1; height: 3px; border-radius: 2px; background: var(--color-line); overflow: hidden; }
  .bar i { display: block; height: 100%; background: var(--color-accent); }
  .num { font-family: var(--font-mono); font-size: 9.5px; color: var(--color-muted-2); }

  /* editor */
  .prose p { margin: 0 0 0.7em; font-size: 13.5px; line-height: 1.8; color: var(--color-ink-soft); }
  .prose .dim { color: var(--color-muted); }
  .mention {
    color: var(--color-accent);
    border-bottom: 1px dashed color-mix(in srgb, var(--color-accent) 55%, transparent);
  }
  .note {
    margin-top: 4px;
    border-left: 2px solid var(--color-sienna);
    background: color-mix(in srgb, var(--color-sienna) 8%, transparent);
    padding: 8px 11px;
    border-radius: 0 4px 4px 0;
  }
  .note-mark { font-family: var(--font-mono); font-size: 11px; color: var(--color-sienna); }

  /* fact book */
  .cards { display: grid; gap: 10px; }
  .fact {
    border: 1px solid var(--color-line-soft);
    border-radius: 5px;
    padding: 10px 12px;
    background: var(--color-surface);
  }
  .src { font-family: var(--font-mono); font-size: 10px; color: var(--color-muted-2); margin-bottom: 8px; }

  /* contextual edit */
  .swap {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    font-family: var(--font-mono);
    font-size: 11px;
    margin-bottom: 12px;
  }
  .swap .old { color: var(--color-muted-2); text-decoration: line-through; }
  .swap .arrow { color: var(--color-muted-2); }
  .swap .new { color: var(--color-accent); }
  .row { display: flex; align-items: center; gap: 10px; padding: 6px 0; }
  .box {
    width: 12px; height: 12px; flex: none;
    border: 1px solid var(--color-muted-2);
    border-radius: 3px;
  }
  .box.checked { background: var(--color-accent); border-color: var(--color-accent); }
  .row-label { font-size: 12.5px; color: var(--color-ink-soft); min-width: 3.5rem; }
  .row .skel { margin: 0; }

  /* plot */
  .spine { display: grid; gap: 16px; }
  .thread { position: relative; height: 12px; }
  .thread-line {
    position: absolute; inset: 50% 0 auto; height: 1px;
    background: var(--color-line); transform: translateY(-50%);
  }
  .beat {
    position: absolute; top: 50%; transform: translate(-50%, -50%);
    width: 9px; height: 9px; border-radius: 999px;
    background: var(--color-accent);
  }
  .beat.open { background: var(--color-surface); border: 1.5px solid var(--color-muted-2); }
  .axis {
    display: flex; justify-content: space-around;
    font-family: var(--font-mono); font-size: 9.5px; color: var(--color-muted-2);
    border-top: 1px solid var(--color-line-soft); padding-top: 8px;
  }
</style>
