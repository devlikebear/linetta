<script lang="ts">
  import { reveal } from '$lib/reveal';
  import type { Translation } from '$lib/content';

  let { t }: { t: Translation } = $props();
</script>

<section class="border-b border-line">
  <div class="wrap grid gap-12 py-20 md:grid-cols-12 md:py-28">
    <div class="md:col-span-4" use:reveal>
      <p class="mark">{t.faq.mark}</p>
      <h2 class="mt-4 text-[clamp(1.9rem,3.6vw,2.7rem)]">{t.faq.heading}</h2>
    </div>

    <dl class="md:col-span-8">
      {#each t.faq.items as item, i}
        <div class="border-t border-line-soft py-6 first:border-t-0 first:pt-0" use:reveal={i * 50}>
          <dt class="flex gap-4 text-[1.05rem]">
            <span class="font-mono text-[0.7rem] text-muted-2 pt-[0.45em]"
              >{String(i + 1).padStart(2, '0')}</span
            >
            <span>{item.q}</span>
          </dt>
          <dd class="mt-3 max-w-[38rem] pl-[2.1rem] text-[0.95rem] leading-[1.8] text-muted">
            {item.a}
          </dd>
        </div>
      {/each}
    </dl>
  </div>
</section>
