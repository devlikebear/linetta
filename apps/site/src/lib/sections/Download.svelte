<script lang="ts">
  import { reveal } from '$lib/reveal';
  import type { Translation } from '$lib/content';

  let { t }: { t: Translation } = $props();
  const d = $derived(t.download);
</script>

<section id="download" class="border-b border-line">
  <div class="wrap py-20 md:py-28">
    <div class="flex flex-wrap items-end justify-between gap-6" use:reveal>
      <div class="max-w-2xl">
        <p class="mark">{d.mark}</p>
        <h2 class="mt-4 whitespace-pre-line text-[clamp(1.9rem,3.6vw,2.7rem)]">{d.heading}</h2>
        <p class="mt-5 max-w-[34rem] text-ink-soft">{d.sub}</p>
      </div>
      <p class="font-mono text-[0.7rem] uppercase tracking-[0.14em] text-muted-2">
        {d.versionLabel} <span class="text-accent">{d.version}</span>
      </p>
    </div>

    <div class="mt-12 grid gap-4 sm:grid-cols-2">
      {#each d.channels as channel, i}
        <div class="panel flex flex-col gap-4 p-6" use:reveal={i * 60}>
          <div>
            <h3 class="text-[1.15rem]">{channel.label}</h3>
            <p class="mt-2 text-[0.9rem] leading-[1.7] text-muted">{channel.note}</p>
          </div>

          <div class="mt-auto">
            {#if channel.code}
              <pre class="term">{channel.code}</pre>
            {:else if channel.cta}
              <a href={channel.cta.href} target="_blank" rel="noopener" class="btn btn-ghost">
                {channel.cta.label}
                <span aria-hidden="true">→</span>
              </a>
            {/if}
          </div>
        </div>
      {/each}
    </div>

    <p class="mt-8 max-w-[42rem] text-[0.9rem] leading-[1.75] text-muted-2" use:reveal>
      {d.sourceNote}
    </p>
  </div>
</section>
