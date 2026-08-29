<script lang="ts">
  import AppFrame from '$lib/AppFrame.svelte';
  import type { Translation } from '$lib/content';

  let { t }: { t: Translation } = $props();
</script>

<section class="relative overflow-hidden border-b border-line">
  <!-- Ruled paper: hairlines the hero sits on, fading out at both ends. -->
  <div
    class="pointer-events-none absolute inset-0 -z-10"
    style="background-image: repeating-linear-gradient(to bottom, color-mix(in srgb, var(--color-line) 70%, transparent) 0 1px, transparent 1px 2.25rem);
           mask-image: linear-gradient(to bottom, transparent, black 22%, black 62%, transparent 92%);
           -webkit-mask-image: linear-gradient(to bottom, transparent, black 22%, black 62%, transparent 92%);"
    aria-hidden="true"
  ></div>

  <div class="wrap lift py-20 md:py-28">
    <p class="mark" style="--i: 0">{t.hero.mark}</p>

    <!-- The headline runs the full measure rather than sitting in the left
         column: a CJK line at this size does not fit a half-width column. -->
    <h1 class="mt-6 text-[clamp(2.1rem,4.4vw,3.6rem)] leading-[1.12]" style="--i: 1">
      {#each t.hero.headline as run}{#if run.nl}<br />{/if}{#if run.text}<span
            class={run.accent ? 'em-accent' : ''}>{run.text}</span
          >{/if}{/each}
    </h1>

    <div
      class="mt-12 grid gap-12 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.12fr)] lg:items-start lg:gap-16"
      style="--i: 2"
    >
      <div>
        <p class="max-w-[34rem] text-[1.075rem] leading-[1.75] text-ink-soft">{t.hero.sub}</p>

        <div class="mt-9 flex flex-wrap gap-3">
          <a href="#download" class="btn btn-primary">{t.hero.ctaPrimary}</a>
          <a
            href="https://github.com/devlikebear/linetta"
            target="_blank"
            rel="noopener"
            class="btn btn-ghost"
          >
            <svg width="15" height="15" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"
              ><path
                d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.012 8.012 0 0016 8c0-4.42-3.58-8-8-8z"
              /></svg
            >
            {t.hero.ctaSecondary}
          </a>
        </div>

        <ul class="mt-10 flex flex-wrap gap-x-7 gap-y-2.5 font-mono text-[0.7rem] text-muted-2">
          {#each t.hero.pills as pill}
            <li class="flex items-center gap-2">
              <span class="inline-block h-1 w-1 rounded-full bg-accent"></span>{pill}
            </li>
          {/each}
        </ul>
      </div>

      <div>
        <AppFrame {t} />
      </div>
    </div>
  </div>
</section>

<style>
  /* One orchestrated entrance for the fold, staggered by --i. */
  .lift > * {
    animation: lift 850ms cubic-bezier(0.16, 1, 0.3, 1) backwards;
    animation-delay: calc(var(--i, 0) * 90ms + 60ms);
  }
  @keyframes lift {
    from { opacity: 0; transform: translateY(18px); }
    to { opacity: 1; transform: none; }
  }

  @media (prefers-reduced-motion: reduce) {
    .lift > * { animation: none; }
  }
</style>
