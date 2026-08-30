<script lang="ts">
  import Wordmark from '$lib/Wordmark.svelte';
  import type { Translation } from '$lib/content';

  let { t }: { t: Translation } = $props();
  const year = new Date().getFullYear();

  const cols = $derived([
    { title: t.footer.cols.project, links: t.footer.links.project },
    { title: t.footer.cols.docs, links: t.footer.links.docs },
    { title: t.footer.cols.get, links: t.footer.links.get }
  ]);
</script>

<footer>
  <div class="wrap grid gap-10 py-14 md:grid-cols-12">
    <div class="md:col-span-3">
      <div class="flex items-center gap-2.5 text-ink">
        <Wordmark />
        <span class="text-lg tracking-tight">Linetta</span>
      </div>
      <p class="mt-4 max-w-[24rem] text-[0.9rem] leading-[1.75] text-muted">{t.footer.tagline}</p>
      <div class="mt-5 flex gap-3 font-mono text-[0.7rem] text-muted-2">
        {#each t.alts as alt}
          <a href={alt.path} hreflang={alt.locale} class="transition-colors hover:text-ink"
            >{alt.label}</a
          >
        {/each}
      </div>
    </div>

    {#each cols as col}
      <div class="md:col-span-3">
        <p class="mark">{col.title}</p>
        <ul class="mt-4 space-y-2.5">
          {#each col.links as link}
            <li>
              <a
                href={link.href}
                target="_blank"
                rel="noopener"
                class="text-[0.9rem] text-muted transition-colors hover:text-ink">{link.label}</a
              >
            </li>
          {/each}
        </ul>
      </div>
    {/each}
  </div>

  <div class="wrap">
    <hr class="rule" />
    <div class="flex flex-wrap items-center justify-between gap-3 py-6 font-mono text-[0.68rem] text-muted-2">
      <p>© {year} Linetta · {t.footer.legal}</p>
      <p>linetta.marvin-42.com</p>
    </div>
  </div>
</footer>
