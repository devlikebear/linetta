import type { Handle } from '@sveltejs/kit';

/** The document language has to be right per route, and `svelte:head` cannot
 *  reach the <html> element — so it is stamped in here at prerender time. */
export const handle: Handle = ({ event, resolve }) => {
  const path = event.url.pathname;
  const lang = path.startsWith('/ko') ? 'ko' : path.startsWith('/ja') ? 'ja' : 'en';

  return resolve(event, {
    transformPageChunk: ({ html }) => html.replace('%lang%', lang)
  });
};
