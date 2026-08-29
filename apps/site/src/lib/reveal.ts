import type { Action } from 'svelte/action';

/**
 * Fades an element up the first time it enters the viewport. The `reveal`
 * class is added here rather than in markup so that a reader without
 * JavaScript — or a prerendered crawl — sees the content at full opacity.
 */
export const reveal: Action<HTMLElement, number | undefined> = (node, delay = 0) => {
  node.classList.add('reveal');
  node.style.transitionDelay = `${delay}ms`;

  if (typeof IntersectionObserver === 'undefined') {
    node.dataset.shown = 'true';
    return;
  }

  const observer = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (!entry.isIntersecting) continue;
        node.dataset.shown = 'true';
        observer.unobserve(node);
      }
    },
    { rootMargin: '0px 0px -10% 0px', threshold: 0.05 }
  );

  observer.observe(node);
  return { destroy: () => observer.disconnect() };
};
