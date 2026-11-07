import type { Locale } from './content';

/**
 * The key src/app.html reads before the page draws. A writer who picks a
 * language with the switcher keeps it: the root no longer follows the browser.
 */
export const LOCALE_KEY = 'linetta-site-locale';

export function rememberLocale(locale: Locale): void {
  try {
    localStorage.setItem(LOCALE_KEY, locale);
  } catch {
    // Storage can be off (private windows, blocked site data). The switch
    // still works; only the next visit to the root may follow the browser.
  }
}
