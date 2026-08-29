<script lang="ts">
  import { reveal } from '$lib/reveal';
  import type { Translation } from '$lib/content';

  let { t }: { t: Translation } = $props();
  const d = $derived(t.data);
</script>

<section id="data" class="border-b border-line">
  <div class="wrap grid gap-12 py-20 md:grid-cols-12 md:py-28">
    <div class="md:col-span-5" use:reveal>
      <p class="mark">{d.mark}</p>
      <h2 class="mt-4 text-[clamp(1.9rem,3.6vw,2.7rem)]">{d.heading}</h2>
      <p class="mt-5 max-w-[30rem] text-ink-soft">{d.sub}</p>

      <div class="mt-9">
        <p class="mark">{d.pathsLabel}</p>
        <dl class="mt-4">
          {#each d.paths as row}
            <div class="grid grid-cols-[4.5rem_1fr] gap-3 border-t border-line-soft py-2.5">
              <dt class="font-mono text-[0.72rem] uppercase tracking-[0.1em] text-muted-2">
                {row.os}
              </dt>
              <dd class="overflow-x-auto font-mono text-[0.72rem] leading-[1.7] text-muted">
                {row.path}
              </dd>
            </div>
          {/each}
        </dl>
      </div>
    </div>

    <div class="md:col-span-7">
      <div class="grid gap-px overflow-hidden rounded-md border border-line-soft bg-line-soft sm:grid-cols-2">
        {#each d.cards as card, i}
          <div class="bg-paper p-7" use:reveal={i * 60}>
            <code class="font-mono text-[0.7rem] tracking-[0.06em] text-sienna">{card.tag}</code>
            <h3 class="mt-3 text-[1.1rem]">{card.title}</h3>
            <p class="mt-2.5 text-[0.9rem] leading-[1.75] text-muted">{card.body}</p>
          </div>
        {/each}
      </div>
    </div>
  </div>
</section>
