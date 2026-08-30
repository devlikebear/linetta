import factBook from '../../../../docs/assets/screenshots/fact-book.png';
import library from '../../../../docs/assets/screenshots/library.png';
import storyWorld from '../../../../docs/assets/screenshots/story-world.png';
import workspace from '../../../../docs/assets/screenshots/workspace.png';

import type { PanelId } from './content';

/**
 * Captures are read from docs/assets/screenshots rather than copied into
 * static/, so retaking them for the README updates the site in the same commit
 * and the two can never show different builds.
 */
export const screenshots: Record<PanelId, string> = {
  workspace,
  'story-world': storyWorld,
  'fact-book': factBook,
  library
};

/** Every capture is the same window size. Declaring it reserves the space so
 *  nothing reflows while an image loads. */
export const SHOT_WIDTH = 1442;
export const SHOT_HEIGHT = 909;
