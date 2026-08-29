<script lang="ts">
  import Header from '$lib/sections/Header.svelte';
  import Hero from '$lib/sections/Hero.svelte';
  import Intro from '$lib/sections/Intro.svelte';
  import Workspace from '$lib/sections/Workspace.svelte';
  import Agent from '$lib/sections/Agent.svelte';
  import DataSection from '$lib/sections/DataSection.svelte';
  import Download from '$lib/sections/Download.svelte';
  import Faq from '$lib/sections/Faq.svelte';
  import Footer from '$lib/sections/Footer.svelte';
  import type { Translation } from '$lib/content';

  let { t }: { t: Translation } = $props();

  const SITE = 'https://linetta.marvin-42.com';
  const canonical = $derived(t.path === '/' ? `${SITE}/` : `${SITE}${t.path}`);

  const jsonLd = $derived(
    JSON.stringify({
      '@context': 'https://schema.org',
      '@type': 'SoftwareApplication',
      name: 'Linetta',
      applicationCategory: 'WritingApplication',
      operatingSystem: 'macOS, Windows, Linux',
      url: canonical,
      description: t.metaDescription,
      inLanguage: t.htmlLang,
      license: 'https://www.gnu.org/licenses/agpl-3.0.html',
      offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },
      author: { '@type': 'Person', name: 'devlikebear' }
    })
  );
</script>

<svelte:head>
  <title>{t.pageTitle}</title>
  <meta name="description" content={t.metaDescription} />
  <link rel="canonical" href={canonical} />

  <meta property="og:title" content={t.pageTitle} />
  <meta property="og:description" content={t.metaDescription} />
  <meta property="og:url" content={canonical} />
  <meta property="og:locale" content={t.ogLocale} />
  <meta name="twitter:title" content={t.pageTitle} />
  <meta name="twitter:description" content={t.metaDescription} />

  <link rel="alternate" hreflang="en" href="{SITE}/" />
  <link rel="alternate" hreflang="ko" href="{SITE}/ko" />
  <link rel="alternate" hreflang="ja" href="{SITE}/ja" />
  <link rel="alternate" hreflang="x-default" href="{SITE}/" />

  {@html `<script type="application/ld+json">${jsonLd}</script>`}
</svelte:head>

<a
  href="#main"
  class="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-[60] focus:rounded focus:bg-surface focus:px-4 focus:py-2"
>
  {t.nav.skip}
</a>

<Header {t} />
<main id="main">
  <Hero {t} />
  <Intro {t} />
  <Workspace {t} />
  <Agent {t} />
  <DataSection {t} />
  <Download {t} />
  <Faq {t} />
</main>
<Footer {t} />
