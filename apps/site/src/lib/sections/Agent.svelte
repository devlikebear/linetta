<script lang="ts">
  import { reveal } from '$lib/reveal';
  import type { Translation } from '$lib/content';

  let { t }: { t: Translation } = $props();
  const a = $derived(t.agent);
</script>

<section id="agents" class="relative border-b border-line">
  <div
    class="pointer-events-none absolute inset-0 -z-10"
    style="background:
      radial-gradient(70% 55% at 12% 0%, color-mix(in srgb, var(--color-accent) 9%, transparent), transparent 70%),
      radial-gradient(60% 50% at 95% 100%, color-mix(in srgb, var(--color-sienna) 7%, transparent), transparent 70%);"
    aria-hidden="true"
  ></div>

  <div class="wrap py-20 md:py-28">
    <div class="max-w-3xl" use:reveal>
      <p class="mark">{a.mark}</p>
      <h2 class="mt-4 whitespace-pre-line text-[clamp(1.9rem,3.8vw,2.8rem)]">{a.heading}</h2>
      <p class="mt-6 max-w-[38rem] text-[1.05rem] leading-[1.8] text-ink-soft">{a.lead}</p>
    </div>

    <div class="mt-14 grid gap-12 md:grid-cols-12">
      <div class="md:col-span-6" use:reveal>
        {#each a.paragraphs as paragraph}
          <p class="mb-5 max-w-[34rem] leading-[1.8] text-muted last:mb-0">{@html paragraph}</p>
        {/each}
      </div>

      <div class="md:col-span-6" use:reveal={80}>
        <p class="mark">{a.commandLabel}</p>
        <pre class="term mt-3">{a.command}</pre>
        <p class="mt-3 text-[0.85rem] leading-[1.7] text-muted-2">{@html a.commandNote}</p>
      </div>
    </div>

    <!-- Access modes -->
    <div class="mt-16" use:reveal>
      <p class="mark">{a.modesLabel}</p>
      <div class="mt-5 grid gap-4 sm:grid-cols-3">
        {#each a.modes as mode, i}
          <div class="panel p-5">
            <div class="flex items-center gap-2.5">
              <span
                class="inline-block h-2 w-2 rounded-full"
                style="background: {['var(--color-muted-2)', 'var(--color-sienna)', 'var(--color-accent)'][i]}"
              ></span>
              <h3 class="text-[1.05rem]">{mode.name}</h3>
            </div>
            <p class="mt-2.5 text-[0.9rem] leading-[1.7] text-muted">{mode.body}</p>
          </div>
        {/each}
      </div>
    </div>

    <!-- Tool catalogue -->
    <div class="mt-16" use:reveal>
      <div class="max-w-2xl">
        <h3 class="text-[clamp(1.25rem,2.2vw,1.6rem)]">{a.toolsLabel}</h3>
        <p class="mt-3 text-[0.95rem] leading-[1.75] text-muted">{a.toolsNote}</p>
      </div>

      <div class="mt-7 grid gap-x-12 gap-y-8 md:grid-cols-2">
        {#each [{ label: a.readLabel, tools: a.readTools }, { label: a.writeLabel, tools: a.writeTools }] as group}
          <div>
            <div class="flex items-baseline justify-between border-b border-line pb-2">
              <span class="mark">{group.label}</span>
              <span class="font-mono text-[0.68rem] text-muted-2">{group.tools.length}</span>
            </div>
            <ul>
              {#each group.tools as tool}
                <li class="grid gap-1 border-b border-line-soft py-3 sm:grid-cols-[13.5rem_1fr] sm:gap-4">
                  <code class="font-mono text-[0.76rem] text-accent">{tool.name}</code>
                  <span class="text-[0.85rem] leading-[1.6] text-muted">{tool.note}</span>
                </li>
              {/each}
            </ul>
          </div>
        {/each}
      </div>
    </div>

    <!-- Safety -->
    <div class="mt-16 panel p-7 md:p-9" use:reveal>
      <p class="mark">{a.safetyLabel}</p>
      <ul class="mt-5 grid gap-x-12 gap-y-4 md:grid-cols-2">
        {#each a.safety as item, i}
          <li class="flex gap-4 text-[0.92rem] leading-[1.7] text-muted">
            <span class="font-mono text-[0.7rem] text-muted-2 pt-[0.28em]"
              >{String(i + 1).padStart(2, '0')}</span
            >
            <span>{item}</span>
          </li>
        {/each}
      </ul>
    </div>
  </div>
</section>
