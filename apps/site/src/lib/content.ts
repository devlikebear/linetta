import { version as VERSION } from '../../../desktop/package.json';

export type Locale = 'en' | 'ko' | 'ja';

export type AltLink = { locale: Locale; path: string; label: string };

/** A headline is a list of runs so a locale can put the accent and the line
 *  break where its own grammar wants them. */
export type HeadlineRun = { text?: string; accent?: boolean; nl?: boolean };

/** One per screenshot in docs/assets/screenshots. */
export type PanelId = 'workspace' | 'story-world' | 'fact-book' | 'library';

export type Panel = {
  id: PanelId;
  tab: string;
  kicker: string;
  title: string;
  body: string;
  alt: string;
  points: string[];
};

export type Card = { tag: string; title: string; body: string };
export type Tool = { name: string; note: string };
export type Mode = { name: string; body: string };
export type Channel = {
  id: 'mas' | 'brew' | 'windows' | 'linux';
  label: string;
  note: string;
  code?: string;
  cta?: { label: string; href: string };
};
export type Faq = { q: string; a: string };
export type Link = { label: string; href: string };

/** A section that is a heading and a set of cards. */
export type FeatureSection = {
  mark: string;
  heading: string;
  sub: string;
  link?: Link;
  cards: Card[];
};

export type Translation = {
  locale: Locale;
  htmlLang: string;
  ogLocale: string;
  path: string;
  alts: AltLink[];
  pageTitle: string;
  metaDescription: string;

  nav: {
    workspace: string;
    longform: string;
    assist: string;
    illustrations: string;
    publish: string;
    agent: string;
    data: string;
    download: string;
    github: string;
    cta: string;
    skip: string;
  };

  hero: {
    mark: string;
    headline: HeadlineRun[];
    sub: string;
    ctaPrimary: string;
    ctaSecondary: string;
    pills: string[];
    imageAlt: string;
  };

  intro: { mark: string; heading: string; paragraphs: string[] };

  workspace: {
    mark: string;
    heading: string;
    sub: string;
    panels: Panel[];
    alsoLabel: string;
    also: Card[];
  };

  assist: FeatureSection;
  longform: FeatureSection;
  illustrations: FeatureSection;
  publish: FeatureSection;

  agent: {
    mark: string;
    heading: string;
    lead: string;
    paragraphs: string[];
    commandLabel: string;
    command: string;
    commandNote: string;
    modesLabel: string;
    modes: Mode[];
    toolsLabel: string;
    toolsNote: string;
    readLabel: string;
    writeLabel: string;
    readTools: Tool[];
    writeTools: Tool[];
    safetyLabel: string;
    safety: string[];
    byokLabel: string;
    byokLead: string;
    byokProviders: Tool[];
    byokPoints: string[];
  };

  data: {
    mark: string;
    heading: string;
    sub: string;
    cards: Card[];
    pathsLabel: string;
    paths: { os: string; path: string }[];
  };

  download: {
    mark: string;
    heading: string;
    sub: string;
    versionLabel: string;
    version: string;
    channels: Channel[];
    sourceNote: string;
  };

  faq: { mark: string; heading: string; items: Faq[] };

  footer: {
    tagline: string;
    cols: { project: string; docs: string; get: string };
    links: { project: Link[]; docs: Link[]; get: Link[] };
    legal: string;
  };
};

/** The deployed origin. Canonical tags, hreflang links and the sitemap all
 *  derive from this, so a domain change is a one-line change. */
export const SITE = 'https://linetta.marvin-42.com';

/** Absolute URL for a locale's path. The root keeps its trailing slash so the
 *  canonical tag and the sitemap agree with what the host serves. */
export const canonicalUrl = (path: string) => (path === '/' ? `${SITE}/` : `${SITE}${path}`);

const REPO = 'https://github.com/devlikebear/linetta';
const MAS = 'https://apps.apple.com/app/id6781664781';
const WORKS = 'https://works.marvin-42.com';
const RELEASES = `${REPO}/releases/latest`;

const MCP_COMMAND =
  'claude mcp add --transport http linetta http://127.0.0.1:7391/mcp \\\n  --header "Authorization: Bearer <token>"';

const alts = (self: Locale): AltLink[] =>
  (
    [
      { locale: 'en' as const, path: '/', label: 'EN' },
      { locale: 'ko' as const, path: '/ko', label: '한국어' },
      { locale: 'ja' as const, path: '/ja', label: '日本語' }
    ]
  ).filter((l) => l.locale !== self);

