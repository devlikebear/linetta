<script lang="ts">
  import { reveal } from '$lib/reveal';
  import type { FeatureSection } from '$lib/content';

  let {
    id,
    s,
    layout = 'split',
    flip = false
  }: { id: string; s: FeatureSection; layout?: 'split' | 'wide'; flip?: boolean } = $props();
</script>

<section {id} class="border-b border-line">
  {#if layout === 'wide'}
    <div class="wrap py-20 md:py-28">
      <div class="max-w-2xl" use:reveal>
        <p class="mark">{s.mark}</p>
        <h2 class="mt-4 text-[clamp(1.9rem,3.6vw,2.7rem)]">{s.heading}</h2>
        <p class="mt-5 text-ink-soft">{s.sub}</p>
      </div>
      <div
        class="mt-12 grid gap-px overflow-hidden rounded-md border border-line-soft bg-line-soft sm:grid-cols-2 lg:grid-cols-3"
      >
        {@render cards()}
      </div>
    </div>
  {:else}
    <div class="wrap grid gap-12 py-20 md:grid-cols-12 md:py-28">
      <div class="md:col-span-5 {flip ? 'md:order-last' : ''}" use:reveal>
        <p class="mark">{s.mark}</p>
        <h2 class="mt-4 text-[clamp(1.9rem,3.6vw,2.7rem)]">{s.heading}</h2>
        <p class="mt-5 max-w-[30rem] text-ink-soft">{s.sub}</p>
        {#if s.link}
          <a href={s.link.href} class="btn btn-ghost mt-7" rel="noopener">{s.link.label}</a>
        {/if}
      </div>
      <div class="md:col-span-7">
        <div class="grid gap-px overflow-hidden rounded-md border border-line-soft bg-line-soft sm:grid-cols-2">
          {@render cards()}
        </div>
      </div>
    </div>
  {/if}
</section>

{#snippet cards()}
  {#each s.cards as card, i}
    <div class="bg-paper p-7" use:reveal={i * 60}>
      <p class="font-mono text-[0.68rem] uppercase tracking-[0.14em] text-accent">{card.tag}</p>
      <h3 class="mt-3 text-[1.1rem]">{card.title}</h3>
      <p class="mt-2.5 text-[0.9rem] leading-[1.75] text-muted">{card.body}</p>
    </div>
  {/each}
{/snippet}
