<script lang="ts">
  import { reveal } from '$lib/reveal';
  import type { Translation } from '$lib/content';

  let { t }: { t: Translation } = $props();
</script>

<section class="border-b border-line">
  <div class="wrap grid gap-10 py-20 md:grid-cols-12 md:py-28">
    <div class="md:col-span-5" use:reveal>
      <p class="mark">{t.intro.mark}</p>
      <h2 class="mt-4 text-[clamp(1.9rem,3.6vw,2.7rem)] whitespace-pre-line">{t.intro.heading}</h2>
    </div>

    <div class="md:col-span-7 md:pt-2">
      {#each t.intro.paragraphs as paragraph, i}
        <p
          class="mb-6 max-w-[38rem] text-[1.05rem] leading-[1.8] text-ink-soft last:mb-0"
          use:reveal={i * 80}
        >
          {@html paragraph}
        </p>
      {/each}
    </div>
  </div>
</section>
