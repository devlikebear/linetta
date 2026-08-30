<script lang="ts">
  let { size = 22 }: { size?: number } = $props();
</script>

<!-- The shipped app icon, redrawn: black L, sienna dot, paper tile. -->
<svg width={size} height={size} viewBox="0 0 64 64" aria-hidden="true" class="shrink-0">
  <rect x="4" y="4" width="56" height="56" rx="12" fill="var(--color-surface)" stroke="var(--color-line)" stroke-width="1.5" />
  <path d="M19.4 13.8h9.1v34.3h-9.1z" fill="currentColor" />
  <path d="M19.4 40.6h27.1v7.5H19.4z" fill="currentColor" />
  <rect x="38.8" y="14.5" width="9.5" height="9.5" rx="3" fill="var(--color-sienna)" />
</svg>
