<script lang="ts">
  import { SHOT_HEIGHT, SHOT_WIDTH } from '$lib/screenshots';

  let {
    src,
    alt,
    priority = false
  }: { src: string; alt: string; priority?: boolean } = $props();
</script>

<!-- The captures carry the OS window chrome already, so the frame here is only
     a hairline and a shadow to lift them off the paper. -->
<img
  {src}
  {alt}
  width={SHOT_WIDTH}
  height={SHOT_HEIGHT}
  loading={priority ? 'eager' : 'lazy'}
  decoding={priority ? 'sync' : 'async'}
  fetchpriority={priority ? 'high' : 'auto'}
  class="shot"
/>

<style>
  .shot {
    display: block;
    width: 100%;
    height: auto;
    border: 1px solid var(--color-line);
    border-radius: 8px;
    background: var(--color-surface);
    box-shadow: 0 24px 60px -32px rgb(var(--shade) / 0.5);
  }

  /* The app's own palette is light. Against the dark ground the capture reads
     as a lamp, so take a little brightness off it — enough to seat it on the
     page, not enough to change what the screen shows. */
  @media (prefers-color-scheme: dark) {
    .shot {
      filter: brightness(0.88);
    }
  }

  /* On a phone a 1442px window is small enough already; give it the gutter
     back rather than letting the frame eat into it. */
  @media (max-width: 40rem) {
    .shot {
      width: calc(100% + 3rem);
      margin-inline: -1.5rem;
      border-inline: 0;
      border-radius: 0;
    }
  }
</style>