export const en: Translation = {
  locale: 'en',
  htmlLang: 'en',
  ogLocale: 'en_US',
  path: '/',
  alts: alts('en'),
  pageTitle: 'Linetta — write long-form fiction and publish it on Linetta Works',
  metaDescription:
    'Plan a novel, keep it consistent, and write it scene by scene in one quiet workspace. Outline, characters, plot threads, research and a ledger of what the story owes sit beside the manuscript. When you want more, a built-in agent or your own over MCP checks continuity across a long serial, a scene becomes an illustration, an episode goes out to readers on Linetta Works, and a work follows you between Mac, PC, iPhone and iPad.',

  nav: {
    workspace: 'Workspace',
    longform: 'Long form',
    assist: 'Assist',
    illustrations: 'Illustrations',
    publish: 'Publish',
    agent: 'Agents',
    data: 'Your data',
    download: 'Download',
    github: 'GitHub',
    cta: 'Get Linetta',
    skip: 'Skip to content'
  },

  hero: {
    mark: 'macOS · iPhone · iPad · Windows · Linux — with Linetta Works',
    headline: [
      { text: 'The whole story, ' },
      { text: 'within reach', accent: true },
      { nl: true },
      { text: 'of the sentence you are writing.' }
    ],
    sub: 'Linetta is a calm writing studio for novels and web fiction. Outline, characters, places, relationships, plot threads, research and a ledger of what the story owes live beside the manuscript. When you want more, an agent — built in, or your own over MCP — keeps a long serial consistent, a scene becomes an illustration, an episode reaches readers on Linetta Works, and the work follows you from the desk to an iPhone. All of it is optional.',
    ctaPrimary: 'Get Linetta',
    ctaSecondary: 'Source on GitHub',
    pills: ['Free, no subscription', 'Built-in agent, or your own', 'Korean, English, Japanese'],
    imageAlt:
      'The Linetta workspace: an outline rail, a serif editor holding the opening of a scene, and a right-hand panel with the scene’s character count, targets and synopsis.'
  },

  intro: {
    mark: '§ 01 — What it is',
    heading: 'Writing is not\nproject management.',
    paragraphs: [
      'A long story accumulates state. By episode forty, a promise made in chapter three, a debt, and the colour of somebody’s coat are all load-bearing — and none of it is in the paragraph on screen.',
      'Most tools answer that with more ceremony: boards, tags, statuses, a second app to keep current. Linetta answers it by putting the story’s memory <em>beside the sentence</em> — one keystroke away, never in the way.',
      'So the editor stays a quiet serif page. Everything else — outline, characters, plot spine, research, history — is a rail or a panel you open when the sentence needs it, and close when it does not.'
    ]
  },

  workspace: {
    mark: '§ 02 — The workspace',
    heading: 'One room for the whole work.',
    sub: 'Every panel is built around the same question: what does this scene need to be consistent with?',
    panels: [
      {
        id: 'workspace',
        tab: 'Workspace',
        kicker: 'The page',
        title: 'A serif page, the outline beside it, and the scene’s own record.',
        body: 'Write scene by scene with the outline within reach and the count where you can ignore it. The right rail holds what you check without leaving the sentence — characters written today, the episode target, the synopsis, the entities this scene mentions.',
        alt: 'The workspace: an outline rail on the left, a serif editor in the middle, and a right panel showing 442 characters, an episode target and a project overview.',
        points: [
          'Parts and chapters, or arcs and episodes — Linetta numbers the tree in the language of the interface',
          'Per-scene length targets with a live count, beside the work total',
          'ZEN empties the room down to the text and a counter'
        ]
      },
      {
        id: 'story-world',
        tab: 'Story World',
        kicker: 'The invented world',
        title: 'Characters, places, items and concepts, with the ties between them.',
        body: 'Fact Book is for the real world; Story World is for the invented one. Each card carries a role, a summary and the relationships it stands in, and the filters narrow the shelf to one kind.',
        alt: 'The Story World panel listing entity cards — a place, two characters, a concept and an item — each with a role, a one-line summary and a relationship count.',
        points: [
          'Roles like protagonist, main stage or villain, and the relationships that connect them',
          'Search by name, alias, role or summary',
          'An agent can add one directly when you ask; anything drawn from the manuscript waits in the suggestions inbox'
        ]
      },
      {
        id: 'fact-book',
        tab: 'Fact Book',
        kicker: 'Research',
        title: 'Source-backed notes beside the scene that needs them.',
        body: 'Write down what you checked and where; Linetta fetches the page and files it alongside. A card is marked verified or uncertain, so a claim you have not settled stays visibly unsettled instead of hardening into fact.',
        alt: 'The Fact Book panel with research cards, each tagged verified or uncertain and carrying the source it came from.',
        points: [
          'Verified and uncertain badges, with the source kept on the card',
          'Scoped to the current scene or to the whole work',
          'An agent connected over MCP reads these cards too'
        ]
      },
      {
        id: 'library',
        tab: 'Library',
        kicker: 'The shelf',
        title: 'From one line to a full book.',
        body: 'Works sit on a shelf built for more than one of them, with word counts and status. Start a project, or bring an existing manuscript in from Markdown.',
        alt: 'The Linetta library screen with a project card and buttons for a new project, Markdown import, search and browsing Linetta Works.',
        points: [
          'New project, Markdown import, and search across everything',
          'Active and archived shelves',
          'Linetta Works in reach — publish episodes and browse what readers read'
        ]
      }
    ],
    alsoLabel: 'Also in the room',
    also: [
      {
        tag: 'Contextual Edit',
        title: 'Fix a fact everywhere',
        body: 'Change a character, place or relationship once, and Linetta finds the scenes that still carry the old version and walks you through them. No language model involved.'
      },
      {
        tag: 'Plot',
        title: 'Threads on a spine',
        body: 'Storylines and beats placed across the outline, so a promise you opened in episode 3 is something you can see rather than something you remember late.'
      },
      {
        tag: 'History',
        title: 'Snapshots per scene',
        body: 'Manual and automatic snapshots, restorable. Manual ones, and the ones taken before an agent writes, are kept indefinitely.'
      },
      {
        tag: 'Record',
        title: 'Writing pace',
        body: 'A seven-day average and the number of episodes per week it implies. No streaks, no guilt.'
      }
    ]
  },

  assist: {
    mark: '§ 03 — Assist',
    heading: 'A second reader at the desk.',
    sub: 'Everything here is optional and needs an agent you connect — § 07 says how. It reads, checks and proposes; the manuscript changes only when you ask.',
    cards: [
      {
        tag: 'Ask the agent',
        title: 'Ask about the passage you selected',
        body: 'Select text and choose Ask the agent. The passage arrives quoted above the message box, and a follow-up like “make it shorter” still knows which paragraph you meant. Asked only a question, the agent changes nothing.'
      },
      {
        tag: 'Writer’s Direction',
        title: 'Your direction comes first',
        body: 'A current focus for the next few episodes, and a one-line intent for any scene. Only you write them, and every agent reads them first, as binding. Three written episodes on, the app asks whether the focus still holds.'
      },
      {
        tag: 'Continuity checks',
        title: 'Findings that show their work',
        body: 'The agent holds a scene against Story World, your fact cards, earlier scenes and your direction. Every finding must quote the scene exactly and point at something the work really has. The manuscript does not change.'
      },
      {
        tag: 'Suggestions inbox',
        title: 'Proposals wait for you',
        body: 'What the agent draws from the manuscript waits here to be accepted, edited or rejected, one at a time. A likely duplicate can become an alias instead. After a Markdown import, Linetta offers to fill Story World this way, with a token estimate first.'
      },
      {
        tag: 'Where the story could go',
        title: 'Branches, not canon',
        body: 'Two to five possible continuations side by side: a premise, beats episode by episode, risk badges. Nothing in the outline or the manuscript changes. Pick one and it goes into the agent’s brief as a plan, marked stale once the story moves on.'
      },
      {
        tag: 'Images in messages',
        title: 'Show it what you mean',
        body: 'Attach up to four images to a message, 5 MB each. They go to the model with that message only, and the agent can keep one as a character’s reference image. The Claude Code provider takes text only.'
      },
      {
        tag: 'Questions in a card',
        title: 'Several decisions, one answer',
        body: 'When the built-in agent needs more than one decision, it asks on a single card of up to six questions. Pick an option or write your own for each, then send them together. An open card survives a restart.'
      },
      {
        tag: 'Tidy on request',
        title: 'Housekeeping, when you say so',
        body: 'Tell it to, and the agent edits or deletes elements, relationships, fact cards and threads across Story World, clears continuity findings in bulk, and accepts or rejects what waits in the inbox. The last eight changes can be undone until the app restarts.'
      },
      {
        tag: 'Long conversations',
        title: 'It looks back instead of guessing',
        body: 'When a conversation outgrows what the built-in agent reads, it searches the part that was cut and reads around a match. Finished decisions in its work notes move to an archive it can still search, and you can start the conversation over without deleting any of it.'
      }
    ]
  },

  longform: {
    mark: '§ 04 — Long form',
    heading: 'Episode 100 still remembers episode 3.',
    sub: 'A long serial goes wrong where it forgets. Linetta keeps a ledger of what the story owes and what is settled, puts it in every brief, and checks new episodes against it. The agent does most of the checking, you see all of it, and none of it rewrites the manuscript.',
    cards: [
      {
        tag: 'Story ledger',
        title: 'What the story owes',
        body: 'Promises, running gags, open questions, decisions, numbers, the timeline, who knows what, and planned cuts — each open, paid or dropped. Paying one off asks for the line that does it. The agent records them as it writes and sees what is still open.'
      },
      {
        tag: 'Rules of the work',
        title: 'Pin what must not move',
        body: 'Pin a settled entry and it reaches every episode’s brief as a rule the agent cannot change or delete. A fact can name its subject, what is fixed and the episodes it holds for; recording a different value reports the conflict instead of saving over it.'
      },
      {
        tag: 'Arcs and parts',
        title: 'Promises with a horizon',
        body: 'A promise can reach an arc, a volume or the whole work, and leaves the brief past its reach unless it is due. Each part has a role, what must be true by its end and a target tension, and the brief shows the arc you are in.'
      },
      {
        tag: 'Scene plans',
        title: 'A plan beside the summary',
        body: 'Each scene keeps a plan of its own, apart from the summary of what was written. The brief hands it over as the plan for the scene being drafted.'
      },
      {
        tag: 'Independent review',
        title: 'A second reader for each episode',
        body: 'Turn it on and a separate, read-only pass rereads what the agent just wrote, without the context of writing it, and files what it finds as continuity findings. Off by default. The agent panel also offers a review of this episode and a critique of the whole part as one-tap follow-ups.'
      },
      {
        tag: 'After the turn',
        title: 'Settle, then trace the impact',
        body: 'Two optional passes: one settles what a finished episode changed into the ledger, the other finds earlier episodes a changed decision no longer agrees with. Both are off by default and send manuscript text to your provider again.'
      },
      {
        tag: 'Sample check',
        title: 'Two episodes, side by side',
        body: 'Continuity can compare episodes two at a time with no conversation behind it — the first episode, the last two and a related one — and records only contradictions.'
      },
      {
        tag: 'Style at length',
        title: 'Habits that only show over many episodes',
        body: 'Pick a scene as the reference for a work’s style. The style check finds “not this, but that” parallels, sentences that explain instead of show, sentences repeated across episodes, episodes that all end the same way, and the phrases you listed to avoid.'
      },
      {
        tag: 'Counts and copies',
        title: 'Counted the way your platform counts',
        body: 'Count characters with or without spaces, per work; the editor, the outline, the publish dialog and the agent’s brief all follow. Presets pair a count basis with an episode target. Duplicate a work to try a large change on the copy.'
      }
    ]
  },

  illustrations: {
    mark: '§ 05 — Illustrations',
    heading: 'Pictures that stay in character.',
    sub: 'Off until you choose ChatGPT or Gemini under Settings → Image generation, with its own consent. ChatGPT counts against your plan’s usage; Gemini is billed per image by Google AI. Nothing is drawn until you press Generate.',
    cards: [
      {
        tag: 'Draw this scene',
        title: 'From a passage to a picture',
        body: 'Select a passage, describe the moment, pick the characters, then choose a single picture or four panels. Linetta builds the prompt from the scene, the visual sheets and your art style. You can edit it, and what you edit is what goes.'
      },
      {
        tag: 'Character sheets',
        title: 'A sheet for every face',
        body: 'Appearance notes and up to eight reference images per character. Drop images in, or take one from the gallery: the face episode 1 got right.'
      },
      {
        tag: 'Earlier pictures',
        title: 'The same face in episode 40',
        body: 'Up to two earlier pictures go along after the reference images, those of the same characters first. They appear as thumbnails, and one click leaves a picture out.'
      },
      {
        tag: 'Agents ask',
        title: 'An agent cannot draw',
        body: 'It leaves a request card with a prompt, aspect and caption for you to edit. Nothing reaches the image provider until you press Generate.'
      },
      {
        tag: 'Gallery',
        title: 'Up to 200 pictures a work',
        body: 'Each keeps the prompt and model it came from. Insert one into a scene, or let it stand for a scene, an episode or a part on the outline.'
      },
      {
        tag: 'Everywhere the work goes',
        title: 'Pictures travel with the text',
        body: 'Illustrations are in library backups, inside a Markdown export, in folder and Git sync, and go up with an episode you publish.'
      }
    ]
  },

  publish: {
    mark: '§ 06 — Publish',
    heading: 'Readers, when you choose.',
    sub: 'Linetta Works is an optional reading site for fiction written in Linetta, run by Linetta’s developer. Nothing goes to it until you connect an account, and after that only what you choose to publish or sync. Every build publishes, the App Store apps included.',
    link: { label: 'Visit Linetta Works', href: WORKS },
    cards: [
      {
        tag: 'Episodes',
        title: 'One episode at a time',
        body: 'Publish from the outline or the command palette. Each episode gets an address, and the outline marks it Edited when the text moves on; readers keep the old text until you publish again. The publish dialog shows any open high-severity continuity findings first. An agent can publish when you ask, once you have connected.'
      },
      {
        tag: 'Sheets and cover',
        title: 'Show the world, if you want',
        body: 'Character and world sheets are private until you turn each one on, so you can check it for spoilers first. Make a gallery picture the cover straight from the gallery, or ask the agent to; the cover’s review state shows there. Publishing never sends notes.'
      },
      {
        tag: 'Share',
        title: 'Tell people',
        body: 'X, Threads and Naver Blog open with the title and link filled in. KakaoTalk and Instagram copy them for you to paste.'
      },
      {
        tag: 'What goes',
        title: 'Public, and reviewed',
        body: 'Published text, its illustrations and the sheets you chose are public, and the site sends them to the Anthropic API for content review. Publishing sends no unpublished episodes, notes, fact cards or agent memories.'
      },
      {
        tag: 'In the app',
        title: 'See what readers read',
        body: 'The library opens Linetta Works inside the app: search the works readers can see, filter by genre, and open one on the site. Browsing needs no account and sends nothing of yours.'
      },
      {
        tag: 'Sync',
        title: 'Carry a work between devices',
        body: 'With an account the operator has approved, Linetta Works sync seals a work on your device and picks it up on another computer, iPhone or iPad — scenes, story notes, pictures, the agent conversation, and your global skills and writer profile. Automatic sync sends changes a few minutes after you stop, you choose which kinds of data go, and when two devices edited the same work a conflict screen keeps both. Not in the Mac App Store build. A free beta, never your only copy.'
      }
    ]
  },

  agent: {
    mark: '§ 07 — Agents',
    heading: 'Bring your own agent,\nor your own key.',
    lead: 'Linetta ships no model and no subscription of its own. Connect an agent you already run — Claude Code, Claude Desktop, Codex CLI, Gemini CLI — over MCP, using the subscription you already pay for and no credential to Linetta at all. Or turn on the built-in agent below and give it a provider of your own.',
    paragraphs: [
      'A generic filesystem MCP server can already read and write Markdown. What it cannot assemble is the brief for <em>one scene</em>: where that scene sits in the outline, the summaries above it, the previous scene’s summary, character and relationship briefs, the plot spine, fact cards, memories, and your style and length targets.',
      'That brief is what Linetta hands over, and it is the reason a draft written outside the app comes back without contradicting episode fourteen. The writer keeps the desk, the filing cabinet, and the veto.'
    ],
    commandLabel: 'Claude Code — one line in your terminal',
    command: MCP_COMMAND,
    commandNote:
      'When Settings finds Claude Code, Claude Desktop, Codex CLI or Gemini CLI, it offers to connect them in one click through the <code>linetta-mcp</code> stdio bridge, which some builds ship and others do not — Settings says which, and where to get it otherwise. The token is generated when you turn MCP on and can be regenerated or revoked from Settings.',
    modesLabel: 'Access',
    modes: [
      { name: 'Off', body: 'The default. Nothing listens, and nothing is exposed.' },
      {
        name: 'Read only',
        body: 'Nineteen read tools. The write tools are never registered, so they do not appear in tools/list and cannot be called.'
      },
      {
        name: 'Full',
        body: 'All forty-six. A scene write is snapshotted first, and an outline restructuring can be undone in one call.'
      }
    ],
    toolsLabel: 'Forty-six tools, not a hundred RPCs',
    toolsNote:
      'A client’s tool budget is finite and selection accuracy falls as the list grows, so the engine’s RPC surface is not exposed one-to-one. Access can also be pinned to a single work, and Settings → Tool budget shows what each optional group costs and turns it off.',
    readLabel: 'Read',
    writeLabel: 'Write',
    readTools: [
      { name: 'linetta_get_story_context', note: 'The curated brief for one scene — the core tool' },
      { name: 'linetta_read_scene', note: 'Scene text with its content version' },
      { name: 'linetta_get_outline', note: 'The tree with labels, kinds, status and length' },
      { name: 'linetta_search_manuscript', note: 'Full-text search across the work' },
      { name: 'linetta_list_works', note: 'Works, titles, status, scene counts' },
      { name: 'linetta_list_characters', note: 'Characters, places, objects, concepts' },
      { name: 'linetta_where_does_appear', note: 'Every scene an entity appears in' },
      { name: 'linetta_list_proposals', note: 'What is waiting in the suggestions inbox' },
      { name: 'linetta_list_continuity', note: 'Recorded continuity findings and their ids' },
      { name: 'linetta_list_ledger', note: 'The story ledger, newest first: what is owed and what is settled' },
      { name: 'linetta_find_impact', note: 'Written scenes a ledger entry may touch, in story order' },
      { name: 'linetta_get_plot', note: 'Storylines and beats' },
      { name: 'linetta_get_fact_cards', note: 'Research notes with their sources' },
      { name: 'linetta_get_character_visuals', note: 'Character designs and reference images' },
      { name: 'linetta_build_illustration_prompt', note: 'Illustration and four-panel prompts' },
      { name: 'linetta_list_illustrations', note: 'The gallery, and how recent requests were decided' },
      { name: 'linetta_analyze_style', note: 'Reports where prose breaks your style rules' },
      { name: 'linetta_read_skill', note: 'Open one recorded how-to in full — the brief lists only names and descriptions' },
      { name: 'linetta_publish_status', note: 'What is published on Linetta Works' }
    ],
    writeTools: [
      { name: 'linetta_create_work', note: 'Start a new work with its first scene ready to draft' },
      { name: 'linetta_write_scene', note: 'Requires the expected content version' },
      { name: 'linetta_revise_scene', note: 'Partial revision without resending the scene' },
      { name: 'linetta_apply_story_ops', note: 'Structured story changes as one batch, including edits and deletions across Story World' },
      { name: 'linetta_write_summary', note: 'Scene, container and synopsis summaries' },
      { name: 'linetta_create_checkpoint', note: 'A labelled restore point before a big rewrite' },
      { name: 'linetta_undo_last_change', note: 'Undo a batch in one call' },
      { name: 'linetta_edit_memory', note: 'Record what it has learned — the writer profile, which applies to every work, or this work’s notes' },
      { name: 'linetta_edit_skill', note: 'Write a how-to as a SKILL.md file — global, or tied to one work. No approval step; the writer gets attribution, history and an off switch' },
      { name: 'linetta_propose_story_ops', note: 'Send candidates to the suggestions inbox; nothing is applied' },
      { name: 'linetta_accept_proposals', note: 'Accept inbox items when you tell it to' },
      { name: 'linetta_reject_proposals', note: 'Reject inbox items when you tell it to' },
      { name: 'linetta_report_continuity', note: 'Record continuity findings, each with an exact quote' },
      { name: 'linetta_update_continuity', note: 'Dismiss, resolve or delete findings in bulk when you tell it to' },
      { name: 'linetta_edit_ledger', note: 'Add, change or delete a ledger entry; pinned rules stay put' },
      { name: 'linetta_set_part_arc', note: 'A part’s role, goal and target tension' },
      { name: 'linetta_save_forecast', note: 'Save possible branches; none of it is canon' },
      { name: 'linetta_propose_style_profile', note: 'Drafts a style profile for you to approve' },
      { name: 'linetta_set_character_visuals', note: 'Save character designs' },
      { name: 'linetta_set_art_style', note: 'Save illustration style' },
      { name: 'linetta_add_reference_image', note: 'Attach a reference image, including one from the gallery' },
      { name: 'linetta_delete_reference_image', note: 'Remove reference image' },
      { name: 'linetta_request_illustration', note: 'Ask for a picture; nothing is drawn until you approve' },
      { name: 'linetta_set_representative_illustration', note: 'Set or clear the picture that stands for a scene, episode or part' },
      { name: 'linetta_register_work', note: 'Register a work on Linetta Works — only after you approve it in the app' },
      { name: 'linetta_publish_episode', note: 'Publish an episode — only after you approve it in the app' },
      { name: 'linetta_publish_cover', note: 'Upload a gallery picture as the cover — only after you approve it' }
    ],
    safetyLabel: 'What keeps it safe',
    safety: [
      'Bound to 127.0.0.1 only, with an Origin check against DNS rebinding. No LAN binding and no tunnel, at any setting.',
      'A 32-byte bearer token, held in the operating system’s secret store where there is one and in a 0600 file otherwise, regenerable and revocable from Settings.',
      'Optimistic version checks: if you are typing in the scene an agent is rewriting, the write is refused rather than silently applied.',
      'A snapshot before every scene write, and an outline capture before every structural change.',
      'An activity log of every call — time, tool, work, target, result — shown in Settings.',
      'A cap of 120 tool calls a minute, so a runaway agent loop hits a wall instead of forty rewritten scenes.'
    ],
    byokLabel: 'Or, the built-in agent',
    byokLead: 'Rather write with a panel inside Linetta than switch to a separate app? Turn on the built-in agent and connect a provider of your own.',
    byokProviders: [
      { name: 'ChatGPT (Codex)', note: 'Sign in with your ChatGPT account — no key to hold' },
      { name: 'Claude Code', note: 'Runs the claude CLI you already have, under your own sign-in — no key; not in the Mac App Store build' },
      { name: 'Anthropic', note: 'By API key' },
      { name: 'Google Gemini', note: 'By API key' },
      { name: 'OpenAI-compatible', note: 'By API key, any compatible endpoint — including one on your own machine' }
    ],
    byokPoints: [
      'Consent is per provider, and it gates even the connection test',
      'An API key goes into your OS’s secure store — on iPhone and iPad that is the Keychain, and the API-key providers are the ones that work there. Linux has no store, so only the ChatGPT sign-in and Claude Code work there',
      'Open it with Cmd/Ctrl+J — it reaches Linetta’s tools through the same MCP layer an external client uses',
      'Every call lands in the same activity log, marked with which agent made it',
      'A structural change or a scene the agent wrote gets an Undo button on its line — the last eight, and only while the app stays open. The previous text is also kept indefinitely as a restorable version',
      'It reads two short memories at the start of every turn — a writer profile and this work’s notes — and writes to them itself. The profile is global: it is not scoped to the work you have open. Both are yours to rewrite in Settings → Memory',
      'It also keeps skills — how-to documents about method, as plain SKILL.md files under your data directory, which you can edit in any editor or point your own Claude Code at. A skill can apply to every work or to one. It writes them without asking; you get the author badge, the version history and an off switch, in Settings → Skills',
      'After a turn that ran eight or more tool calls, and once your reply has gone, it asks the same provider and model one more time whether that turn taught it a skill — an extra call your provider meters, on by default, switched off under Settings → Skills. The Claude Code provider never runs it',
      'A turn stops at 48 tool calls by default, then says what it finished and what is left. Change the limit from 8 to 200 under Settings → Tool budget',
      'Conversation memory is sized to the provider — about 100,000 characters on Claude Code and 40,000 elsewhere — and you can set it from 20,000 to 200,000 under Settings',
      'The daily backup is the database only. It carries every skill’s version history and not the skills folder itself'
    ]
  },

  data: {
    mark: '§ 08 — Your data',
    heading: 'The manuscript is a file you own.',
    sub: 'Writing needs no account and no server. What you write is kept on this device, in formats you can read without Linetta, and leaves it only through the agent, publishing or sync you turn on.',
    cards: [
      {
        tag: 'library.db',
        title: 'One SQLite library',
        body: 'Projects, scenes, outline, entities, relationships, plot, version snapshots, the memories an agent reads, and every skill’s version history, in a single database file.'
      },
      {
        tag: 'Snapshots',
        title: 'History that thins, not disappears',
        body: 'Manual snapshots are kept indefinitely. Autosave snapshots go from every save on the first day to daily after thirty.'
      },
      {
        tag: 'backups/',
        title: 'Verified daily backups',
        body: 'A daily backup and a pre-migration backup, kept fourteen days, with restore controls at startup. It copies the database and nothing beside it — the skills folder included.'
      },
      {
        tag: 'Markdown',
        title: 'In and out, freely',
        body: 'Import an existing manuscript, export scenes as Markdown into a folder, and optionally let Git carry that folder somewhere else. Illustrations travel with the export.'
      }
    ],
    pathsLabel: 'Where it lives',
    paths: [
      { os: 'macOS', path: '~/Library/Application Support/com.devlikebear.linetta' },
      { os: 'Linux', path: '${XDG_DATA_HOME:-~/.local/share}/com.devlikebear.linetta' },
      { os: 'Windows', path: '%APPDATA%\\com.devlikebear.linetta' }
    ]
  },

  download: {
    mark: '§ 09 — Get it',
    heading: 'Free on the App Store for Mac, iPhone and iPad.\nSigned builds everywhere else.',
    sub: 'The direct macOS build is Developer ID signed and notarised by Apple. Windows and Linux installers are published on every GitHub release.',
    versionLabel: 'Current release',
    version: `v${VERSION}`,
    channels: [
      {
        id: 'mas',
        label: 'App Store — Mac, iPhone, iPad',
        note: 'Free. The simplest way onto an Apple Silicon Mac, and the way onto iPhone and iPad.',
        cta: { label: 'Open the App Store', href: MAS }
      },
      {
        id: 'brew',
        label: 'Homebrew — Apple Silicon',
        note: 'Signed and notarised, updated with the tap.',
        code: 'brew install --cask devlikebear/tap/linetta'
      },
      {
        id: 'windows',
        label: 'Windows',
        note: 'NSIS and MSI installers on every release.',
        cta: { label: 'Latest release', href: RELEASES }
      },
      {
        id: 'linux',
        label: 'Linux',
        note: 'AppImage, .deb and .rpm packages on every release.',
        cta: { label: 'Latest release', href: RELEASES }
      }
    ],
    sourceNote:
      'Intel Macs, and anyone who would rather compile it, can build from source: a Tauri 2 Rust shell, React and Vite, an embedded Go engine, and SQLite.'
  },

  faq: {
    mark: '§ 10 — Questions',
    heading: 'Before you download.',
    items: [
      {
        q: 'Does Linetta need an account or a subscription?',
        a: 'There is no subscription. Writing, organisation, import and export, snapshots and backups all work without an account. You need a Linetta Works account only to publish or to sync between devices.'
      },
      {
        q: 'Is there AI inside the app?',
        a: 'Optionally. A built-in agent can write for you once you connect a provider — a ChatGPT sign-in, the Claude Code CLI, Anthropic, Google Gemini or an OpenAI-compatible endpoint — and consent to it, provider by provider. Or skip that entirely and connect your own agent, such as Claude Code or Codex CLI, over the MCP server instead. Illustrations are a separate choice, with their own consent.'
      },
      {
        q: 'Can I bring an existing manuscript?',
        a: 'Yes. Markdown import and export both work, so a work can arrive from another tool and leave for one.'
      },
      {
        q: 'Is my writing uploaded anywhere?',
        a: 'Not by default. It leaves this device only once you turn something on: the built-in agent sends what it works on to the provider you connected, an MCP client you run reads it, the image provider you chose receives a prompt and any reference images when you press Generate, an episode you publish goes to Linetta Works for anyone to read, a work you upload through Linetta Works sync goes to the same operator’s server sealed so nobody there can read it, and GitHub Sync or Folder Sync, once configured, export your whole library on a daily tick and at every launch, along with a sealed copy if you choose them as a sync target.'
      },
      {
        q: 'What about iPhone, iPad and Android?',
        a: 'Linetta is on the App Store for iPhone and iPad. There you plan and write the same works, use the built-in agent with an API key (Anthropic, Google Gemini or an OpenAI-compatible endpoint), publish to Linetta Works and sync through it. The iOS app hosts no MCP server and has no Git or folder sync. There is no Android app.'
      },
      {
        q: 'What licence is it under?',
        a: 'AGPL-3.0-only. The source is on GitHub, and commercial licensing options are described in the licence notice.'
      }
    ]
  },

  footer: {
    tagline:
      'A calm writing studio for long-form fiction, connected to Linetta Works. Built by one developer, in the open.',
    cols: { project: 'Project', docs: 'Documentation', get: 'Get Linetta' },
    links: {
      project: [
        { label: 'GitHub', href: REPO },
        { label: 'Linetta Works', href: WORKS },
        { label: 'Releases', href: `${REPO}/releases` },
        { label: 'Changelog', href: `${REPO}/blob/main/CHANGELOG.md` },
        { label: 'Issues', href: `${REPO}/issues` }
      ],
      docs: [
        { label: 'README', href: `${REPO}#readme` },
        { label: 'Development guide', href: `${REPO}/blob/main/docs/DEVELOPMENT.md` },
        { label: 'Privacy policy', href: `${REPO}/blob/main/docs/privacy-policy.md` },
        { label: 'Licence notice', href: `${REPO}/blob/main/LICENSE-NOTICE.md` }
      ],
      get: [
        { label: 'App Store', href: MAS },
        { label: 'Homebrew tap', href: 'https://github.com/devlikebear/homebrew-tap' },
        { label: 'Windows & Linux', href: RELEASES }
      ]
    },
    legal: 'AGPL-3.0-only · an independent project by devlikebear'
  }
};

