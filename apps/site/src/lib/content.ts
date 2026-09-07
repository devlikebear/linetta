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
  pageTitle: 'Linetta — a local-first writing studio for long-form fiction',
  metaDescription:
    'Plan a novel, keep it consistent, and write it scene by scene in one quiet workspace. Outline, characters, plot threads and research sit beside the manuscript, in a SQLite file on your own disk. An optional local MCP server lets Claude Code or Claude Desktop work on the manuscript with you.',

  nav: {
    workspace: 'Workspace',
    agent: 'Agents',
    data: 'Your data',
    download: 'Download',
    github: 'GitHub',
    cta: 'Get Linetta',
    skip: 'Skip to content'
  },

  hero: {
    mark: 'macOS · Windows · Linux — local first',
    headline: [
      { text: 'The whole story, ' },
      { text: 'within reach', accent: true },
      { nl: true },
      { text: 'of the sentence you are writing.' }
    ],
    sub: 'Linetta is a calm writing studio for novels and web fiction. Outline, characters, places, relationships, plot threads and research live beside the manuscript — in a SQLite file on your own disk, with no account and no mandatory cloud.',
    ctaPrimary: 'Get Linetta',
    ctaSecondary: 'Source on GitHub',
    pills: [
      'No account, no subscription',
      'The manuscript stays on your disk',
      'Korean, English, Japanese'
    ],
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
          'An agent connected over MCP can add entities here, and you see what it added'
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
        alt: 'The Linetta library screen with a project card, buttons for a new project, Markdown import and search, and a note that core data stays local.',
        points: [
          'New project, Markdown import, and search across everything',
          'Active and archived shelves',
          'Core data stays local — the library is a file on your own disk'
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

  agent: {
    mark: '§ 03 — Agents',
    heading: 'Bring your own agent,\nor your own key.',
    lead: 'Linetta ships no model and no subscription of its own. Connect an agent you already run — Claude Code, Claude Desktop — over MCP, using the subscription you already pay for and no credential to Linetta at all. Or turn on the built-in agent below and give it a provider of your own.',
    paragraphs: [
      'A generic filesystem MCP server can already read and write Markdown. What it cannot assemble is the brief for <em>one scene</em>: where that scene sits in the outline, the summaries above it, the previous scene’s summary, character and relationship briefs, the plot spine, fact cards, memories, and your style and length targets.',
      'That brief is what Linetta hands over, and it is the reason a draft written outside the app comes back without contradicting episode fourteen. The writer keeps the desk, the filing cabinet, and the veto.'
    ],
    commandLabel: 'Claude Code — one line in your terminal',
    command: MCP_COMMAND,
    commandNote:
      'Claude Desktop connects through the <code>linetta-mcp</code> stdio bridge, which some builds ship and others do not — Settings says which, and where to get it otherwise. The token is generated when you turn MCP on and can be regenerated or revoked from Settings.',
    modesLabel: 'Access',
    modes: [
      { name: 'Off', body: 'The default. Nothing listens, and nothing is exposed.' },
      {
        name: 'Read only',
        body: 'Ten read tools. The write tools are never registered, so they do not appear in tools/list and cannot be called.'
      },
      {
        name: 'Full',
        body: 'All nineteen. A scene write is snapshotted first, and an outline restructuring can be undone in one call.'
      }
    ],
    toolsLabel: 'Nineteen tools, not a hundred RPCs',
    toolsNote:
      'A client’s tool budget is finite and selection accuracy falls as the list grows, so the engine’s RPC surface is not exposed one-to-one. Access can also be pinned to a single work.',
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
      { name: 'linetta_get_plot', note: 'Storylines and beats' },
      { name: 'linetta_get_fact_cards', note: 'Research notes with their sources' },
      {
        name: 'linetta_read_skill',
        note: 'Open one recorded how-to in full — the brief lists only names and descriptions'
      }
    ],
    writeTools: [
      { name: 'linetta_create_work', note: 'Start a new work with its first scene ready to draft' },
      { name: 'linetta_write_scene', note: 'Requires the expected content version' },
      { name: 'linetta_revise_scene', note: 'Partial revision without resending the scene' },
      { name: 'linetta_apply_story_ops', note: 'Structured story changes as one batch' },
      { name: 'linetta_write_summary', note: 'Scene, container and synopsis summaries' },
      { name: 'linetta_create_checkpoint', note: 'A labelled restore point before a big rewrite' },
      { name: 'linetta_undo_last_change', note: 'Undo a batch in one call' },
      {
        name: 'linetta_edit_memory',
        note: 'Record what it has learned — the writer profile, which applies to every work, or this work’s notes'
      },
      {
        name: 'linetta_edit_skill',
        note: 'Write a how-to as a SKILL.md file — global, or tied to one work. No approval step; the writer gets attribution, history and an off switch'
      }
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
      { name: 'Anthropic', note: 'By API key' },
      { name: 'Google Gemini', note: 'By API key' },
      { name: 'OpenAI-compatible', note: 'By API key, any compatible endpoint — including one on your own machine' }
    ],
    byokPoints: [
      'Consent is per provider, and it gates even the connection test',
      'An API key goes into your OS’s secure store; Linux has none, so only the ChatGPT sign-in works there',
      'Open it with Cmd/Ctrl+J — it reaches Linetta’s tools through the same MCP layer an external client uses',
      'Every call lands in the same activity log, marked with which agent made it',
      'A structural change gets an Undo button on its line — the last eight, and only while the app stays open; a scene-prose rewrite has no one-click undo yet, but the previous text is kept as a restorable version',
      'It reads two short memories at the start of every turn — a writer profile and this work’s notes — and writes to them itself. The profile is global: it is not scoped to the work you have open. Both are yours to rewrite in Settings → Memory',
      'It also keeps skills — how-to documents about method, as plain SKILL.md files under your data directory, which you can edit in any editor or point your own Claude Code at. A skill can apply to every work or to one. It writes them without asking; you get the author badge, the version history and an off switch, in Settings → Skills',
      'After a turn that ran eight or more tool calls, and once your reply has gone, it asks the same provider and model one more time whether that turn taught it a skill — an extra call your provider meters, on by default, switched off under Settings → Skills',
      'The daily backup is the database only. It carries every skill’s version history and not the skills folder itself'
    ]
  },

  data: {
    mark: '§ 04 — Your data',
    heading: 'The manuscript is a file you own.',
    sub: 'Linetta has no account and no mandatory cloud. Everything below is on your own disk, in formats you can read without it.',
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
        body: 'Import an existing manuscript, export scenes as Markdown into a folder, and optionally let Git carry that folder somewhere else.'
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
    mark: '§ 05 — Get it',
    heading: 'Free on the Mac App Store.\nSigned builds everywhere else.',
    sub: 'The direct macOS build is Developer ID signed and notarised by Apple. Windows and Linux installers are published on every GitHub release.',
    versionLabel: 'Current release',
    version: `v${VERSION}`,
    channels: [
      {
        id: 'mas',
        label: 'Mac App Store',
        note: 'Free. The simplest way onto an Apple Silicon Mac.',
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
    mark: '§ 06 — Questions',
    heading: 'Before you download.',
    items: [
      {
        q: 'Does Linetta need an account or a subscription?',
        a: 'No. Writing, organisation, import and export, snapshots and backups all work without one. There is no Linetta account to create.'
      },
      {
        q: 'Is there AI inside the app?',
        a: 'Optionally. A built-in agent can write for you once you connect a provider — Anthropic, Google Gemini, a ChatGPT sign-in or an OpenAI-compatible endpoint — and consent to it, provider by provider. Or skip that entirely and connect your own agent, such as Claude Code, over the MCP server instead.'
      },
      {
        q: 'Can I bring an existing manuscript?',
        a: 'Yes. Markdown import and export both work, so a work can arrive from another tool and leave for one.'
      },
      {
        q: 'Is my writing uploaded anywhere?',
        a: 'Not by default, and never to a Linetta cloud — there is none. It leaves this device only once you turn something on: the built-in agent sends what it works on to the provider you connected, an MCP client you run reads it, and GitHub Sync or Folder Sync, once configured, export your whole library on a daily tick and at every launch.'
      },
      {
        q: 'What about iPad and Android?',
        a: 'The engine is built for mobile targets and the iPad layout is in progress, but mobile does not host the MCP server. The desktop app is the supported product today.'
      },
      {
        q: 'What licence is it under?',
        a: 'AGPL-3.0-only. The source is on GitHub, and commercial licensing options are described in the licence notice.'
      }
    ]
  },

  footer: {
    tagline:
      'A calm, local-first writing studio for long-form fiction. Built by one developer, in the open.',
    cols: { project: 'Project', docs: 'Documentation', get: 'Get Linetta' },
    links: {
      project: [
        { label: 'GitHub', href: REPO },
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
        { label: 'Mac App Store', href: MAS },
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
  pageTitle: 'Linetta — 장편 소설을 위한 로컬 우선 집필 스튜디오',
  metaDescription:
    '아웃라인, 등장인물, 플롯 스레드, 자료를 원고 옆에 두고 씬 단위로 씁니다. 모든 데이터는 내 디스크의 SQLite 파일에 남고, 계정도 필수 클라우드도 없습니다. 선택적인 로컬 MCP 서버로 Claude Code나 Claude Desktop이 원고 작업을 함께할 수 있습니다.',

  nav: {
    workspace: '작업 공간',
    agent: '에이전트',
    data: '내 데이터',
    download: '내려받기',
    github: 'GitHub',
    cta: 'Linetta 받기',
    skip: '본문으로 건너뛰기'
  },

  hero: {
    mark: 'macOS · Windows · Linux — 로컬 우선',
    headline: [
      { text: '쓰고 있는 문장 옆에,' },
      { nl: true },
      { text: '작품 전체를 ' },
      { text: '펼쳐 둡니다.', accent: true }
    ],
    sub: 'Linetta는 소설과 웹소설을 위한 조용한 집필 스튜디오입니다. 아웃라인, 등장인물, 장소, 관계, 플롯 스레드, 조사 자료가 원고 옆에 놓입니다. 전부 내 디스크의 SQLite 파일 안에 있고, 계정도 필수 클라우드도 없습니다.',
    ctaPrimary: 'Linetta 받기',
    ctaSecondary: 'GitHub에서 소스 보기',
    pills: ['계정도 구독도 없음', '원고는 내 디스크에', '한국어 · English · 日本語'],
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
          'MCP로 연결된 에이전트가 여기에 엔티티를 만들 수 있고, 무엇을 추가했는지 보입니다'
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
        alt: 'Linetta 서재 화면. 작품 카드 하나와 새 작품·마크다운 가져오기·검색 버튼, 그리고 핵심 데이터가 로컬에 남는다는 표시.',
        points: [
          '새 작품, 마크다운 가져오기, 전체 검색',
          '진행 중과 보관함',
          '핵심 데이터는 로컬에 — 서재는 내 디스크의 파일입니다'
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

  agent: {
    mark: '§ 03 — 에이전트',
    heading: '내 에이전트를 데려오거나,\n내 키를 연결하세요.',
    lead: 'Linetta 자체에는 모델도 구독도 없습니다. 이미 쓰고 있는 에이전트 — Claude Code, Claude Desktop — 를 MCP로 연결하면 이미 있는 구독으로 동작하고, Linetta에는 어떤 자격 증명도 넘기지 않습니다. 아니면 아래의 내장 에이전트를 켜고 직접 연결한 프로바이더를 쓸 수도 있습니다.',
    paragraphs: [
      '일반적인 파일시스템 MCP 서버도 마크다운은 읽고 씁니다. 그것이 조립하지 못하는 것은 <em>씬 하나를 위한 브리프</em>입니다. 그 씬이 아웃라인의 어디에 있는지, 상위 계층의 요약, 직전 씬의 요약, 인물과 관계 브리프, 플롯 스파인, 팩트 카드, 메모리, 그리고 문체와 분량 목표.',
      '그 브리프가 Linetta가 건네는 것이고, 앱 바깥에서 쓴 초고가 14화와 모순되지 않고 돌아오는 이유입니다. 책상과 서류함과 거부권은 계속 작가가 쥐고 있습니다.'
    ],
    commandLabel: 'Claude Code — 터미널에 한 줄',
    command: MCP_COMMAND,
    commandNote:
      'Claude Desktop은 <code>linetta-mcp</code> stdio 브리지로 연결합니다. 이 브리지를 함께 배포하는 빌드도, 그렇지 않은 빌드도 있으며 어느 쪽인지는 설정 화면이 알려줍니다. 토큰은 MCP를 켤 때 발급되고 설정에서 재발급·폐기할 수 있습니다.',
    modesLabel: '권한',
    modes: [
      { name: '꺼짐', body: '기본값입니다. 아무것도 열리지 않고, 아무것도 노출되지 않습니다.' },
      {
        name: '읽기 전용',
        body: '읽기 툴 10개. 쓰기 툴은 아예 등록되지 않아 tools/list에 나타나지도, 호출되지도 않습니다.'
      },
      {
        name: '전체',
        body: '19개 전부. 씬 쓰기는 먼저 스냅샷되고, 아웃라인 구조 변경은 한 번의 호출로 되돌릴 수 있습니다.'
      }
    ],
    toolsLabel: 'RPC 100개가 아니라, 툴 19개',
    toolsNote:
      '클라이언트의 툴 예산은 유한하고 목록이 길어질수록 선택 정확도가 떨어집니다. 그래서 엔진 RPC를 1:1로 노출하지 않습니다. 접근을 한 작품으로 제한할 수도 있습니다.',
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
      { name: 'linetta_get_plot', note: '스토리라인과 비트' },
      { name: 'linetta_get_fact_cards', note: '출처가 붙은 조사 노트' },
      { name: 'linetta_read_skill', note: '기록해 둔 기법 하나를 본문까지 열기 — 브리프에는 이름과 설명만 실립니다' }
    ],
    writeTools: [
      { name: 'linetta_create_work', note: '첫 씬이 준비된 새 작품 만들기' },
      { name: 'linetta_write_scene', note: '읽은 시점의 content_version 필수' },
      { name: 'linetta_revise_scene', note: '씬 전체를 다시 보내지 않는 부분 수정' },
      { name: 'linetta_apply_story_ops', note: '구조화된 스토리 변경을 한 배치로' },
      { name: 'linetta_write_summary', note: '씬·컨테이너 요약과 작품 시놉시스' },
      { name: 'linetta_create_checkpoint', note: '큰 개작 전 라벨 붙은 복원 지점' },
      { name: 'linetta_undo_last_change', note: '배치 단위로 한 번에 되돌리기' },
      {
        name: 'linetta_edit_memory',
        note: '알아낸 것을 기록 — 모든 작품에 적용되는 작가 프로필, 또는 이 작품의 노트'
      },
      {
        name: 'linetta_edit_skill',
        note: '기법을 SKILL.md 파일로 기록 — 전역이거나 한 작품에 묶이거나. 승인 절차는 없고, 작가는 대신 작성자 표시와 버전 기록과 끄는 스위치를 받습니다'
      }
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
      { name: 'Anthropic', note: 'API 키로' },
      { name: 'Google Gemini', note: 'API 키로' },
      { name: 'OpenAI 호환', note: 'API 키로, 호환되는 어떤 엔드포인트든 — 내 컴퓨터에서 도는 모델도 포함' }
    ],
    byokPoints: [
      '동의는 프로바이더별이고, 연결 테스트 자체도 이 동의가 있어야 통과합니다',
      'API 키는 OS의 시크릿 저장소로 들어갑니다. Linux는 저장소가 없어서 ChatGPT 로그인만 됩니다',
      'Cmd/Ctrl+J로 엽니다 — 외부 클라이언트가 쓰는 것과 같은 MCP 계층으로 Linetta의 툴에 접속합니다',
      '모든 호출은 같은 활동 로그에 남고, 어느 에이전트가 호출했는지 표시됩니다',
      '구조 변경은 해당 줄의 되돌리기 버튼으로 한 번에 되돌립니다(앱이 켜져 있는 동안, 최근 8건까지). 씬 본문 다시쓰기는 아직 한 번에 되돌릴 수 없지만, 이전 본문이 버전으로 남아 복원할 수 있습니다',
      '매 턴 시작에 짧은 기억 두 개 — 작가 프로필과 이 작품의 노트 — 를 읽고, 스스로 거기에 기록합니다. 작가 프로필은 전역이어서 열려 있는 작품에 한정되지 않습니다. 둘 다 설정 → 기억에서 직접 고쳐 쓸 수 있습니다',
      '스킬도 함께 쌓습니다. 방법을 적어 두는 문서이고, 데이터 폴더 아래 평범한 SKILL.md 파일이라 아무 편집기로나 열 수 있고 자기 Claude Code를 그 폴더에 붙일 수도 있습니다. 모든 작품에 적용할 수도, 한 작품에만 묶을 수도 있습니다. 에이전트는 묻지 않고 씁니다. 작가는 대신 작성자 표시와 버전 기록과 끄는 스위치를 설정 → 스킬에서 받습니다',
      '툴을 8번 이상 호출한 턴이 끝나고 답장이 나간 뒤, 같은 프로바이더와 같은 모델에 한 번 더 묻습니다 — 적어 둘 기법이 있었는지. 프로바이더에 그만큼 과금되는 추가 호출이며, 기본은 켜짐이고 설정 → 스킬에서 끕니다',
      '매일 도는 백업은 데이터베이스만 담습니다. 스킬의 버전 기록은 담기고, 스킬 폴더 자체는 담기지 않습니다'
    ]
  },

  data: {
    mark: '§ 04 — 내 데이터',
    heading: '원고는 내가 가진 파일입니다.',
    sub: 'Linetta에는 계정도 필수 클라우드도 없습니다. 아래는 전부 내 디스크에, 이 앱 없이도 읽을 수 있는 형식으로 남습니다.',
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
        body: '기존 원고를 가져오고, 씬을 마크다운으로 폴더에 내보내고, 원하면 Git이 그 폴더를 다른 곳으로 옮깁니다.'
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
    mark: '§ 05 — 받기',
    heading: 'Mac App Store에서 무료.\n나머지 플랫폼은 서명된 빌드로.',
    sub: 'macOS 직접 배포 빌드는 Apple Developer ID로 서명되고 공증되었습니다. Windows와 Linux 설치 파일은 모든 GitHub 릴리스에 함께 올라갑니다.',
    versionLabel: '현재 릴리스',
    version: `v${VERSION}`,
    channels: [
      {
        id: 'mas',
        label: 'Mac App Store',
        note: '무료. Apple Silicon Mac에 가장 간단히 설치하는 방법입니다.',
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
    mark: '§ 06 — 질문',
    heading: '내려받기 전에.',
    items: [
      {
        q: '계정이나 구독이 필요한가요?',
        a: '아니요. 집필, 정리, 가져오기·내보내기, 스냅샷, 백업 모두 계정 없이 동작합니다. 만들어야 할 Linetta 계정 자체가 없습니다.'
      },
      {
        q: '앱 안에 AI가 들어 있나요?',
        a: '선택적으로요. 프로바이더 — Anthropic, Google Gemini, ChatGPT 로그인, 또는 OpenAI 호환 엔드포인트 — 를 연결하고 각각 동의하면 내장 에이전트가 대신 씁니다. 아니면 그것 없이 Claude Code 같은 에이전트를 MCP 서버로 직접 연결할 수도 있습니다.'
      },
      {
        q: '쓰던 원고를 가져올 수 있나요?',
        a: '네. 마크다운 가져오기와 내보내기가 모두 되므로, 다른 도구에서 들어오고 다른 도구로 나갈 수 있습니다.'
      },
      {
        q: '제 글이 어딘가로 업로드되나요?',
        a: '기본값으로는 아니고, Linetta 클라우드는 애초에 없습니다. 무언가를 켰을 때만 이 기기를 벗어납니다 — 내장 에이전트는 작업 중인 내용을 연결한 프로바이더로 보내고, 직접 실행한 MCP 클라이언트는 그것을 읽으며, GitHub 동기화나 폴더 동기화를 설정하면 하루 한 번과 실행할 때마다 전체 서재를 내보냅니다.'
      },
      {
        q: 'iPad나 Android는요?',
        a: '엔진은 모바일 타깃으로도 빌드되고 iPad 레이아웃은 작업 중이지만, 모바일은 MCP 서버를 호스팅하지 않습니다. 오늘 지원되는 제품은 데스크톱 앱입니다.'
      },
      {
        q: '라이선스는 무엇인가요?',
        a: 'AGPL-3.0-only 입니다. 소스는 GitHub에 있고, 상용 라이선스 옵션은 라이선스 고지에 적혀 있습니다.'
      }
    ]
  },

  footer: {
    tagline: '장편 소설을 위한 조용한 로컬 우선 집필 스튜디오. 개발자 한 명이 공개적으로 만듭니다.',
    cols: { project: '프로젝트', docs: '문서', get: 'Linetta 받기' },
    links: {
      project: [
        { label: 'GitHub', href: REPO },
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
        { label: 'Mac App Store', href: MAS },
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
  pageTitle: 'Linetta — 長編小説のためのローカルファースト執筆スタジオ',
  metaDescription:
    'アウトライン、登場人物、プロットの糸、資料を原稿のとなりに置き、シーン単位で書きます。データは自分のディスクの SQLite ファイルに残り、アカウントも必須のクラウドもありません。任意のローカル MCP サーバーで Claude Code や Claude Desktop が原稿作業を手伝えます。',

  nav: {
    workspace: 'ワークスペース',
    agent: 'エージェント',
    data: '自分のデータ',
    download: 'ダウンロード',
    github: 'GitHub',
    cta: 'Linetta を入手',
    skip: '本文へスキップ'
  },

  hero: {
    mark: 'macOS · Windows · Linux — ローカルファースト',
    headline: [
      { text: 'いま書いている一文のとなりに、' },
      { nl: true },
      { text: '物語の全部を。', accent: true }
    ],
    sub: 'Linetta は小説とウェブ小説のための静かな執筆スタジオです。アウトライン、登場人物、場所、関係、プロットの糸、調べ物が原稿のとなりに並びます。すべては自分のディスクの SQLite ファイルの中にあり、アカウントも必須のクラウドもありません。',
    ctaPrimary: 'Linetta を入手',
    ctaSecondary: 'GitHub でソースを見る',
    pills: ['アカウントも購読も不要', '原稿は自分のディスクに', '한국어 · English · 日本語'],
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
          'MCP でつないだエージェントがここにエンティティを作れ、何を足したか見えます'
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
        alt: 'Linetta のライブラリ画面。作品カードが一つ、新規作成・Markdown 取り込み・検索のボタン、そして中核データがローカルに残る旨の表示。',
        points: [
          '新規作成、Markdown 取り込み、全体検索',
          '進行中と保管',
          '中核データはローカルに — ライブラリは自分のディスクのファイルです'
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

  agent: {
    mark: '§ 03 — エージェント',
    heading: '自分のエージェントを連れてくるか、\n自分の鍵をつなぐか。',
    lead: 'Linetta 自体にはモデルも購読もありません。すでに使っているエージェント — Claude Code、Claude Desktop — を MCP でつなげば、すでにある購読で動き、Linetta には資格情報を一切渡しません。あるいは下の内蔵エージェントをオンにして、自分でつないだプロバイダーを使うこともできます。',
    paragraphs: [
      '一般的なファイルシステム MCP サーバーでも Markdown の読み書きはできます。できないのは<em>一つのシーンのためのブリーフ</em>を組み立てることです。そのシーンがアウトラインのどこにあるか、上位階層の要約、直前のシーンの要約、人物と関係のブリーフ、プロットの背骨、ファクトカード、メモリ、そして文体と分量の目標。',
      'そのブリーフこそ Linetta が渡すものであり、アプリの外で書かれた草稿が第14話と矛盾せずに戻ってくる理由です。机と書類棚と拒否権は、作家が持ったままです。'
    ],
    commandLabel: 'Claude Code — 端末に一行',
    command: MCP_COMMAND,
    commandNote:
      'Claude Desktop は <code>linetta-mcp</code> stdio ブリッジ経由でつながります。同梱するビルドとしないビルドがあり、どちらかは設定画面が教えます。トークンは MCP をオンにしたときに発行され、設定から再発行・失効できます。',
    modesLabel: 'アクセス',
    modes: [
      { name: 'オフ', body: '既定値です。何も待ち受けず、何も公開されません。' },
      {
        name: '読み取り専用',
        body: '読み取りツール10個。書き込みツールは登録すらされないので、tools/list に現れず、呼び出せません。'
      },
      {
        name: 'フル',
        body: '19個すべて。シーンの書き込みは先にスナップショットされ、アウトラインの構造変更は一度の呼び出しで取り消せます。'
      }
    ],
    toolsLabel: 'RPC 百個ではなく、ツール19個',
    toolsNote:
      'クライアントのツール予算は有限で、一覧が伸びるほど選択の精度は落ちます。だからエンジンの RPC を1対1では公開しません。アクセスを一つの作品に限定することもできます。',
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
      { name: 'linetta_get_plot', note: 'ストーリーラインとビート' },
      { name: 'linetta_get_fact_cards', note: '出典付きの調査ノート' },
      { name: 'linetta_read_skill', note: '記録された手法を一つ、本文まで開く — ブリーフには名前と説明だけが載ります' }
    ],
    writeTools: [
      { name: 'linetta_create_work', note: '最初のシーンを備えた新しい作品を作成' },
      { name: 'linetta_write_scene', note: '読んだ時点の content_version が必須' },
      { name: 'linetta_revise_scene', note: 'シーン全体を送り直さない部分修正' },
      { name: 'linetta_apply_story_ops', note: '構造化された物語の変更を一括で' },
      { name: 'linetta_write_summary', note: 'シーン・コンテナの要約と作品のあらすじ' },
      { name: 'linetta_create_checkpoint', note: '大きな改稿の前のラベル付き復元点' },
      { name: 'linetta_undo_last_change', note: 'バッチ単位で一度に取り消し' },
      {
        name: 'linetta_edit_memory',
        note: '学んだことを記録 — すべての作品に適用される作家プロフィール、またはこの作品のノート'
      },
      {
        name: 'linetta_edit_skill',
        note: '手法を SKILL.md ファイルとして書く — グローバルにも、一つの作品に紐づけることも。承認の手順はなく、作家は代わりに作成者の表示とバージョン履歴とオフのスイッチを得ます'
      }
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
      { name: 'Anthropic', note: 'API キーで' },
      { name: 'Google Gemini', note: 'API キーで' },
      { name: 'OpenAI 互換', note: 'API キーで、互換エンドポイントなら何でも — 自分のマシン上のものも含む' }
    ],
    byokPoints: [
      '同意はプロバイダーごとで、接続テスト自体もこの同意がないと通りません',
      'API キーは OS のシークレットストアに入ります。Linux にはストアがないので、ChatGPT サインインだけが使えます',
      'Cmd/Ctrl+J で開きます — 外部クライアントと同じ MCP レイヤーで Linetta のツールに接続します',
      'すべての呼び出しは同じ活動ログに残り、どのエージェントが呼んだか表示されます',
      '構造的な変更はその行の取り消しボタンで一手で戻せます（アプリを起動している間、直近 8 件まで）。シーン本文の書き直しにワンクリックの取り消しはまだありませんが、直前の本文はバージョンとして残り復元できます',
      '毎ターンの初めに短い記憶を二つ — 作家プロフィールとこの作品のノート — を読み、自分でもそこに書き込みます。プロフィールはグローバルで、開いている作品に限定されません。どちらも設定 → 記憶で自分で書き直せます',
      'スキルも貯めます。方法を書き留める文書で、データフォルダの下の普通の SKILL.md ファイルなので、どの編集ソフトでも開けますし、自分の Claude Code をそのフォルダに向けることもできます。すべての作品に適用することも、一つの作品に紐づけることもできます。エージェントは尋ねずに書きます。作家は代わりに、作成者の表示とバージョン履歴とオフのスイッチを設定 → スキルで受け取ります',
      'ツールを8回以上呼んだターンが終わり、返信が送られたあとに、同じプロバイダーと同じモデルへもう一度だけ尋ねます — 書き留める価値のある手法があったか。プロバイダーに課金される追加の呼び出しで、既定はオン、設定 → スキルでオフにできます',
      '毎日のバックアップはデータベースだけを含みます。スキルのバージョン履歴は含まれ、スキルのフォルダそのものは含まれません'
    ]
  },

  data: {
    mark: '§ 04 — 自分のデータ',
    heading: '原稿は自分が持つファイルです。',
    sub: 'Linetta にアカウントも必須のクラウドもありません。以下はすべて自分のディスクに、このアプリなしでも読める形式で残ります。',
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
        body: '既存の原稿を取り込み、シーンを Markdown でフォルダに書き出し、必要なら Git にそのフォルダを運ばせます。'
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
    mark: '§ 05 — 入手する',
    heading: 'Mac App Store では無料。\n他のプラットフォームは署名済みビルドで。',
    sub: 'macOS の直接配布ビルドは Apple Developer ID で署名され、公証済みです。Windows と Linux のインストーラーは毎回の GitHub リリースに含まれます。',
    versionLabel: '現在のリリース',
    version: `v${VERSION}`,
    channels: [
      {
        id: 'mas',
        label: 'Mac App Store',
        note: '無料。Apple Silicon Mac に入れる一番簡単な方法です。',
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
    mark: '§ 06 — 質問',
    heading: 'ダウンロードの前に。',
    items: [
      {
        q: 'アカウントや購読は必要ですか。',
        a: 'いいえ。執筆、整理、取り込みと書き出し、スナップショット、バックアップはすべてアカウントなしで動きます。作るべき Linetta アカウント自体がありません。'
      },
      {
        q: 'アプリの中に AI は入っていますか。',
        a: '任意で入っています。プロバイダー — Anthropic、Google Gemini、ChatGPT サインイン、または OpenAI 互換エンドポイント — をつないでそれぞれ同意すれば、内蔵エージェントが代わりに書きます。それをせずに、Claude Code のようなエージェントを MCP サーバーで直接つなぐこともできます。'
      },
      {
        q: '書きかけの原稿を持ち込めますか。',
        a: 'はい。Markdown の取り込みと書き出しが両方できるので、他の道具から入り、他の道具へ出ていけます。'
      },
      {
        q: '書いたものはどこかにアップロードされますか。',
        a: '既定ではされず、Linetta のクラウドはそもそも存在しません。何かをオンにしたときだけこの端末を離れます — 内蔵エージェントは作業中の内容をつないだプロバイダーへ送り、自分で動かす MCP クライアントはそれを読み、GitHub 同期やフォルダ同期を設定すると毎日一回と起動のたびに書庫全体を書き出します。'
      },
      {
        q: 'iPad や Android はどうですか。',
        a: 'エンジンはモバイル向けにもビルドされ、iPad のレイアウトも進行中ですが、モバイルは MCP サーバーをホストしません。今日サポートされている製品はデスクトップアプリです。'
      },
      {
        q: 'ライセンスは何ですか。',
        a: 'AGPL-3.0-only です。ソースは GitHub にあり、商用ライセンスの選択肢はライセンス告知に書かれています。'
      }
    ]
  },

  footer: {
    tagline:
      '長編小説のための静かなローカルファースト執筆スタジオ。開発者一人が公開の場で作っています。',
    cols: { project: 'プロジェクト', docs: 'ドキュメント', get: 'Linetta を入手' },
    links: {
      project: [
        { label: 'GitHub', href: REPO },
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
        { label: 'Mac App Store', href: MAS },
        { label: 'Homebrew tap', href: 'https://github.com/devlikebear/homebrew-tap' },
        { label: 'Windows · Linux', href: RELEASES }
      ]
    },
    legal: 'AGPL-3.0-only · devlikebear による独立プロジェクト'
  }
};

export const translations: Record<Locale, Translation> = { en, ko, ja };
