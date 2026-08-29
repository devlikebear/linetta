<script lang="ts">
  import Wordmark from '$lib/Wordmark.svelte';
  import type { Translation } from '$lib/content';

  let { t }: { t: Translation } = $props();

  const items = $derived([
    { label: t.nav.workspace, href: '#workspace' },
    { label: t.nav.agent, href: '#agents' },
    { label: t.nav.data, href: '#data' },
    { label: t.nav.download, href: '#download' }
  ]);
</script>

<header
  class="sticky top-0 z-50 border-b border-line bg-paper/85 backdrop-blur-md supports-[backdrop-filter]:bg-paper/70"
>
  <div class="wrap flex h-16 items-center justify-between gap-4">
    <a href={t.path} class="flex items-center gap-2.5 text-ink" aria-label="Linetta">
      <Wordmark />
      <span class="text-lg tracking-tight">Linetta</span>
    </a>

    <nav class="hidden items-center gap-7 md:flex" aria-label={t.nav.workspace}>
      {#each items as item}
        <a
          href={item.href}
          class="font-mono text-[0.7rem] uppercase tracking-[0.14em] text-muted transition-colors hover:text-ink"
          >{item.label}</a
        >
      {/each}
    </nav>

    <div class="flex items-center gap-3">
      <div class="hidden items-center gap-2 font-mono text-[0.7rem] text-muted-2 sm:flex">
        {#each t.alts as alt}
          <a href={alt.path} hreflang={alt.locale} class="transition-colors hover:text-ink"
            >{alt.label}</a
          >
        {/each}
      </div>
      <a href="#download" class="btn btn-primary">{t.nav.cta}</a>
    </div>
  </div>
</header>