export const ko: Translation = {
  locale: 'ko',
  htmlLang: 'ko',
  ogLocale: 'ko_KR',
  path: '/ko',
  alts: alts('ko'),
  pageTitle: 'Linetta — 장편 소설을 쓰고 Linetta Works에 연재하는 집필 스튜디오',
  metaDescription:
    '아웃라인, 등장인물, 플롯 스레드, 자료, 그리고 이야기가 갚아야 할 것을 적은 원장을 원고 옆에 두고 씬 단위로 씁니다. 원할 때는 내장 에이전트나 MCP로 연결한 내 에이전트가 긴 연재의 연속성을 점검하고, 장면은 삽화가 되고, 회차는 Linetta Works에서 독자를 만나고, 작품은 Mac·PC·iPhone·iPad 사이를 따라옵니다.',

  nav: {
    workspace: '작업 공간',
    longform: '장편',
    assist: '집필 보조',
    illustrations: '삽화',
    publish: '게시',
    agent: '에이전트',
    data: '내 데이터',
    download: '내려받기',
    github: 'GitHub',
    cta: 'Linetta 받기',
    skip: '본문으로 건너뛰기'
  },

  hero: {
    mark: 'macOS · iPhone · iPad · Windows · Linux — Linetta Works와 함께',
    headline: [
      { text: '쓰고 있는 문장 옆에,' },
      { nl: true },
      { text: '작품 전체를 ' },
      { text: '펼쳐 둡니다.', accent: true }
    ],
    sub: 'Linetta는 소설과 웹소설을 위한 조용한 집필 스튜디오입니다. 아웃라인, 등장인물, 장소, 관계, 플롯 스레드, 조사 자료, 그리고 이야기가 아직 갚지 않은 약속을 적은 원장이 원고 옆에 놓입니다. 원할 때는 에이전트 — 내장 에이전트든, MCP로 연결한 내 에이전트든 — 가 긴 연재의 앞뒤를 맞추고, 장면은 삽화가 되고, 회차는 Linetta Works에서 독자를 만나고, 작품은 책상에서 iPhone까지 따라옵니다. 전부 선택입니다.',
    ctaPrimary: 'Linetta 받기',
    ctaSecondary: 'GitHub에서 소스 보기',
    pills: ['무료, 구독 없음', '내장 에이전트 또는 내 에이전트', '한국어 · English · 日本語'],
    imageAlt:
      'Linetta 작업 공간. 왼쪽에 아웃라인 레일, 가운데에 씬 도입부가 놓인 본문 편집기, 오른쪽에 이 씬의 글자 수·목표·시놉시스 패널.'
  },

  intro: {
    mark: '§ 01 — 무엇인가',
    heading: '집필은\n프로젝트 관리가 아닙니다.',
    paragraphs: [
      '장편은 상태를 쌓아 갑니다. 40화쯤 되면 3장에서 한 약속, 갚지 않은 빚, 누군가의 코트 색깔이 전부 하중을 받는 기둥이 됩니다. 그런데 그중 무엇도 지금 화면의 문단 안에는 없습니다.',
      '대부분의 도구는 여기에 절차를 더 얹어 답합니다. 보드, 태그, 상태값, 그리고 따로 관리해야 하는 두 번째 앱. Linetta는 대신 작품의 기억을 <em>문장 옆에</em> 둡니다. 단축키 하나 거리에, 그러나 길을 막지 않는 자리에.',
      '그래서 편집기는 조용한 본문 한 장으로 남습니다. 아웃라인, 등장인물, 플롯 스파인, 자료, 이력은 문장이 필요로 할 때 여는 레일과 패널이고, 필요 없을 때는 닫습니다.'
    ]
  },

  workspace: {
    mark: '§ 02 — 작업 공간',
    heading: '작품 전체가 한 방에.',
    sub: '모든 패널이 같은 질문 위에 서 있습니다. 이 씬은 무엇과 어긋나면 안 되는가?',
    panels: [
      {
        id: 'workspace',
        tab: '작업 공간',
        kicker: '본문',
        title: '본문 한 장, 그 옆의 아웃라인, 그리고 이 씬의 기록.',
        body: '아웃라인은 손 닿는 곳에, 글자 수는 무시할 수 있는 자리에 둔 채 씬 단위로 씁니다. 오른쪽 레일에는 문장을 떠나지 않고 확인하는 것들이 있습니다 — 오늘 쓴 글자 수, 화 분량 목표, 시놉시스, 이 씬이 언급한 엔티티.',
        alt: '작업 공간. 왼쪽에 아웃라인 레일, 가운데에 본문 편집기, 오른쪽에 442자·화 분량 목표·작품 개요를 보여주는 패널.',
        points: [
          '부·장·씬 또는 권·화 — 번호와 라벨은 인터페이스 언어에 맞춰 붙습니다',
          '씬마다 분량 목표와 실시간 글자 수, 그 옆에 작품 총합',
          'ZEN은 방을 비워 본문과 카운터만 남깁니다'
        ]
      },
      {
        id: 'story-world',
        tab: '스토리 월드',
        kicker: '창작된 세계',
        title: '인물·장소·사물·개념, 그리고 그 사이의 관계.',
        body: '팩트북이 현실 세계를 위한 것이라면, 스토리 월드는 지어낸 세계를 위한 것입니다. 카드마다 역할과 요약, 맺고 있는 관계 수가 붙고, 필터로 한 종류만 추려 볼 수 있습니다.',
        alt: '스토리 월드 패널. 장소 하나, 인물 둘, 개념 하나, 사물 하나의 카드가 각각 역할과 한 줄 요약, 관계 수를 달고 나열되어 있습니다.',
        points: [
          '주인공·주요 무대·빌런 같은 역할과, 그것들을 잇는 관계',
          '이름·별칭·역할·요약으로 검색',
          '요청하면 에이전트가 바로 추가할 수 있고, 원고에서 뽑아낸 것은 제안함에서 기다립니다'
        ]
      },
      {
        id: 'fact-book',
        tab: '팩트북',
        kicker: '자료',
        title: '출처가 붙은 메모를, 그 자료가 필요한 씬 옆에.',
        body: '무엇을 어디서 확인했는지 적으면 Linetta가 그 페이지를 가져와 옆에 정리해 둡니다. 카드에는 확인됨 또는 불확실 표시가 붙어서, 아직 결론 나지 않은 주장이 슬그머니 사실로 굳지 않습니다.',
        alt: '팩트북 패널. 조사 카드마다 확인됨 또는 불확실 배지가 붙고 출처가 함께 표시되어 있습니다.',
        points: [
          '확인됨·불확실 배지와, 카드에 남는 출처',
          '현재 씬 또는 작품 전체로 범위 지정',
          'MCP로 연결된 에이전트도 이 카드들을 읽습니다'
        ]
      },
      {
        id: 'library',
        tab: '서재',
        kicker: '서가',
        title: '한 줄에서 한 권까지.',
        body: '작품이 여러 권을 전제로 만든 서가에 놓입니다. 분량과 상태가 함께 보이고, 새 작품을 시작하거나 쓰던 원고를 마크다운으로 가져올 수 있습니다.',
        alt: 'Linetta 서재 화면. 작품 카드 하나와 새 작품·마크다운 가져오기·검색·Linetta Works 둘러보기 버튼.',
        points: [
          '새 작품, 마크다운 가져오기, 전체 검색',
          '진행 중과 아카이브',
          'Linetta Works를 곁에 — 회차를 연재하고 독자가 읽는 작품을 둘러봅니다'
        ]
      }
    ],
    alsoLabel: '같은 방 안에',
    also: [
      {
        tag: '컨텍스트 편집',
        title: '설정을 한 번에 정리',
        body: '인물·장소·관계를 한 번 고치면, 옛 설정을 아직 담고 있는 씬들을 찾아 하나씩 안내합니다. 언어 모델은 전혀 쓰지 않습니다.'
      },
      {
        tag: '플롯',
        title: '스파인 위의 실',
        body: '스토리라인과 비트가 아웃라인을 가로질러 놓입니다. 3화에서 연 약속이 뒤늦게 떠올리는 것이 아니라 보이는 것이 됩니다.'
      },
      {
        tag: '이력',
        title: '씬 단위 스냅샷',
        body: '수동·자동 스냅샷을 복원할 수 있습니다. 수동 스냅샷과 에이전트가 쓰기 전에 찍힌 스냅샷은 기한 없이 보관됩니다.'
      },
      {
        tag: '기록',
        title: '집필 속도',
        body: '7일 평균과 그 속도로 환산한 주당 화수. 연속 기록도, 죄책감도 없습니다.'
      }
    ]
  },

  assist: {
    mark: '§ 03 — 집필 보조',
    heading: '책상 옆의 두 번째 독자.',
    sub: '여기 있는 것은 전부 선택이고, 연결한 에이전트가 있어야 합니다. 연결 방법은 § 07에 있습니다. 에이전트는 읽고, 점검하고, 제안합니다. 원고는 요청할 때만 바뀝니다.',
    cards: [
      {
        tag: '에이전트에게 묻기',
        title: '고른 문단에 대해 묻기',
        body: '글을 선택하고 에이전트에게 묻기를 고르면 그 문단이 입력창 위에 인용됩니다. “더 짧게” 같은 후속 요청도 어느 문단인지 압니다. 질문만 하면 에이전트는 아무것도 바꾸지 않습니다.'
      },
      {
        tag: '작가 방향',
        title: '작가의 방향이 먼저입니다',
        body: '앞으로 몇 회차의 초점과, 씬마다 한 줄의 의도. 작가만 쓸 수 있고, 모든 에이전트가 가장 먼저 읽고 지켜야 할 지시로 다룹니다. 회차 셋을 쓰고 나면 앱이 그 초점이 아직 유효한지 묻습니다.'
      },
      {
        tag: '연속성 점검',
        title: '근거를 대는 지적',
        body: '에이전트가 씬을 스토리 월드, 팩트 카드, 앞선 씬, 작가 방향과 맞대어 봅니다. 모든 지적은 씬을 정확히 인용하고 작품에 실제로 있는 것을 가리켜야 합니다. 원고는 바뀌지 않습니다.'
      },
      {
        tag: '제안함',
        title: '제안은 작가를 기다립니다',
        body: '에이전트가 원고에서 뽑아낸 것은 여기서 하나씩 수락, 수정, 거절을 기다립니다. 같은 대상으로 보이면 별칭으로 합칠 수 있습니다. 마크다운을 가져온 뒤에는 토큰 추정치를 먼저 보여주고 이 방식으로 스토리 월드를 채워 줍니다.'
      },
      {
        tag: '갈래 예측',
        title: '정사가 아닌 갈래',
        body: '앞으로 가능한 전개 2~5개를 나란히 봅니다. 전제, 회차별 비트, 위험 배지가 붙습니다. 아웃라인도 원고도 바뀌지 않습니다. 하나를 고르면 에이전트 브리프에 계획으로 들어가고, 작품이 달라지면 오래됨으로 표시됩니다.'
      },
      {
        tag: '메시지에 이미지',
        title: '보여 주며 말하기',
        body: '메시지 하나에 이미지를 4장까지, 장당 5MB까지 첨부합니다. 이미지는 그 메시지와 함께만 모델로 가고, 에이전트는 그중 하나를 인물의 참조 이미지로 저장할 수 있습니다. Claude Code 프로바이더는 텍스트만 받습니다.'
      },
      {
        tag: '질문 카드',
        title: '여러 결정을 한 번에',
        body: '내장 에이전트가 정할 것이 여러 개면 질문을 최대 여섯 개까지 카드 한 장에 묻습니다. 질문마다 보기를 고르거나 직접 적고 한 번에 보냅니다. 열린 카드는 앱을 다시 켜도 남습니다.'
      },
      {
        tag: '시키면 정리',
        title: '정리도, 말할 때만',
        body: '시키면 에이전트가 세계관 전체의 요소, 관계, 팩트 카드, 스레드를 고치거나 지우고, 연속성 점검 결과를 한꺼번에 정리하고, 제안함에 쌓인 것을 수락하거나 거절합니다. 최근 변경 8건은 앱을 다시 켜기 전까지 되돌릴 수 있습니다.'
      },
      {
        tag: '긴 대화',
        title: '추측하지 않고 다시 찾아봅니다',
        body: '대화가 내장 에이전트가 읽는 분량을 넘으면, 잘린 부분을 검색해 찾은 곳의 앞뒤를 읽습니다. 작업 노트의 끝난 결정은 지우지 않고 보관해 계속 찾을 수 있고, 대화는 지우지 않은 채 새로 시작할 수 있습니다.'
      }
    ]
  },

  longform: {
    mark: '§ 04 — 장편',
    heading: '100화가 되어도 3화를 기억합니다.',
    sub: '긴 연재는 잊은 곳에서 무너집니다. Linetta는 이야기가 갚아야 할 것과 정해진 것을 원장에 적고, 모든 브리프에 넣고, 새 회차를 그것과 맞대어 봅니다. 점검은 대부분 에이전트가 하고, 작가는 그 전부를 보며, 원고를 스스로 고치는 것은 없습니다.',
    cards: [
      {
        tag: '작품 원장',
        title: '이야기가 갚아야 할 것',
        body: '약속, 반복 개그, 열린 질문, 결정, 숫자, 타임라인, 누가 무엇을 아는지, 지울 계획까지 — 각각 열림, 회수, 버림으로 표시됩니다. 회수하려면 그 약속을 갚는 문장을 적어야 합니다. 에이전트는 쓰면서 기록하고, 아직 열린 것을 보며 씁니다.'
      },
      {
        tag: '작품의 규칙',
        title: '움직이면 안 되는 것은 고정',
        body: '정해진 사실을 고정하면 모든 회차의 브리프에 규칙으로 들어가고, 에이전트는 바꾸거나 지울 수 없습니다. 사실에는 대상, 고정된 내용, 유효한 회차를 적을 수 있고, 다른 값을 기록하려 하면 덮어쓰지 않고 충돌을 알립니다.'
      },
      {
        tag: '아크와 부',
        title: '범위가 있는 약속',
        body: '약속은 아크, 권, 작품 전체 중 어디까지 유효한지 정할 수 있고, 그 범위를 지나면 기한이 된 것이 아닌 한 브리프에서 빠집니다. 부마다 역할, 끝날 때까지 이뤄야 할 것, 목표 긴장도를 두고, 브리프는 지금 어느 아크인지 보여 줍니다.'
      },
      {
        tag: '씬 계획',
        title: '요약 옆의 계획',
        body: '씬마다 이미 쓴 내용의 요약과 따로 계획을 둡니다. 브리프는 그것을 지금 쓰는 씬의 계획으로 넘깁니다.'
      },
      {
        tag: '독립 검수',
        title: '회차마다 두 번째 독자',
        body: '켜 두면 에이전트가 방금 쓴 것을, 쓴 맥락이 없는 별도의 읽기 전용 호출이 다시 읽고 찾은 것을 연속성 점검에 남깁니다. 기본은 꺼짐입니다. 에이전트 패널은 이 회차 검토와 부 전체 비평도 한 번에 누르는 후속 요청으로 내놓습니다.'
      },
      {
        tag: '턴이 끝난 뒤',
        title: '정리하고, 영향을 찾고',
        body: '고를 수 있는 두 단계입니다. 하나는 끝난 회차가 바꾼 것을 원장에 정리하고, 다른 하나는 바뀐 결정과 더는 맞지 않는 앞 회차를 찾습니다. 둘 다 기본은 꺼짐이고, 원고를 프로바이더에 다시 보냅니다.'
      },
      {
        tag: '표본 대조',
        title: '두 회차를 나란히',
        body: '연속성 점검이 대화 없이 회차를 두 개씩 맞대어 봅니다. 첫 회차, 직전 두 회차, 관련 회차를 고르고, 어긋남만 기록합니다.'
      },
      {
        tag: '장편의 문체',
        title: '여러 회차에 걸쳐야 보이는 버릇',
        body: '씬 하나를 작품 문체의 기준으로 고릅니다. 문체 검사는 “A가 아니라 B” 식의 대구, 보여 주지 않고 설명하는 문장, 회차를 넘나들며 반복되는 문장, 늘 같은 방식으로 끝나는 회차, 그리고 피하겠다고 적어 둔 표현을 찾습니다.'
      },
      {
        tag: '분량과 복제',
        title: '연재처의 기준대로 세기',
        body: '글자 수를 공백 포함으로 셀지 제외로 셀지 작품마다 고르면 편집기, 아웃라인, 게시 창, 에이전트 브리프가 모두 따릅니다. 프리셋은 그 기준과 회차 목표를 한 번에 정합니다. 큰 변경은 작품을 복제해 사본에서 시험할 수 있습니다.'
      }
    ]
  },

  illustrations: {
    mark: '§ 05 — 삽화',
    heading: '인물이 흔들리지 않는 그림.',
    sub: '설정 → 이미지 생성에서 ChatGPT나 Gemini를 고르기 전까지 꺼져 있고, 동의도 따로 받습니다. ChatGPT는 요금제 사용량에서 차감되고 Gemini는 Google AI가 장당 과금합니다. 생성을 누르기 전에는 아무것도 그리지 않습니다.',
    cards: [
      {
        tag: '이 장면으로 삽화 만들기',
        title: '문단에서 그림으로',
        body: '문단을 선택하고 장면을 설명한 뒤 인물을 고르고, 한 장 또는 4컷을 정합니다. 프롬프트는 장면, 비주얼 시트, 화풍으로 만들어지고, 직접 고칠 수 있으며 고친 그대로 전송됩니다.'
      },
      {
        tag: '인물 시트',
        title: '얼굴마다 한 장의 시트',
        body: '인물마다 외형 메모와 참조 이미지 최대 8장. 이미지를 끌어다 놓거나, 1화 삽화에서 잘 나온 얼굴처럼 갤러리에서 가져올 수 있습니다.'
      },
      {
        tag: '이전 삽화',
        title: '40화에도 같은 얼굴로',
        body: '참조 이미지 다음에 이전 삽화를 2장까지 함께 보냅니다. 같은 인물이 나온 그림이 먼저입니다. 썸네일로 보이고, 한 번 누르면 뺄 수 있습니다.'
      },
      {
        tag: '에이전트의 요청',
        title: '에이전트는 그리지 못합니다',
        body: '프롬프트, 비율, 캡션이 담긴 요청 카드를 남길 뿐이고, 작가가 고칠 수 있습니다. 생성을 누르기 전에는 이미지 프로바이더로 아무것도 가지 않습니다.'
      },
      {
        tag: '갤러리',
        title: '작품당 200장까지',
        body: '그림마다 프롬프트와 모델이 함께 남습니다. 씬에 넣거나, 아웃라인에서 씬·회차·부를 대표하게 할 수 있습니다.'
      },
      {
        tag: '작품이 가는 곳마다',
        title: '그림은 글과 함께 다닙니다',
        body: '삽화는 서재 백업, 마크다운 내보내기, 폴더·Git 동기화에 함께 들어가고, 게시하는 회차와 함께 올라갑니다.'
      }
    ]
  },

  publish: {
    mark: '§ 06 — 게시',
    heading: '독자는, 원할 때.',
    sub: 'Linetta Works는 Linetta로 쓴 소설을 읽는 선택형 사이트이고, Linetta 개발자가 운영합니다. 계정을 연결하기 전에는 아무것도 가지 않고, 연결한 뒤에도 게시하거나 동기화하기로 고른 것만 갑니다. App Store의 Mac·iPhone·iPad 앱을 포함해 모든 빌드에서 게시할 수 있습니다.',
    link: { label: 'Linetta Works 둘러보기', href: WORKS },
    cards: [
      {
        tag: '회차',
        title: '한 회차씩',
        body: '아웃라인이나 명령 팔레트에서 게시합니다. 회차마다 주소가 생기고, 본문이 바뀌면 아웃라인에 수정됨으로 표시되며, 독자는 다시 게시할 때까지 이전 본문을 봅니다. 게시 창은 열려 있는 심각한 연속성 지적을 먼저 보여 줍니다. 계정을 연결한 뒤에는 요청하면 에이전트도 게시할 수 있습니다.'
      },
      {
        tag: '시트와 표지',
        title: '세계관도, 원한다면',
        body: '인물·세계관 시트는 하나씩 켜기 전까지 비공개이므로 스포일러를 먼저 확인할 수 있습니다. 갤러리에서 그림을 골라 바로 표지로 올리거나 에이전트에게 맡길 수 있고, 표지의 심사 상태도 거기서 보입니다. 메모는 게시로 보내지 않습니다.'
      },
      {
        tag: '공유',
        title: '알리기',
        body: 'X, Threads, 네이버 블로그는 제목과 링크가 채워진 채 열립니다. 카카오톡과 인스타그램은 붙여 넣을 수 있게 복사해 줍니다.'
      },
      {
        tag: '가는 것',
        title: '공개되고, 심사됩니다',
        body: '게시한 본문과 삽화, 고른 시트는 공개되고, 사이트가 내용 심사를 위해 Anthropic API로 보냅니다. 게시하지 않은 회차, 메모, 팩트 카드, 에이전트 기억은 게시로 보내지 않습니다.'
      },
      {
        tag: '앱 안에서',
        title: '독자가 읽는 작품 둘러보기',
        body: '서재에서 앱 안으로 Linetta Works를 엽니다. 독자에게 보이는 작품을 찾고 장르로 거르고, 하나를 골라 사이트에서 엽니다. 둘러보기에는 계정이 필요 없고, 내 것은 아무것도 보내지 않습니다.'
      },
      {
        tag: '동기화',
        title: '기기 사이로 작품 옮기기',
        body: '운영자가 허가한 계정이면 Linetta Works 동기화가 작품을 기기에서 암호화해 맡기고, 다른 컴퓨터나 iPhone·iPad에서 이어 받습니다. 원고, 설정, 그림, 에이전트와 나눈 대화, 전역 스킬과 작가 프로필까지 갑니다. 자동 동기화를 켜면 편집을 멈추고 몇 분 뒤 올라가고, 옮길 데이터 종류를 고를 수 있으며, 두 기기에서 같은 작품을 고쳤다면 충돌 화면이 두 원고를 모두 남깁니다. Mac App Store 빌드에는 없습니다. 무료 베타이며 유일한 사본으로 삼지 마십시오.'
      }
    ]
  },

  agent: {
    mark: '§ 07 — 에이전트',
    heading: '내 에이전트를 데려오거나,\n내 키를 연결하세요.',
    lead: 'Linetta 자체에는 모델도 구독도 없습니다. 이미 쓰고 있는 에이전트 — Claude Code, Claude Desktop, Codex CLI, Gemini CLI — 를 MCP로 연결하면 이미 있는 구독으로 동작하고, Linetta에는 어떤 자격 증명도 넘기지 않습니다. 아니면 아래의 내장 에이전트를 켜고 직접 연결한 프로바이더를 쓸 수도 있습니다.',
    paragraphs: [
      '일반적인 파일시스템 MCP 서버도 마크다운은 읽고 씁니다. 그것이 조립하지 못하는 것은 <em>씬 하나를 위한 브리프</em>입니다. 그 씬이 아웃라인의 어디에 있는지, 상위 계층의 요약, 직전 씬의 요약, 인물과 관계 브리프, 플롯 스파인, 팩트 카드, 메모리, 그리고 문체와 분량 목표.',
      '그 브리프가 Linetta가 건네는 것이고, 앱 바깥에서 쓴 초고가 14화와 모순되지 않고 돌아오는 이유입니다. 책상과 서류함과 거부권은 계속 작가가 쥐고 있습니다.'
    ],
    commandLabel: 'Claude Code — 터미널에 한 줄',
    command: MCP_COMMAND,
    commandNote:
      '설정이 Claude Code, Claude Desktop, Codex CLI, Gemini CLI를 찾으면 <code>linetta-mcp</code> stdio 브리지로 한 번에 연결해 줍니다. 이 브리지를 함께 배포하는 빌드도, 그렇지 않은 빌드도 있으며 어느 쪽인지는 설정 화면이 알려줍니다. 토큰은 MCP를 켤 때 발급되고 설정에서 재발급·폐기할 수 있습니다.',
    modesLabel: '권한',
    modes: [
      { name: '꺼짐', body: '기본값입니다. 아무것도 열리지 않고, 아무것도 노출되지 않습니다.' },
      {
        name: '읽기 전용',
        body: '읽기 툴 19개. 쓰기 툴은 아예 등록되지 않아 tools/list에 나타나지도, 호출되지도 않습니다.'
      },
      {
        name: '전체',
        body: '46개 전부. 씬 쓰기는 먼저 스냅샷되고, 아웃라인 구조 변경은 한 번의 호출로 되돌릴 수 있습니다.'
      }
    ],
    toolsLabel: 'RPC 100개가 아니라, 툴 46개',
    toolsNote:
      '클라이언트의 툴 예산은 유한하고 목록이 길어질수록 선택 정확도가 떨어집니다. 그래서 엔진 RPC를 1:1로 노출하지 않습니다. 접근을 한 작품으로 제한할 수도 있고, 설정 → 툴 예산에서 선택 그룹별 비용을 보고 끌 수 있습니다.',
    readLabel: '읽기',
    writeLabel: '쓰기',
    readTools: [
      { name: 'linetta_get_story_context', note: '씬 하나를 위한 큐레이션 브리프 — 핵심 툴' },
      { name: 'linetta_read_scene', note: '본문과 content_version' },
      { name: 'linetta_get_outline', note: '라벨·종류·상태·분량이 붙은 트리' },
      { name: 'linetta_search_manuscript', note: '원고 전문 검색' },
      { name: 'linetta_list_works', note: '작품, 제목, 상태, 씬 수' },
      { name: 'linetta_list_characters', note: '인물, 장소, 사물, 개념' },
      { name: 'linetta_where_does_appear', note: '특정 엔티티가 등장하는 모든 씬' },
      { name: 'linetta_list_proposals', note: '제안함에서 대기 중인 항목' },
      { name: 'linetta_list_continuity', note: '기록된 연속성 점검 결과와 id' },
      { name: 'linetta_list_ledger', note: '작품 원장 — 갚아야 할 것과 정해진 것, 최신순' },
      { name: 'linetta_find_impact', note: '원장 항목이 건드릴 수 있는 쓴 씬들, 이야기 순서로' },
      { name: 'linetta_get_plot', note: '스토리라인과 비트' },
      { name: 'linetta_get_fact_cards', note: '출처가 붙은 조사 노트' },
      { name: 'linetta_get_character_visuals', note: '인물 디자인과 참조 이미지' },
      { name: 'linetta_build_illustration_prompt', note: '삽화와 4컷 프롬프트' },
      { name: 'linetta_list_illustrations', note: '갤러리와 최근 요청의 처리 결과' },
      { name: 'linetta_analyze_style', note: '작가의 문체 규칙에 어긋난 곳을 찾아 보고' },
      { name: 'linetta_read_skill', note: '기록해 둔 기법 하나를 본문까지 열기 — 브리프에는 이름과 설명만 실립니다' },
      { name: 'linetta_publish_status', note: 'Linetta Works에 게시된 내용' }
    ],
    writeTools: [
      { name: 'linetta_create_work', note: '첫 씬이 준비된 새 작품 만들기' },
      { name: 'linetta_write_scene', note: '읽은 시점의 content_version 필수' },
      { name: 'linetta_revise_scene', note: '씬 전체를 다시 보내지 않는 부분 수정' },
      { name: 'linetta_apply_story_ops', note: '구조화된 스토리 변경을 한 배치로 — 세계관 전체의 수정과 삭제 포함' },
      { name: 'linetta_write_summary', note: '씬·컨테이너 요약과 작품 시놉시스' },
      { name: 'linetta_create_checkpoint', note: '큰 개작 전 라벨 붙은 복원 지점' },
      { name: 'linetta_undo_last_change', note: '배치 단위로 한 번에 되돌리기' },
      { name: 'linetta_edit_memory', note: '알아낸 것을 기록 — 모든 작품에 적용되는 작가 프로필, 또는 이 작품의 노트' },
      { name: 'linetta_edit_skill', note: '기법을 SKILL.md 파일로 기록 — 전역이거나 한 작품에 묶이거나. 승인 절차는 없고, 작가는 대신 작성자 표시와 버전 기록과 끄는 스위치를 받습니다' },
      { name: 'linetta_propose_story_ops', note: '후보를 제안함으로 보냄 — 아무것도 적용되지 않습니다' },
      { name: 'linetta_accept_proposals', note: '작가가 시킬 때 제안함 항목을 수락' },
      { name: 'linetta_reject_proposals', note: '작가가 시킬 때 제안함 항목을 거절' },
      { name: 'linetta_report_continuity', note: '연속성 점검 결과를 정확한 인용과 함께 기록' },
      { name: 'linetta_update_continuity', note: '작가가 시킬 때 점검 결과를 한꺼번에 무시·해결·삭제' },
      { name: 'linetta_edit_ledger', note: '원장 항목 추가·수정·삭제 — 고정된 규칙은 건드리지 못함' },
      { name: 'linetta_set_part_arc', note: '부의 역할, 목표, 목표 긴장도' },
      { name: 'linetta_save_forecast', note: '가능한 갈래를 저장 — 어느 것도 정사가 아닙니다' },
      { name: 'linetta_propose_style_profile', note: '작가가 승인할 문체 프로필 초안 제안' },
      { name: 'linetta_set_character_visuals', note: '인물 디자인 저장' },
      { name: 'linetta_set_art_style', note: '화풍 저장' },
      { name: 'linetta_add_reference_image', note: '참조 이미지 추가 — 갤러리의 그림도 가능' },
      { name: 'linetta_delete_reference_image', note: '참조 이미지 삭제' },
      { name: 'linetta_request_illustration', note: '삽화 요청 — 승인 전에는 그리지 않습니다' },
      { name: 'linetta_set_representative_illustration', note: '씬·회차·부를 대표하는 그림 지정 또는 해제' },
      { name: 'linetta_register_work', note: '작품을 Linetta Works에 등록 — 앱에서 작가가 승인해야 올라감' },
      { name: 'linetta_publish_episode', note: '회차 게시 — 앱에서 작가가 승인해야 올라감' },
      { name: 'linetta_publish_cover', note: '갤러리 그림을 작품 표지로 — 앱에서 작가가 승인해야 올라감' }
    ],
    safetyLabel: '무엇이 이것을 안전하게 하는가',
    safety: [
      '127.0.0.1에만 바인딩하고 DNS 리바인딩을 막는 Origin 검사를 합니다. 어떤 설정에서도 LAN 바인딩이나 터널은 없습니다.',
      '32바이트 베어러 토큰. 시크릿 저장소가 있는 운영체제에서는 거기에, 없으면 0600 파일에 보관하며, 설정에서 재발급·폐기할 수 있습니다.',
      '낙관적 버전 검사: 에이전트가 고쳐 쓰는 씬을 당신이 타이핑 중이면, 쓰기는 조용히 적용되지 않고 거부됩니다.',
      '씬을 쓰기 전마다 스냅샷, 구조를 바꾸기 전마다 아웃라인 캡처.',
      '모든 호출의 활동 로그 — 시각, 툴, 작품, 대상, 결과 — 를 설정에서 보여줍니다.',
      '분당 120회 호출 상한. 폭주하는 에이전트 루프는 씬 40개가 아니라 벽에 부딪힙니다.'
    ],
    byokLabel: '또는, 내장 에이전트',
    byokLead: '별도 앱 대신 Linetta 안의 패널에서 쓰고 싶다면, 내장 에이전트를 켜고 직접 프로바이더를 연결하세요.',
    byokProviders: [
      { name: 'ChatGPT (Codex)', note: 'ChatGPT 계정으로 로그인 — 키를 보관할 필요 없음' },
      { name: 'Claude Code', note: '이미 설치한 claude CLI를 내 로그인으로 실행 — 키 불필요. Mac App Store 빌드에는 없음' },
      { name: 'Anthropic', note: 'API 키로' },
      { name: 'Google Gemini', note: 'API 키로' },
      { name: 'OpenAI 호환', note: 'API 키로, 호환되는 어떤 엔드포인트든 — 내 컴퓨터에서 도는 모델도 포함' }
    ],
    byokPoints: [
      '동의는 프로바이더별이고, 연결 테스트 자체도 이 동의가 있어야 통과합니다',
      'API 키는 OS의 시크릿 저장소로 들어갑니다. iPhone·iPad에서는 키체인이고, 거기서는 API 키 프로바이더를 씁니다. Linux는 저장소가 없어서 ChatGPT 로그인과 Claude Code만 됩니다',
      'Cmd/Ctrl+J로 엽니다 — 외부 클라이언트가 쓰는 것과 같은 MCP 계층으로 Linetta의 툴에 접속합니다',
      '모든 호출은 같은 활동 로그에 남고, 어느 에이전트가 호출했는지 표시됩니다',
      '구조 변경과 에이전트가 쓴 씬은 해당 줄의 되돌리기 버튼으로 한 번에 되돌립니다(앱이 켜져 있는 동안, 최근 8건까지). 이전 본문은 기한 없이 버전으로도 남아 복원할 수 있습니다',
      '매 턴 시작에 짧은 기억 두 개 — 작가 프로필과 이 작품의 노트 — 를 읽고, 스스로 거기에 기록합니다. 작가 프로필은 전역이어서 열려 있는 작품에 한정되지 않습니다. 둘 다 설정 → 기억에서 직접 고쳐 쓸 수 있습니다',
      '스킬도 함께 쌓습니다. 방법을 적어 두는 문서이고, 데이터 폴더 아래 평범한 SKILL.md 파일이라 아무 편집기로나 열 수 있고 자기 Claude Code를 그 폴더에 붙일 수도 있습니다. 모든 작품에 적용할 수도, 한 작품에만 묶을 수도 있습니다. 에이전트는 묻지 않고 씁니다. 작가는 대신 작성자 표시와 버전 기록과 끄는 스위치를 설정 → 스킬에서 받습니다',
      '툴을 8번 이상 호출한 턴이 끝나고 답장이 나간 뒤, 같은 프로바이더와 같은 모델에 한 번 더 묻습니다 — 적어 둘 기법이 있었는지. 프로바이더에 그만큼 과금되는 추가 호출이며, 기본은 켜짐이고 설정 → 스킬에서 끕니다. Claude Code 프로바이더에서는 실행되지 않습니다',
      '한 턴은 기본 48번의 툴 호출에서 멈추고, 끝낸 일과 남은 일을 말합니다. 한도는 설정 → 툴 예산에서 8~200으로 바꿉니다',
      '대화 기억은 프로바이더에 맞춰 정해집니다 — Claude Code는 약 10만 자, 나머지는 4만 자 — 설정에서 2만~20만 자 사이로 바꿀 수 있습니다',
      '매일 도는 백업은 데이터베이스만 담습니다. 스킬의 버전 기록은 담기고, 스킬 폴더 자체는 담기지 않습니다'
    ]
  },

  data: {
    mark: '§ 08 — 내 데이터',
    heading: '원고는 내가 가진 파일입니다.',
    sub: '집필에는 계정도 서버도 필요 없습니다. 쓴 것은 이 기기에, 이 앱 없이도 읽을 수 있는 형식으로 남고, 직접 켠 에이전트·게시·동기화를 통해서만 밖으로 나갑니다.',
    cards: [
      {
        tag: 'library.db',
        title: 'SQLite 라이브러리 하나',
        body: '작품, 씬, 아웃라인, 엔티티, 관계, 플롯, 버전 스냅샷, 에이전트가 읽는 기억, 그리고 모든 스킬의 버전 기록이 단일 데이터베이스 파일 안에 있습니다.'
      },
      {
        tag: '스냅샷',
        title: '사라지지 않고 솎아지는 이력',
        body: '수동 스냅샷은 기한 없이 보관됩니다. 자동 저장 스냅샷은 첫날 매 저장에서 30일 뒤 하루 하나로 솎아집니다.'
      },
      {
        tag: 'backups/',
        title: '검증된 일일 백업',
        body: '일일 백업과 마이그레이션 전 백업을 14일 보관하고, 시작 시 복원할 수 있습니다. 담는 것은 데이터베이스뿐이고, 그 옆의 스킬 폴더는 담기지 않습니다.'
      },
      {
        tag: '마크다운',
        title: '자유롭게 들어오고 나가기',
        body: '기존 원고를 가져오고, 씬을 마크다운으로 폴더에 내보내고, 원하면 Git이 그 폴더를 다른 곳으로 옮깁니다. 삽화도 함께 나갑니다.'
      }
    ],
    pathsLabel: '저장 위치',
    paths: [
      { os: 'macOS', path: '~/Library/Application Support/com.devlikebear.linetta' },
      { os: 'Linux', path: '${XDG_DATA_HOME:-~/.local/share}/com.devlikebear.linetta' },
      { os: 'Windows', path: '%APPDATA%\\com.devlikebear.linetta' }
    ]
  },

  download: {
    mark: '§ 09 — 받기',
    heading: 'App Store에서 무료 — Mac, iPhone, iPad.\n나머지 플랫폼은 서명된 빌드로.',
    sub: 'macOS 직접 배포 빌드는 Apple Developer ID로 서명되고 공증되었습니다. Windows와 Linux 설치 파일은 모든 GitHub 릴리스에 함께 올라갑니다.',
    versionLabel: '현재 릴리스',
    version: `v${VERSION}`,
    channels: [
      {
        id: 'mas',
        label: 'App Store — Mac, iPhone, iPad',
        note: '무료. Apple Silicon Mac에 가장 간단히 설치하는 방법이고, iPhone과 iPad는 여기서 받습니다.',
        cta: { label: 'App Store 열기', href: MAS }
      },
      {
        id: 'brew',
        label: 'Homebrew — Apple Silicon',
        note: '서명·공증된 빌드를 탭으로 갱신합니다.',
        code: 'brew install --cask devlikebear/tap/linetta'
      },
      {
        id: 'windows',
        label: 'Windows',
        note: '모든 릴리스에 NSIS와 MSI 설치 파일이 있습니다.',
        cta: { label: '최신 릴리스', href: RELEASES }
      },
      {
        id: 'linux',
        label: 'Linux',
        note: '모든 릴리스에 AppImage, .deb, .rpm 패키지가 있습니다.',
        cta: { label: '최신 릴리스', href: RELEASES }
      }
    ],
    sourceNote:
      'Intel Mac 사용자와 직접 빌드하고 싶은 사람은 소스에서 빌드할 수 있습니다. Tauri 2 Rust 셸, React와 Vite, 내장 Go 엔진, SQLite로 되어 있습니다.'
  },

  faq: {
    mark: '§ 10 — 질문',
    heading: '내려받기 전에.',
    items: [
      {
        q: '계정이나 구독이 필요한가요?',
        a: '구독은 없습니다. 집필, 정리, 가져오기·내보내기, 스냅샷, 백업은 모두 계정 없이 동작합니다. Linetta Works 계정은 게시하거나 기기 사이에 동기화할 때만 필요합니다.'
      },
      {
        q: '앱 안에 AI가 들어 있나요?',
        a: '선택적으로요. 프로바이더 — ChatGPT 로그인, Claude Code CLI, Anthropic, Google Gemini, 또는 OpenAI 호환 엔드포인트 — 를 연결하고 각각 동의하면 내장 에이전트가 대신 씁니다. 아니면 그것 없이 Claude Code나 Codex CLI 같은 에이전트를 MCP 서버로 직접 연결할 수도 있습니다. 삽화는 따로 고르고 따로 동의합니다.'
      },
      {
        q: '쓰던 원고를 가져올 수 있나요?',
        a: '네. 마크다운 가져오기와 내보내기가 모두 되므로, 다른 도구에서 들어오고 다른 도구로 나갈 수 있습니다.'
      },
      {
        q: '제 글이 어딘가로 업로드되나요?',
        a: '기본값으로는 아닙니다. 무언가를 켰을 때만 이 기기를 벗어납니다 — 내장 에이전트는 작업 중인 내용을 연결한 프로바이더로 보내고, 직접 실행한 MCP 클라이언트는 그것을 읽으며, 고른 이미지 프로바이더는 생성을 누를 때 프롬프트와 참조 이미지를 받고, 게시한 회차는 누구나 읽도록 Linetta Works로 가고, Linetta Works 동기화로 올린 작품은 같은 운영자의 서버로 가되 아무도 읽을 수 없게 암호화되어 가며, GitHub 동기화나 폴더 동기화를 설정하면 하루 한 번과 실행할 때마다 전체 서재를 내보내고, 동기화 대상으로 고르면 암호화된 사본도 함께 둡니다.'
      },
      {
        q: 'iPhone, iPad, Android는요?',
        a: 'iPhone과 iPad용 Linetta가 App Store에 있습니다. 같은 작품을 기획하고 쓰며, 내장 에이전트를 API 키(Anthropic, Google Gemini, OpenAI 호환 엔드포인트)로 쓰고, Linetta Works에 게시하고 동기화합니다. iOS 앱은 MCP 서버를 열지 않고, Git·폴더 동기화가 없습니다. Android 앱은 없습니다.'
      },
      {
        q: '라이선스는 무엇인가요?',
        a: 'AGPL-3.0-only 입니다. 소스는 GitHub에 있고, 상용 라이선스 옵션은 라이선스 고지에 적혀 있습니다.'
      }
    ]
  },

  footer: {
    tagline: '장편 소설을 위한 조용한 집필 스튜디오. Linetta Works와 이어져 있습니다. 개발자 한 명이 공개적으로 만듭니다.',
    cols: { project: '프로젝트', docs: '문서', get: 'Linetta 받기' },
    links: {
      project: [
        { label: 'GitHub', href: REPO },
        { label: 'Linetta Works', href: WORKS },
        { label: '릴리스', href: `${REPO}/releases` },
        { label: '변경 기록', href: `${REPO}/blob/main/CHANGELOG.md` },
        { label: '이슈', href: `${REPO}/issues` }
      ],
      docs: [
        { label: 'README', href: `${REPO}#readme` },
        { label: '개발 가이드', href: `${REPO}/blob/main/docs/DEVELOPMENT.md` },
        { label: '개인정보 처리방침', href: `${REPO}/blob/main/docs/privacy-policy.md` },
        { label: '라이선스 고지', href: `${REPO}/blob/main/LICENSE-NOTICE.md` }
      ],
      get: [
        { label: 'App Store', href: MAS },
        { label: 'Homebrew 탭', href: 'https://github.com/devlikebear/homebrew-tap' },
        { label: 'Windows · Linux', href: RELEASES }
      ]
    },
    legal: 'AGPL-3.0-only · devlikebear의 독립 프로젝트'
  }
};

