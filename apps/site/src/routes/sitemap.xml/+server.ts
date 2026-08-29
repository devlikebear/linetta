import { canonicalUrl, translations } from '$lib/content';

// Prerendered alongside the pages, so the deployment stays static files only.
export const prerender = true;

const XML_ESCAPES: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&apos;'
};

const escape = (value: string) => value.replace(/[&<>"']/g, (c) => XML_ESCAPES[c]);

/**
 * One <url> per locale, each carrying the full set of alternates — the shape
 * Google asks for on a multilingual site. Both the list and the URLs come from
 * the content catalogue, so a fourth locale lands here without an edit.
 */
export function GET() {
  const locales = Object.values(translations);

  const alternates = locales
    .map(
      (locale) =>
        `    <xhtml:link rel="alternate" hreflang="${locale.htmlLang}" href="${escape(canonicalUrl(locale.path))}" />`
    )
    .concat(
      `    <xhtml:link rel="alternate" hreflang="x-default" href="${escape(canonicalUrl('/'))}" />`
    )
    .join('\n');

  const urls = locales
    .map(
      (locale) =>
        `  <url>\n    <loc>${escape(canonicalUrl(locale.path))}</loc>\n${alternates}\n  </url>`
    )
    .join('\n');

  const body = `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">
${urls}
</urlset>
`;

  return new Response(body, {
    headers: { 'Content-Type': 'application/xml; charset=utf-8' }
  });
}
