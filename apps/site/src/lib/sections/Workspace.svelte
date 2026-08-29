<script lang="ts">
  import PanelMock from '$lib/PanelMock.svelte';
  import { reveal } from '$lib/reveal';
  import type { Translation } from '$lib/content';

  let { t }: { t: Translation } = $props();

  let active = $state(0);
  const panel = $derived(t.workspace.panels[active]);
</script>

<section id="workspace" class="border-b border-line">
  <div class="wrap py-20 md:py-28">
    <div class="max-w-2xl" use:reveal>
      <p class="mark">{t.workspace.mark}</p>
      <h2 class="mt-4 text-[clamp(1.9rem,3.6vw,2.7rem)]">{t.workspace.heading}</h2>
      <p class="mt-5 text-ink-soft">{t.workspace.sub}</p>
    </div>

    <div class="mt-12" use:reveal={60}>
      <div
        class="-mx-6 flex gap-1 overflow-x-auto px-6 pb-px [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
        role="tablist"
        aria-label={t.workspace.heading}
      >
        {#each t.workspace.panels as p, i}
          <button
            type="button"
            role="tab"
            id="tab-{p.id}"
            aria-selected={active === i}
            aria-controls="panel-{p.id}"
            tabindex={active === i ? 0 : -1}
            onclick={() => (active = i)}
            class="shrink-0 whitespace-nowrap border-b-2 px-4 py-3 font-mono text-[0.72rem] uppercase tracking-[0.12em] transition-colors
              {active === i
              ? 'border-accent text-ink'
              : 'border-transparent text-muted-2 hover:text-ink-soft'}"
          >
            {p.tab}
          </button>
        {/each}
      </div>
      <hr class="rule -mt-px" />

      {#key panel.id}
        <div
          id="panel-{panel.id}"
          role="tabpanel"
          aria-labelledby="tab-{panel.id}"
          class="fade grid gap-10 pt-10 md:grid-cols-12 md:gap-12"
        >
          <div class="md:col-span-6 lg:col-span-5">
            <p class="mark">{panel.kicker}</p>
            <h3 class="mt-3 text-[clamp(1.35rem,2.4vw,1.75rem)]">{panel.title}</h3>
            <p class="mt-4 text-ink-soft">{panel.body}</p>
            <ul class="mt-6 space-y-3">
              {#each panel.points as point}
                <li class="flex gap-3 text-[0.95rem] leading-[1.7] text-muted">
                  <span class="mt-[0.62em] h-px w-4 shrink-0 bg-accent"></span>
                  <span>{point}</span>
                </li>
              {/each}
            </ul>
          </div>
          <div class="md:col-span-6 lg:col-span-7">
            <PanelMock id={panel.id} {t} />
          </div>
        </div>
      {/key}
    </div>

    <div class="mt-20" use:reveal>
      <p class="mark">{t.workspace.alsoLabel}</p>
      <div class="mt-6 grid gap-px overflow-hidden rounded-md border border-line-soft bg-line-soft sm:grid-cols-2 lg:grid-cols-4">
        {#each t.workspace.also as card}
          <div class="bg-paper p-6">
            <p class="font-mono text-[0.68rem] uppercase tracking-[0.14em] text-accent">{card.tag}</p>
            <h3 class="mt-3 text-[1.05rem]">{card.title}</h3>
            <p class="mt-2.5 text-[0.9rem] leading-[1.7] text-muted">{card.body}</p>
          </div>
        {/each}
      </div>
    </div>
  </div>
</section>

<style>
  .fade { animation: fade 420ms cubic-bezier(0.2, 0.7, 0.2, 1) backwards; }
  @keyframes fade {
    from { opacity: 0; transform: translateY(8px); }
    to { opacity: 1; transform: none; }
  }
  @media (prefers-reduced-motion: reduce) {
    .fade { animation: none; }
  }
</style>