export const ja: Translation = {
  locale: 'ja',
  htmlLang: 'ja',
  ogLocale: 'ja_JP',
  path: '/ja',
  alts: alts('ja'),
  pageTitle: 'Linetta — 長編小説を書き、Linetta Works で連載する執筆スタジオ',
  metaDescription:
    'アウトライン、登場人物、プロットの糸、資料、そして物語がまだ返していない約束を記す台帳を原稿のとなりに置き、シーン単位で書きます。ほしいときには内蔵エージェントや MCP でつないだ自分のエージェントが長い連載の連続性を点検し、場面は挿絵になり、話は Linetta Works で読者に届き、作品は Mac・PC・iPhone・iPad の間をついてきます。',

  nav: {
    workspace: 'ワークスペース',
    longform: '長編',
    assist: '執筆支援',
    illustrations: '挿絵',
    publish: '公開',
    agent: 'エージェント',
    data: '自分のデータ',
    download: 'ダウンロード',
    github: 'GitHub',
    cta: 'Linetta を入手',
    skip: '本文へスキップ'
  },

  hero: {
    mark: 'macOS · iPhone · iPad · Windows · Linux — Linetta Works とともに',
    headline: [
      { text: 'いま書いている一文のとなりに、' },
      { nl: true },
      { text: '物語の全部を。', accent: true }
    ],
    sub: 'Linetta は小説とウェブ小説のための静かな執筆スタジオです。アウトライン、登場人物、場所、関係、プロットの糸、調べ物、そして物語がまだ返していない約束を記す台帳が原稿のとなりに並びます。ほしいときにはエージェント — 内蔵のものでも、MCP でつないだ自分のものでも — が長い連載のつじつまを保ち、場面は挿絵になり、話は Linetta Works で読者に届き、作品は机から iPhone までついてきます。どれも任意です。',
    ctaPrimary: 'Linetta を入手',
    ctaSecondary: 'GitHub でソースを見る',
    pills: ['無料、購読なし', '内蔵エージェントか、自分のエージェント', '한국어 · English · 日本語'],
    imageAlt:
      'Linetta のワークスペース。左にアウトラインのレール、中央にシーン冒頭を表示した本文エディタ、右にこのシーンの文字数・目標・あらすじのパネル。'
  },

  intro: {
    mark: '§ 01 — これは何か',
    heading: '執筆は\nプロジェクト管理ではありません。',
    paragraphs: [
      '長編は状態を溜めていきます。第40話にもなれば、第3章で交わした約束も、返していない借りも、誰かのコートの色も、すべてが荷重を受ける柱になります。そしてそのどれも、いま画面にある段落の中にはありません。',
      '多くの道具はそこに手続きを足して答えます。ボード、タグ、ステータス、そして別に世話をする二つ目のアプリ。Linetta は代わりに、物語の記憶を<em>文のとなりに</em>置きます。ショートカット一つの距離に、しかし邪魔にならない位置に。',
      'だから編集画面は静かな本文のページのままです。アウトライン、登場人物、プロットの背骨、資料、履歴は、文が必要とするときに開くレールとパネルであり、必要ないときは閉じます。'
    ]
  },

  workspace: {
    mark: '§ 02 — ワークスペース',
    heading: '作品まるごとが一つの部屋に。',
    sub: 'どのパネルも同じ問いの上に立っています。このシーンは何と食い違ってはいけないのか。',
    panels: [
      {
        id: 'workspace',
        tab: 'ワークスペース',
        kicker: '本文',
        title: '本文一枚、そのとなりのアウトライン、そしてこのシーンの記録。',
        body: 'アウトラインは手の届くところに、文字数は無視できる位置に置いたまま、シーン単位で書きます。右のレールには文を離れずに確かめるものが並びます — 今日書いた文字数、話の分量目標、あらすじ、このシーンが言及したエンティティ。',
        alt: 'ワークスペース。左にアウトラインのレール、中央に本文エディタ、右に442字・話の分量目標・作品概要を示すパネル。',
        points: [
          '部・章・シーン、あるいは巻・話 — 番号とラベルはインターフェースの言語で付きます',
          'シーンごとの分量目標とリアルタイムの文字数、そのとなりに作品の総計',
          'ZEN は部屋を空にして本文とカウンターだけを残します'
        ]
      },
      {
        id: 'story-world',
        tab: 'ストーリーワールド',
        kicker: '創作の世界',
        title: '人物・場所・もの・概念と、その間の関係。',
        body: 'ファクトブックが現実の世界のためのものなら、ストーリーワールドは創作の世界のためのものです。カードごとに役割と要約、結んでいる関係の数が付き、フィルタで一種類だけに絞れます。',
        alt: 'ストーリーワールドのパネル。場所一つ、人物二人、概念一つ、ものが一つ、それぞれ役割と一行の要約、関係の数を伴って並んでいます。',
        points: [
          '主人公・主要な舞台・敵役といった役割と、それらをつなぐ関係',
          '名前・別名・役割・要約で検索',
          '頼めばエージェントがその場で追加でき、原稿から抜き出したものは提案箱で待ちます'
        ]
      },
      {
        id: 'fact-book',
        tab: 'ファクトブック',
        kicker: '資料',
        title: '出典付きのメモを、それが要るシーンのとなりに。',
        body: '何をどこで確かめたかを書けば、Linetta がそのページを取ってきて横に綴じます。カードには確認済みか不確実かの印が付くので、決着していない主張がいつの間にか事実として固まりません。',
        alt: 'ファクトブックのパネル。調査カードごとに確認済みまたは不確実のバッジが付き、出典が併記されています。',
        points: [
          '確認済み・不確実のバッジと、カードに残る出典',
          '現在のシーン、または作品全体に範囲を指定',
          'MCP でつないだエージェントもこれらのカードを読みます'
        ]
      },
      {
        id: 'library',
        tab: 'ライブラリ',
        kicker: '書架',
        title: '一行から一冊まで。',
        body: '作品が複数あることを前提にした書架に並びます。分量と状態が見え、新しい作品を始めることも、書きかけの原稿を Markdown から取り込むこともできます。',
        alt: 'Linetta のライブラリ画面。作品カードが一つ、新規作成・Markdown 取り込み・検索・Linetta Works を見るボタン。',
        points: [
          '新規作成、Markdown 取り込み、全体検索',
          '進行中とアーカイブ',
          'Linetta Works をすぐそばに — 回を連載し、読者が読んでいる作品を見て回れます'
        ]
      }
    ],
    alsoLabel: '同じ部屋の中に',
    also: [
      {
        tag: '文脈編集',
        title: '設定を一度で直す',
        body: '人物・場所・関係を一度直せば、古い設定を抱えたままのシーンを探して一つずつ案内します。言語モデルは一切使いません。'
      },
      {
        tag: 'プロット',
        title: '背骨の上の糸',
        body: 'ストーリーラインとビートがアウトラインを横切って並びます。第3話で張った約束が、後から思い出すものではなく見えるものになります。'
      },
      {
        tag: '履歴',
        title: 'シーンごとのスナップショット',
        body: '手動と自動のスナップショットを復元できます。手動のものと、エージェントが書く前に取られたものは期限なく保管されます。'
      },
      {
        tag: '記録',
        title: '執筆のペース',
        body: '7日平均と、そのペースで換算した週あたりの話数。連続記録も罪悪感もありません。'
      }
    ]
  },

  assist: {
    mark: '§ 03 — 執筆支援',
    heading: '机のそばの、二人目の読者。',
    sub: 'ここにあるものはすべて任意で、つないだエージェントが必要です。つなぎ方は § 07 にあります。エージェントは読み、点検し、提案します。原稿は頼んだときだけ変わります。',
    cards: [
      {
        tag: 'エージェントに聞く',
        title: '選んだ段落について聞く',
        body: '文を選んでエージェントに聞くを選ぶと、その段落が入力欄の上に引用されます。「もっと短く」のような続きの依頼も、どの段落のことか分かっています。質問だけなら、エージェントは何も変えません。'
      },
      {
        tag: '作家の方向性',
        title: '作家の方向性が先です',
        body: 'これからの数話の焦点と、シーンごとの一行の意図。書けるのは作家だけで、どのエージェントも最初に読み、守るべき指示として扱います。三話書き進むと、その焦点がまだ有効かアプリが尋ねます。'
      },
      {
        tag: '連続性チェック',
        title: '根拠を示す指摘',
        body: 'エージェントがシーンをストーリーワールド、ファクトカード、これまでのシーン、作家の方向性と突き合わせます。どの指摘もシーンを正確に引用し、作品に実在するものを指していなければなりません。原稿は変わりません。'
      },
      {
        tag: '提案箱',
        title: '提案は作家を待ちます',
        body: 'エージェントが原稿から抜き出したものは、ここで一つずつ承認、編集、却下を待ちます。同じ対象らしいものは別名にまとめられます。Markdown を取り込んだあとは、トークンの見積もりを先に示してこの方法でストーリーワールドを埋めます。'
      },
      {
        tag: '次の展開予測',
        title: '正史ではない分岐',
        body: 'ありうる続きを2〜5本並べて見ます。前提、話ごとのビート、リスクのバッジが付きます。アウトラインも原稿も変わりません。一つ選ぶとエージェントのブリーフに計画として入り、物語が動けば古いと表示されます。'
      },
      {
        tag: 'メッセージに画像',
        title: '見せて伝える',
        body: '一つのメッセージに画像を4枚まで、1枚5MBまで添付できます。画像はそのメッセージとともにだけモデルへ送られ、エージェントはそのうちの一枚を人物の参照画像として保存できます。Claude Code プロバイダーはテキストのみです。'
      },
      {
        tag: '質問カード',
        title: 'いくつもの決定を一度に',
        body: '内蔵エージェントが決めてほしいことが複数あるときは、最大六つの質問を一枚のカードで尋ねます。質問ごとに選択肢を選ぶか自分で書き、まとめて送ります。開いたカードはアプリを再起動しても残ります。'
      },
      {
        tag: '頼めば整理',
        title: '片づけも、言ったときだけ',
        body: '頼めばエージェントが世界観全体の要素、関係、ファクトカード、糸を直したり消したりし、連続性の指摘をまとめて整理し、提案箱にたまったものを承認・却下します。直近 8 件の変更はアプリを再起動するまで取り消せます。'
      },
      {
        tag: '長い会話',
        title: '推測せず、探して読み返す',
        body: '会話が内蔵エージェントの読める量を超えると、切れた部分を検索し、見つけた箇所の前後を読みます。作業ノートの終わった決定は消さずに保管されて検索でき、会話は消さないまま新しく始められます。'
      }
    ]
  },

  longform: {
    mark: '§ 04 — 長編',
    heading: '第100話でも、第3話を覚えている。',
    sub: '長い連載は、忘れたところから崩れます。Linetta は物語が返すべきものと決まったことを台帳に記し、すべてのブリーフに入れ、新しい話をそれと突き合わせます。点検の多くはエージェントが行い、作家はそのすべてを見られ、原稿を勝手に書き換えるものはありません。',
    cards: [
      {
        tag: '作品台帳',
        title: '物語が返すべきもの',
        body: '約束、繰り返しのギャグ、開いた問い、決定、数字、時系列、誰が何を知っているか、削る予定まで — それぞれ未回収、回収、破棄で示されます。回収にはそれを果たす一文が要ります。エージェントは書きながら記録し、まだ開いているものを見て書きます。'
      },
      {
        tag: '作品のルール',
        title: '動かしてはいけないものは固定',
        body: '決まった事実を固定すると、すべての話のブリーフにルールとして入り、エージェントは変更も削除もできません。事実には対象、固定された内容、有効な話を書けて、違う値を記録しようとすると上書きせず衝突を知らせます。'
      },
      {
        tag: 'アークと部',
        title: '範囲のある約束',
        body: '約束はアーク、巻、作品全体のどこまで有効かを決められ、その範囲を過ぎると期限が来ていない限りブリーフから外れます。部ごとに役割、終わりまでに成り立つべきこと、目標の緊張度を置き、ブリーフはいまどのアークにいるかを示します。'
      },
      {
        tag: 'シーンの計画',
        title: '要約のとなりに計画',
        body: 'シーンごとに、書いた内容の要約とは別に計画を持ちます。ブリーフはそれをいま書くシーンの計画として渡します。'
      },
      {
        tag: '独立検収',
        title: '一話ごとに二人目の読者',
        body: 'オンにすると、エージェントがいま書いたものを、書いた文脈を持たない別の読み取り専用の呼び出しが読み直し、見つけたことを連続性の指摘に残します。既定はオフです。エージェントパネルは、この話の検収と部全体の講評もワンタップの続きとして出します。'
      },
      {
        tag: 'ターンのあとで',
        title: '整理して、影響を探す',
        body: '選べる二つの工程です。一つは書き終えた話が変えたことを台帳に整理し、もう一つは変わった決定ともう合わない前の話を探します。どちらも既定はオフで、原稿をプロバイダーに再送します。'
      },
      {
        tag: 'サンプル照合',
        title: '二つの話を並べて',
        body: '連続性の点検が、会話なしで話を二つずつ突き合わせます。最初の話、直前の二話、関連する話を選び、食い違いだけを記録します。'
      },
      {
        tag: '長編の文体',
        title: '何話も重ねて見える癖',
        body: 'シーンを一つ選んで作品の文体の基準にします。文体チェックは「AではなくB」型の対句、見せずに説明する文、話をまたいで繰り返される文、いつも同じ終わり方をする話、そして避けると決めた表現を探します。'
      },
      {
        tag: '分量と複製',
        title: '連載先の数え方で',
        body: '文字数を空白込みで数えるか除いて数えるかを作品ごとに選ぶと、エディタ、アウトライン、公開ダイアログ、エージェントのブリーフがすべて従います。プリセットはその基準と話の目標をまとめて決めます。大きな変更は作品を複製してコピーで試せます。'
      }
    ]
  },

  illustrations: {
    mark: '§ 05 — 挿絵',
    heading: '人物がぶれない絵を。',
    sub: '設定 → 画像生成で ChatGPT か Gemini を選ぶまでオフで、同意も別に求めます。ChatGPT はプランの利用量から差し引かれ、Gemini は Google AI が枚数で課金します。生成を押すまで何も描かれません。',
    cards: [
      {
        tag: 'この場面で挿絵を作る',
        title: '段落から絵へ',
        body: '段落を選んで場面を説明し、人物を選び、一枚か4コマかを決めます。プロンプトは場面、ビジュアルシート、画風から組み立てられ、自分で編集でき、編集したとおりに送られます。'
      },
      {
        tag: '人物シート',
        title: '顔ごとに一枚のシート',
        body: '人物ごとに外見のメモと参照画像を最大8枚。画像をドロップするか、第1話の挿絵でうまくいった顔のようにギャラリーから取り込めます。'
      },
      {
        tag: '以前の挿絵',
        title: '第40話でも同じ顔で',
        body: '参照画像のあとに以前の挿絵を2枚まで送ります。同じ人物の絵が優先です。サムネイルで表示され、ワンクリックで外せます。'
      },
      {
        tag: 'エージェントの依頼',
        title: 'エージェントは描けません',
        body: 'プロンプト、比率、キャプションを載せた依頼カードを残すだけで、作家が編集できます。生成を押すまで、画像プロバイダーには何も送られません。'
      },
      {
        tag: 'ギャラリー',
        title: '一作品につき200枚まで',
        body: '絵ごとにプロンプトとモデルが残ります。シーンに挿入したり、アウトラインでシーン・話・部を代表させたりできます。'
      },
      {
        tag: '作品の行く先々へ',
        title: '絵は本文とともに',
        body: '挿絵はライブラリのバックアップ、Markdown の書き出し、フォルダ・Git 同期に含まれ、公開する話とともにアップロードされます。'
      }
    ]
  },

  publish: {
    mark: '§ 06 — 公開',
    heading: '読者には、望むときに。',
    sub: 'Linetta Works は Linetta で書かれた小説を読む任意のサイトで、Linetta の開発者が運営しています。アカウントをつなぐまで何も送られず、つないだあとも公開や同期を選んだものだけが送られます。App Store の Mac・iPhone・iPad アプリを含め、すべてのビルドで公開できます。',
    link: { label: 'Linetta Works を見る', href: WORKS },
    cards: [
      {
        tag: '話',
        title: '一話ずつ',
        body: 'アウトラインやコマンドパレットから公開します。話ごとにアドレスが付き、本文が変わるとアウトラインに編集済みと表示され、読者はもう一度公開するまで前の本文を読みます。公開ダイアログは未解決の重大な連続性の指摘を先に示します。つないだあとは、頼めばエージェントも公開できます。'
      },
      {
        tag: 'シートと表紙',
        title: '世界観も、望むなら',
        body: '人物・世界観のシートは一つずつオンにするまで非公開なので、先にネタバレを確かめられます。ギャラリーから絵を選んでそのまま表紙にするか、エージェントに任せられ、表紙の審査状況もそこに表示されます。メモは公開では送られません。'
      },
      {
        tag: '共有',
        title: '知らせる',
        body: 'X、Threads、Naver ブログはタイトルとリンクを入れた状態で開きます。カカオトークと Instagram は貼り付けられるようコピーします。'
      },
      {
        tag: '送られるもの',
        title: '公開され、審査されます',
        body: '公開した本文と挿絵、選んだシートは公開され、サイトが内容審査のため Anthropic API へ送ります。公開していない話、メモ、ファクトカード、エージェントの記憶は公開では送られません。'
      },
      {
        tag: 'アプリの中で',
        title: '読者が読んでいる作品を見る',
        body: '書庫からアプリの中で Linetta Works を開きます。読者に見える作品を探し、ジャンルで絞り込み、選んだ作品をサイトで開きます。見て回るのにアカウントは要らず、あなたのものは何も送りません。'
      },
      {
        tag: '同期',
        title: 'デバイスの間で作品を運ぶ',
        body: '運営者が許可したアカウントなら、Linetta Works 同期が作品をこの端末で暗号化して預け、別のコンピュータや iPhone・iPad で続きを受け取ります。原稿、設定、画像、エージェントとの会話、グローバルなスキルと作家プロフィールまで運びます。自動同期をオンにすると編集を止めて数分後に送られ、運ぶデータの種類を選べ、二つの端末で同じ作品を直したときは衝突画面が両方の原稿を残します。Mac App Store ビルドにはありません。無料ベータなので、唯一のコピーにはしないでください。'
      }
    ]
  },

  agent: {
    mark: '§ 07 — エージェント',
    heading: '自分のエージェントを連れてくるか、\n自分の鍵をつなぐか。',
    lead: 'Linetta 自体にはモデルも購読もありません。すでに使っているエージェント — Claude Code、Claude Desktop、Codex CLI、Gemini CLI — を MCP でつなげば、すでにある購読で動き、Linetta には資格情報を一切渡しません。あるいは下の内蔵エージェントをオンにして、自分でつないだプロバイダーを使うこともできます。',
    paragraphs: [
      '一般的なファイルシステム MCP サーバーでも Markdown の読み書きはできます。できないのは<em>一つのシーンのためのブリーフ</em>を組み立てることです。そのシーンがアウトラインのどこにあるか、上位階層の要約、直前のシーンの要約、人物と関係のブリーフ、プロットの背骨、ファクトカード、メモリ、そして文体と分量の目標。',
      'そのブリーフこそ Linetta が渡すものであり、アプリの外で書かれた草稿が第14話と矛盾せずに戻ってくる理由です。机と書類棚と拒否権は、作家が持ったままです。'
    ],
    commandLabel: 'Claude Code — 端末に一行',
    command: MCP_COMMAND,
    commandNote:
      '設定が Claude Code、Claude Desktop、Codex CLI、Gemini CLI を見つけると、<code>linetta-mcp</code> stdio ブリッジ経由でワンクリックでつなげます。同梱するビルドとしないビルドがあり、どちらかは設定画面が教えます。トークンは MCP をオンにしたときに発行され、設定から再発行・失効できます。',
    modesLabel: 'アクセス',
    modes: [
      { name: 'オフ', body: '既定値です。何も待ち受けず、何も公開されません。' },
      {
        name: '読み取り専用',
        body: '読み取りツール19個。書き込みツールは登録すらされないので、tools/list に現れず、呼び出せません。'
      },
      {
        name: 'フル',
        body: '46個すべて。シーンの書き込みは先にスナップショットされ、アウトラインの構造変更は一度の呼び出しで取り消せます。'
      }
    ],
    toolsLabel: 'RPC 百個ではなく、ツール46個',
    toolsNote:
      'クライアントのツール予算は有限で、一覧が伸びるほど選択の精度は落ちます。だからエンジンの RPC を1対1では公開しません。アクセスを一つの作品に限定することもでき、設定 → ツール予算で任意グループごとのコストを見てオフにできます。',
    readLabel: '読み取り',
    writeLabel: '書き込み',
    readTools: [
      { name: 'linetta_get_story_context', note: '一つのシーンのための編まれたブリーフ — 中核のツール' },
      { name: 'linetta_read_scene', note: '本文と content_version' },
      { name: 'linetta_get_outline', note: 'ラベル・種類・状態・分量を伴うツリー' },
      { name: 'linetta_search_manuscript', note: '原稿の全文検索' },
      { name: 'linetta_list_works', note: '作品、題、状態、シーン数' },
      { name: 'linetta_list_characters', note: '人物、場所、もの、概念' },
      { name: 'linetta_where_does_appear', note: '特定のエンティティが登場する全シーン' },
      { name: 'linetta_list_proposals', note: '提案箱で待機中の項目' },
      { name: 'linetta_list_continuity', note: '記録された連続性の指摘とそのID' },
      { name: 'linetta_list_ledger', note: '作品台帳 — 返すべきものと決まったこと、新しい順' },
      { name: 'linetta_find_impact', note: '台帳の項目が触れうる書かれたシーン、物語の順に' },
      { name: 'linetta_get_plot', note: 'ストーリーラインとビート' },
      { name: 'linetta_get_fact_cards', note: '出典付きの調査ノート' },
      { name: 'linetta_get_character_visuals', note: 'キャラクターデザインと参照画像' },
      { name: 'linetta_build_illustration_prompt', note: '挿絵と4コマのプロンプト' },
      { name: 'linetta_list_illustrations', note: 'ギャラリーと最近の依頼の結果' },
      { name: 'linetta_analyze_style', note: '作者の文体ルールに反する箇所を報告' },
      { name: 'linetta_read_skill', note: '記録された手法を一つ、本文まで開く — ブリーフには名前と説明だけが載ります' },
      { name: 'linetta_publish_status', note: 'Linetta Works に公開された内容' }
    ],
    writeTools: [
      { name: 'linetta_create_work', note: '最初のシーンを備えた新しい作品を作成' },
      { name: 'linetta_write_scene', note: '読んだ時点の content_version が必須' },
      { name: 'linetta_revise_scene', note: 'シーン全体を送り直さない部分修正' },
      { name: 'linetta_apply_story_ops', note: '構造化された物語の変更を一括で — 世界観全体の修正と削除を含む' },
      { name: 'linetta_write_summary', note: 'シーン・コンテナの要約と作品のあらすじ' },
      { name: 'linetta_create_checkpoint', note: '大きな改稿の前のラベル付き復元点' },
      { name: 'linetta_undo_last_change', note: 'バッチ単位で一度に取り消し' },
      { name: 'linetta_edit_memory', note: '学んだことを記録 — すべての作品に適用される作家プロフィール、またはこの作品のノート' },
      { name: 'linetta_edit_skill', note: '手法を SKILL.md ファイルとして書く — グローバルにも、一つの作品に紐づけることも。承認の手順はなく、作家は代わりに作成者の表示とバージョン履歴とオフのスイッチを得ます' },
      { name: 'linetta_propose_story_ops', note: '候補を提案箱へ送る — 何も適用されません' },
      { name: 'linetta_accept_proposals', note: '作家の指示で提案箱の項目を承認' },
      { name: 'linetta_reject_proposals', note: '作家の指示で提案箱の項目を却下' },
      { name: 'linetta_report_continuity', note: '連続性の指摘を正確な引用とともに記録' },
      { name: 'linetta_update_continuity', note: '作家の指示で指摘をまとめて無視・解決・削除' },
      { name: 'linetta_edit_ledger', note: '台帳の項目を追加・変更・削除 — 固定したルールには触れない' },
      { name: 'linetta_set_part_arc', note: '部の役割、目標、目標の緊張度' },
      { name: 'linetta_save_forecast', note: 'ありうる分岐を保存 — どれも正史ではありません' },
      { name: 'linetta_propose_style_profile', note: '作者が承認する文体プロフィールの下書きを提案' },
      { name: 'linetta_set_character_visuals', note: 'キャラクターデザインを保存' },
      { name: 'linetta_set_art_style', note: '画風を保存' },
      { name: 'linetta_add_reference_image', note: '参照画像を追加 — ギャラリーの絵も可' },
      { name: 'linetta_delete_reference_image', note: '参照画像を削除' },
      { name: 'linetta_request_illustration', note: '挿絵を依頼 — 承認まで描かれません' },
      { name: 'linetta_set_representative_illustration', note: 'シーン・話・部を代表する絵を設定または解除' },
      { name: 'linetta_register_work', note: '作品を Linetta Works に登録 — アプリで作者が承認したときだけ' },
      { name: 'linetta_publish_episode', note: '話を公開 — アプリで作者が承認したときだけ' },
      { name: 'linetta_publish_cover', note: 'ギャラリーの絵を表紙に — アプリで作者が承認したときだけ' }
    ],
    safetyLabel: '安全を保つもの',
    safety: [
      '127.0.0.1 のみにバインドし、DNS リバインディングを防ぐ Origin 検査を行います。どの設定でも LAN バインドやトンネルはありません。',
      '32バイトのベアラートークン。シークレットストアのある OS ではそこに、ない場合は 0600 のファイルに保管し、設定から再発行・失効できます。',
      '楽観的バージョン検査。エージェントが書き直しているシーンをあなたが打っていれば、書き込みは黙って適用されず拒否されます。',
      'シーンを書くたびに事前スナップショット、構造を変えるたびにアウトラインの記録。',
      'すべての呼び出しの活動ログ — 時刻、ツール、作品、対象、結果 — を設定に表示します。',
      '毎分120回の呼び出し上限。暴走したエージェントのループはシーン40本ではなく壁にぶつかります。'
    ],
    byokLabel: 'または、内蔵エージェント',
    byokLead: '別のアプリに切り替えるより Linetta の中のパネルで書きたいなら、内蔵エージェントをオンにして自分でプロバイダーをつなぎましょう。',
    byokProviders: [
      { name: 'ChatGPT (Codex)', note: 'ChatGPT アカウントでサインイン — 鍵を持つ必要なし' },
      { name: 'Claude Code', note: 'インストール済みの claude CLI を自分のサインインで実行 — 鍵は不要。Mac App Store 版にはありません' },
      { name: 'Anthropic', note: 'API キーで' },
      { name: 'Google Gemini', note: 'API キーで' },
      { name: 'OpenAI 互換', note: 'API キーで、互換エンドポイントなら何でも — 自分のマシン上のものも含む' }
    ],
    byokPoints: [
      '同意はプロバイダーごとで、接続テスト自体もこの同意がないと通りません',
      'API キーは OS のシークレットストアに入ります。iPhone・iPad ではキーチェーンで、そこでは API キーのプロバイダーを使います。Linux にはストアがないので、ChatGPT サインインと Claude Code だけが使えます',
      'Cmd/Ctrl+J で開きます — 外部クライアントと同じ MCP レイヤーで Linetta のツールに接続します',
      'すべての呼び出しは同じ活動ログに残り、どのエージェントが呼んだか表示されます',
      '構造的な変更とエージェントが書いたシーンは、その行の取り消しボタンで一手で戻せます（アプリを起動している間、直近 8 件まで）。直前の本文は期限なくバージョンとしても残り、復元できます',
      '毎ターンの初めに短い記憶を二つ — 作家プロフィールとこの作品のノート — を読み、自分でもそこに書き込みます。プロフィールはグローバルで、開いている作品に限定されません。どちらも設定 → 記憶で自分で書き直せます',
      'スキルも貯めます。方法を書き留める文書で、データフォルダの下の普通の SKILL.md ファイルなので、どの編集ソフトでも開けますし、自分の Claude Code をそのフォルダに向けることもできます。すべての作品に適用することも、一つの作品に紐づけることもできます。エージェントは尋ねずに書きます。作家は代わりに、作成者の表示とバージョン履歴とオフのスイッチを設定 → スキルで受け取ります',
      'ツールを8回以上呼んだターンが終わり、返信が送られたあとに、同じプロバイダーと同じモデルへもう一度だけ尋ねます — 書き留める価値のある手法があったか。プロバイダーに課金される追加の呼び出しで、既定はオン、設定 → スキルでオフにできます。Claude Code プロバイダーでは実行されません',
      '1ターンは既定で48回のツール呼び出しで止まり、終えたことと残りを伝えます。上限は設定 → ツール予算で8〜200に変更できます',
      '会話の記憶量はプロバイダーに合わせて決まります — Claude Code は約 10 万字、それ以外は 4 万字 — 設定で 2 万〜20 万字の間に変えられます',
      '毎日のバックアップはデータベースだけを含みます。スキルのバージョン履歴は含まれ、スキルのフォルダそのものは含まれません'
    ]
  },

  data: {
    mark: '§ 08 — 自分のデータ',
    heading: '原稿は自分が持つファイルです。',
    sub: '執筆にアカウントもサーバーも要りません。書いたものはこの端末に、このアプリなしでも読める形式で残り、自分でオンにしたエージェント・公開・同期を通してだけ外へ出ます。',
    cards: [
      {
        tag: 'library.db',
        title: '一つの SQLite ライブラリ',
        body: '作品、シーン、アウトライン、エンティティ、関係、プロット、バージョンスナップショット、エージェントが読む記憶、そしてすべてのスキルのバージョン履歴が単一のデータベースファイルの中にあります。'
      },
      {
        tag: 'スナップショット',
        title: '消えずに間引かれる履歴',
        body: '手動スナップショットは期限なく保管されます。自動保存のスナップショットは初日の毎保存から30日後には一日一つへ間引かれます。'
      },
      {
        tag: 'backups/',
        title: '検証済みの日次バックアップ',
        body: '日次バックアップと移行前バックアップを14日保管し、起動時に復元できます。含むのはデータベースだけで、その隣のスキルのフォルダは含みません。'
      },
      {
        tag: 'Markdown',
        title: '自由に出入りする',
        body: '既存の原稿を取り込み、シーンを Markdown でフォルダに書き出し、必要なら Git にそのフォルダを運ばせます。挿絵もいっしょに書き出されます。'
      }
    ],
    pathsLabel: '保存場所',
    paths: [
      { os: 'macOS', path: '~/Library/Application Support/com.devlikebear.linetta' },
      { os: 'Linux', path: '${XDG_DATA_HOME:-~/.local/share}/com.devlikebear.linetta' },
      { os: 'Windows', path: '%APPDATA%\\com.devlikebear.linetta' }
    ]
  },

  download: {
    mark: '§ 09 — 入手する',
    heading: 'App Store では無料 — Mac、iPhone、iPad。\n他のプラットフォームは署名済みビルドで。',
    sub: 'macOS の直接配布ビルドは Apple Developer ID で署名され、公証済みです。Windows と Linux のインストーラーは毎回の GitHub リリースに含まれます。',
    versionLabel: '現在のリリース',
    version: `v${VERSION}`,
    channels: [
      {
        id: 'mas',
        label: 'App Store — Mac、iPhone、iPad',
        note: '無料。Apple Silicon Mac に入れる一番簡単な方法で、iPhone と iPad はここから入手します。',
        cta: { label: 'App Store を開く', href: MAS }
      },
      {
        id: 'brew',
        label: 'Homebrew — Apple Silicon',
        note: '署名・公証済みのビルドを tap で更新します。',
        code: 'brew install --cask devlikebear/tap/linetta'
      },
      {
        id: 'windows',
        label: 'Windows',
        note: '毎回のリリースに NSIS と MSI のインストーラーがあります。',
        cta: { label: '最新リリース', href: RELEASES }
      },
      {
        id: 'linux',
        label: 'Linux',
        note: '毎回のリリースに AppImage、.deb、.rpm があります。',
        cta: { label: '最新リリース', href: RELEASES }
      }
    ],
    sourceNote:
      'Intel Mac の方や自分でビルドしたい方は、ソースからビルドできます。Tauri 2 の Rust シェル、React と Vite、組み込みの Go エンジン、SQLite でできています。'
  },

  faq: {
    mark: '§ 10 — 質問',
    heading: 'ダウンロードの前に。',
    items: [
      {
        q: 'アカウントや購読は必要ですか。',
        a: '購読はありません。執筆、整理、取り込みと書き出し、スナップショット、バックアップはすべてアカウントなしで動きます。Linetta Works のアカウントは、公開するときと端末の間で同期するときだけ必要です。'
      },
      {
        q: 'アプリの中に AI は入っていますか。',
        a: '任意で入っています。プロバイダー — ChatGPT サインイン、Claude Code CLI、Anthropic、Google Gemini、または OpenAI 互換エンドポイント — をつないでそれぞれ同意すれば、内蔵エージェントが代わりに書きます。それをせずに、Claude Code や Codex CLI のようなエージェントを MCP サーバーで直接つなぐこともできます。挿絵は別に選び、別に同意します。'
      },
      {
        q: '書きかけの原稿を持ち込めますか。',
        a: 'はい。Markdown の取り込みと書き出しが両方できるので、他の道具から入り、他の道具へ出ていけます。'
      },
      {
        q: '書いたものはどこかにアップロードされますか。',
        a: '既定ではされません。何かをオンにしたときだけこの端末を離れます — 内蔵エージェントは作業中の内容をつないだプロバイダーへ送り、自分で動かす MCP クライアントはそれを読み、選んだ画像プロバイダーは生成を押したときにプロンプトと参照画像を受け取り、公開した話は誰でも読めるよう Linetta Works へ送られ、Linetta Works 同期でアップロードした作品は同じ運営者のサーバーへ送られますが誰にも読めないよう暗号化されており、GitHub 同期やフォルダ同期を設定すると毎日一回と起動のたびに書庫全体を書き出し、同期先に選ぶと暗号化したコピーも一緒に置きます。'
      },
      {
        q: 'iPhone、iPad、Android はどうですか。',
        a: 'iPhone と iPad 向けの Linetta が App Store にあります。同じ作品を構想して書き、内蔵エージェントを API キー（Anthropic、Google Gemini、OpenAI 互換エンドポイント）で使い、Linetta Works に公開して同期できます。iOS アプリは MCP サーバーを開かず、Git・フォルダ同期はありません。Android アプリはありません。'
      },
      {
        q: 'ライセンスは何ですか。',
        a: 'AGPL-3.0-only です。ソースは GitHub にあり、商用ライセンスの選択肢はライセンス告知に書かれています。'
      }
    ]
  },

  footer: {
    tagline:
      '長編小説のための静かな執筆スタジオ。Linetta Works とつながっています。開発者一人が公開の場で作っています。',
    cols: { project: 'プロジェクト', docs: 'ドキュメント', get: 'Linetta を入手' },
    links: {
      project: [
        { label: 'GitHub', href: REPO },
        { label: 'Linetta Works', href: WORKS },
        { label: 'リリース', href: `${REPO}/releases` },
        { label: '変更履歴', href: `${REPO}/blob/main/CHANGELOG.md` },
        { label: 'Issues', href: `${REPO}/issues` }
      ],
      docs: [
        { label: 'README', href: `${REPO}#readme` },
        { label: '開発ガイド', href: `${REPO}/blob/main/docs/DEVELOPMENT.md` },
        { label: 'プライバシーポリシー', href: `${REPO}/blob/main/docs/privacy-policy.md` },
        { label: 'ライセンス告知', href: `${REPO}/blob/main/LICENSE-NOTICE.md` }
      ],
      get: [
        { label: 'App Store', href: MAS },
        { label: 'Homebrew tap', href: 'https://github.com/devlikebear/homebrew-tap' },
        { label: 'Windows · Linux', href: RELEASES }
      ]
    },
    legal: 'AGPL-3.0-only · devlikebear による独立プロジェクト'
  }
};

export const translations: Record<Locale, Translation> = { en, ko, ja };
