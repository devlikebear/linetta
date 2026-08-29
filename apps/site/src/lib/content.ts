export type Locale = 'en' | 'ko' | 'ja';

export type AltLink = { locale: Locale; path: string; label: string };

/** A headline is a list of runs so a locale can put the accent and the line
 *  break where its own grammar wants them. */
export type HeadlineRun = { text?: string; accent?: boolean; nl?: boolean };

export type PanelMockId = 'outline' | 'editor' | 'factbook' | 'contextual' | 'plot';

export type Panel = {
  id: PanelMockId;
  tab: string;
  kicker: string;
  title: string;
  body: string;
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
    mock: {
      window: string;
      breadcrumb: string[];
      rail: string;
      railItems: { label: string; meta: string; active?: boolean }[];
      kicker: string;
      title: string;
      prose: string[];
      status: string;
      badge: string;
    };
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

const REPO = 'https://github.com/devlikebear/linetta';
const MAS = 'https://apps.apple.com/app/id6781664781';
const RELEASES = `${REPO}/releases/latest`;
const VERSION = '0.9.6';

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
    mock: {
      window: 'Linetta',
      breadcrumb: ['War of Spaces', 'Arc 1', 'Episode 1'],
      rail: 'Outline',
      railItems: [
        { label: 'Episode 1', meta: '3,180 / 5,000', active: true },
        { label: 'Episode 2', meta: '1,024 / 5,000' },
        { label: 'Episode 3', meta: '— / 5,000' }
      ],
      kicker: 'Arc 1 · Episode 1',
      title: 'Episode 1',
      prose: [
        'The rain went up that night, and every clock in the building agreed to stop at the same minute.',
        'She had not heard that particular knock in eleven years. It was the one her sister used when she wanted to be let in without being announced.'
      ],
      status: 'Episode 1 · 3,180 / 5,000 chars · saved 12s ago',
      badge: 'An external agent can read this work (read only)'
    }
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
        id: 'outline',
        tab: 'Outline',
        kicker: 'Structure',
        title: 'Parts and chapters, or arcs and episodes.',
        body: 'Choose the shape the work actually has. Linetta numbers and labels the tree for you, in the language of the interface — Arc 1 · Episode 1, 제1부 · 1화, 第1巻 · 第1話.',
        points: [
          'Drag to reorder; renumbering follows',
          'Per-scene length targets with a live count',
          'Inspect the outline for duplicates, orphans and depth, then auto-repair — and undo the repair'
        ]
      },
      {
        id: 'editor',
        tab: 'Editor',
        kicker: 'The page',
        title: 'A serif page, and nothing asking for attention.',
        body: 'Scene by scene, with the outline within reach and the character count where you can ignore it. Mentions link a name in the prose to the character behind it.',
        points: [
          'Margin notes attached to the text, not to a sidebar',
          'Full-text search across the manuscript, and a command palette for everything else',
          'ZEN empties the room down to the text and a counter'
        ]
      },
      {
        id: 'factbook',
        tab: 'Fact Book',
        kicker: 'Research',
        title: 'Source-backed notes beside the scene that needs them.',
        body: 'Select a claim in the manuscript and check it. What comes back is saved as a card with its source, next to the writing it belongs to — not in a browser tab you will lose.',
        points: [
          'Cards carry the URL they came from',
          'Kept per work, searchable, reusable across scenes',
          'Fed to an external agent as part of the story brief'
        ]
      },
      {
        id: 'contextual',
        tab: 'Contextual Edit',
        kicker: 'Consistency',
        title: 'Change a fact once, and fix every scene that contradicts it.',
        body: 'A character’s age moves, a place gets renamed, a relationship inverts. Linetta finds the scenes that carry the old version and walks you through the revision, scene by scene.',
        points: [
          'Finds affected scenes from entities, facts and relationships',
          'Batch review before anything is written',
          'No language model involved — this is a search-and-revise tool'
        ]
      },
      {
        id: 'plot',
        tab: 'Plot',
        kicker: 'Threads',
        title: 'A thread you opened in episode 3 should not vanish by 40.',
        body: 'Storylines and beats sit on a spine across the outline, so an unresolved promise is something you can see rather than something you remember at the wrong moment.',
        points: [
          'Storylines with beats placed on scenes',
          'Open and resolved beats at a glance',
          'Register any scene as the start of a new storyline'
        ]
      }
    ],
    alsoLabel: 'Also in the room',
    also: [
      {
        tag: 'History',
        title: 'Snapshots per scene',
        body: 'Manual and automatic snapshots, restorable. Manual ones are kept indefinitely.'
      },
      {
        tag: 'Record',
        title: 'Writing pace',
        body: 'A seven-day average and the number of episodes per week it implies. No streaks, no guilt.'
      },
      {
        tag: 'Entities',
        title: 'Characters, places, things',
        body: 'Briefs and relationships, linked into the prose by mention, and listed by the scenes they appear in.'
      },
      {
        tag: 'Sync',
        title: 'Folder and Git',
        body: 'Export the work as Markdown into a folder, and optionally let Git carry it somewhere else.'
      }
    ]
  },

  agent: {
    mark: '§ 03 — Agents',
    heading: 'Linetta does not call a model.\nYour agent calls Linetta.',
    lead: 'There is no provider setting, no API key field and no token budget inside the app. Linetta opens a local MCP server instead, and the agent you already use — Claude Code, Claude Desktop — connects to it.',
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
        body: 'Nine read tools. The write tools are never registered, so they do not appear in tools/list and cannot be called.'
      },
      {
        name: 'Full',
        body: 'All fifteen. Every write is snapshotted first and can be undone in one step.'
      }
    ],
    toolsLabel: 'Fifteen tools, not a hundred RPCs',
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
      { name: 'linetta_get_fact_cards', note: 'Research notes with their sources' }
    ],
    writeTools: [
      { name: 'linetta_write_scene', note: 'Requires the expected content version' },
      { name: 'linetta_revise_scene', note: 'Partial revision without resending the scene' },
      { name: 'linetta_apply_story_ops', note: 'Structured story changes as one batch' },
      { name: 'linetta_write_summary', note: 'Scene, container and synopsis summaries' },
      { name: 'linetta_create_checkpoint', note: 'A labelled restore point before a big rewrite' },
      { name: 'linetta_undo_last_change', note: 'Undo a batch in one call' }
    ],
    safetyLabel: 'What keeps it safe',
    safety: [
      'Bound to 127.0.0.1 only, with an Origin check against DNS rebinding. No LAN binding and no tunnel, at any setting.',
      'A 32-byte bearer token, held in the operating system’s secret store where there is one and in a 0600 file otherwise, regenerable and revocable from Settings.',
      'Optimistic version checks: if you are typing in the scene an agent is rewriting, the write is refused rather than silently applied.',
      'A snapshot before every scene write, and an outline capture before every structural change.',
      'An activity log of every call — time, tool, work, target, result — shown in Settings.',
      'A cap of 120 tool calls a minute, so a runaway agent loop hits a wall instead of forty rewritten scenes.'
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
        body: 'Projects, scenes, outline, entities, relationships, plot and version snapshots, in a single database file.'
      },
      {
        tag: 'Snapshots',
        title: 'History that thins, not disappears',
        body: 'Manual snapshots are kept indefinitely. Autosave snapshots go from every save on the first day to daily after thirty.'
      },
      {
        tag: 'backups/',
        title: 'Verified daily backups',
        body: 'A daily backup and a pre-migration backup, kept fourteen days, with restore controls at startup.'
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
        a: 'Not any more. Linetta used to carry its own provider settings and an in-app companion; both were removed. AI collaboration now happens through the optional MCP server, using an agent and a subscription you already have.'
      },
      {
        q: 'Can I bring an existing manuscript?',
        a: 'Yes. Markdown import and export both work, so a work can arrive from another tool and leave for one.'
      },
      {
        q: 'Is my writing uploaded anywhere?',
        a: 'No. There is no Linetta cloud. The MCP server is loopback-only and off by default; Git sync talks only to the remote you configure.'
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
    mock: {
      window: 'Linetta',
      breadcrumb: ['공간의 전쟁', '1부', '1화'],
      rail: '아웃라인',
      railItems: [
        { label: '1화', meta: '3,180 / 5,000', active: true },
        { label: '2화', meta: '1,024 / 5,000' },
        { label: '3화', meta: '— / 5,000' }
      ],
      kicker: '1부 · 1화',
      title: '1화',
      prose: [
        '그날 밤 비는 위로 내렸고, 건물 안의 모든 시계가 같은 분에서 멈추기로 합의했다.',
        '그 노크 소리를 들은 건 십일 년 만이었다. 알리지 않고 들어오고 싶을 때 동생이 쓰던 방식이었다.'
      ],
      status: '1화 · 3,180 / 5,000자 · 12초 전 저장됨',
      badge: '외부 에이전트가 이 작품을 읽을 수 있습니다 (읽기 전용)'
    }
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
        id: 'outline',
        tab: '아웃라인',
        kicker: '구조',
        title: '부·장·씬, 또는 권·화.',
        body: '작품이 실제로 가진 형태를 고르면 됩니다. 번호와 라벨은 Linetta가 인터페이스 언어에 맞춰 붙입니다 — 1부 · 1화, Arc 1 · Episode 1, 第1巻 · 第1話.',
        points: [
          '끌어서 순서를 바꾸면 번호가 따라옵니다',
          '씬마다 분량 목표와 실시간 글자 수',
          '중복·고아 노드·과도한 깊이를 점검하고 자동 교정, 그리고 그 교정을 되돌리기'
        ]
      },
      {
        id: 'editor',
        tab: '편집기',
        kicker: '본문',
        title: '본문 한 장, 그리고 주의를 요구하지 않는 나머지.',
        body: '아웃라인은 손 닿는 곳에, 글자 수는 무시할 수 있는 자리에 둔 채 씬 단위로 씁니다. 멘션은 본문 속 이름을 그 인물에 연결합니다.',
        points: [
          '사이드바가 아니라 본문에 붙는 여백 노트',
          '원고 전문 검색, 나머지는 커맨드 팔레트로',
          'ZEN은 방을 비워 본문과 카운터만 남깁니다'
        ]
      },
      {
        id: 'factbook',
        tab: '팩트북',
        kicker: '자료',
        title: '출처가 붙은 메모를, 그 자료가 필요한 씬 옆에.',
        body: '원고에서 확인이 필요한 문장을 선택해 점검합니다. 돌아온 결과는 출처와 함께 카드로 저장되어, 잃어버릴 브라우저 탭이 아니라 해당 원고 옆에 남습니다.',
        points: [
          '카드는 출처 URL을 함께 보관합니다',
          '작품 단위로 쌓이고, 검색되고, 여러 씬에서 재사용됩니다',
          '외부 에이전트에게는 스토리 브리프의 일부로 전달됩니다'
        ]
      },
      {
        id: 'contextual',
        tab: '컨텍스트 편집',
        kicker: '일관성',
        title: '설정을 한 번 고치고, 어긋나는 씬을 전부 고칩니다.',
        body: '인물의 나이가 바뀌고, 장소 이름이 바뀌고, 관계가 뒤집힙니다. Linetta는 옛 설정을 담고 있는 씬들을 찾아 하나씩 수정 과정을 안내합니다.',
        points: [
          '엔티티·팩트·관계에서 영향받는 씬을 찾아냅니다',
          '무엇도 쓰이기 전에 일괄 검토합니다',
          '언어 모델을 전혀 쓰지 않는 검색·수정 도구입니다'
        ]
      },
      {
        id: 'plot',
        tab: '플롯',
        kicker: '스레드',
        title: '3화에서 연 실은 40화까지 사라지면 안 됩니다.',
        body: '스토리라인과 비트가 아웃라인을 가로지르는 스파인 위에 놓입니다. 해소되지 않은 약속이 기억해야 할 것이 아니라 보이는 것이 됩니다.',
        points: [
          '씬에 배치되는 비트를 가진 스토리라인',
          '열린 비트와 해소된 비트를 한눈에',
          '아무 씬이나 새 스토리라인의 시작으로 등록'
        ]
      }
    ],
    alsoLabel: '같은 방 안에',
    also: [
      {
        tag: '이력',
        title: '씬 단위 스냅샷',
        body: '수동·자동 스냅샷을 복원할 수 있습니다. 수동 스냅샷은 기한 없이 보관됩니다.'
      },
      {
        tag: '기록',
        title: '집필 속도',
        body: '7일 평균과 그 속도로 환산한 주당 화수. 연속 기록도, 죄책감도 없습니다.'
      },
      {
        tag: '엔티티',
        title: '인물 · 장소 · 사물',
        body: '설정과 관계를 멘션으로 본문에 연결하고, 등장하는 씬 목록으로 되짚습니다.'
      },
      {
        tag: '동기화',
        title: '폴더와 Git',
        body: '작품을 마크다운으로 폴더에 내보내고, 원하면 Git이 그 폴더를 다른 곳으로 옮기게 합니다.'
      }
    ]
  },

  agent: {
    mark: '§ 03 — 에이전트',
    heading: 'Linetta는 모델을 부르지 않습니다.\n당신의 에이전트가 Linetta를 부릅니다.',
    lead: '앱 안에는 프로바이더 설정도, API 키 입력란도, 토큰 예산도 없습니다. 대신 Linetta가 로컬 MCP 서버를 열고, 이미 쓰고 있는 에이전트 — Claude Code, Claude Desktop — 가 거기에 접속합니다.',
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
        body: '읽기 툴 9개. 쓰기 툴은 아예 등록되지 않아 tools/list에 나타나지도, 호출되지도 않습니다.'
      },
      {
        name: '전체',
        body: '15개 전부. 모든 쓰기는 먼저 스냅샷되고 한 번에 되돌릴 수 있습니다.'
      }
    ],
    toolsLabel: 'RPC 100개가 아니라, 툴 15개',
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
      { name: 'linetta_get_fact_cards', note: '출처가 붙은 조사 노트' }
    ],
    writeTools: [
      { name: 'linetta_write_scene', note: '읽은 시점의 content_version 필수' },
      { name: 'linetta_revise_scene', note: '씬 전체를 다시 보내지 않는 부분 수정' },
      { name: 'linetta_apply_story_ops', note: '구조화된 스토리 변경을 한 배치로' },
      { name: 'linetta_write_summary', note: '씬·컨테이너 요약과 작품 시놉시스' },
      { name: 'linetta_create_checkpoint', note: '큰 개작 전 라벨 붙은 복원 지점' },
      { name: 'linetta_undo_last_change', note: '배치 단위로 한 번에 되돌리기' }
    ],
    safetyLabel: '무엇이 이것을 안전하게 하는가',
    safety: [
      '127.0.0.1에만 바인딩하고 DNS 리바인딩을 막는 Origin 검사를 합니다. 어떤 설정에서도 LAN 바인딩이나 터널은 없습니다.',
      '32바이트 베어러 토큰. 시크릿 저장소가 있는 운영체제에서는 거기에, 없으면 0600 파일에 보관하며, 설정에서 재발급·폐기할 수 있습니다.',
      '낙관적 버전 검사: 에이전트가 고쳐 쓰는 씬을 당신이 타이핑 중이면, 쓰기는 조용히 적용되지 않고 거부됩니다.',
      '씬을 쓰기 전마다 스냅샷, 구조를 바꾸기 전마다 아웃라인 캡처.',
      '모든 호출의 활동 로그 — 시각, 툴, 작품, 대상, 결과 — 를 설정에서 보여줍니다.',
      '분당 120회 호출 상한. 폭주하는 에이전트 루프는 씬 40개가 아니라 벽에 부딪힙니다.'
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
        body: '작품, 씬, 아웃라인, 엔티티, 관계, 플롯, 버전 스냅샷이 단일 데이터베이스 파일 안에 있습니다.'
      },
      {
        tag: '스냅샷',
        title: '사라지지 않고 솎아지는 이력',
        body: '수동 스냅샷은 기한 없이 보관됩니다. 자동 저장 스냅샷은 첫날 매 저장에서 30일 뒤 하루 하나로 솎아집니다.'
      },
      {
        tag: 'backups/',
        title: '검증된 일일 백업',
        body: '일일 백업과 마이그레이션 전 백업을 14일 보관하고, 시작 시 복원할 수 있습니다.'
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
        a: '더는 아닙니다. 예전에는 앱 안에 프로바이더 설정과 컴패니언이 있었지만 둘 다 제거했습니다. 지금 AI 협업은 선택적인 MCP 서버를 통해, 이미 쓰고 있는 에이전트와 구독으로 이루어집니다.'
      },
      {
        q: '쓰던 원고를 가져올 수 있나요?',
        a: '네. 마크다운 가져오기와 내보내기가 모두 되므로, 다른 도구에서 들어오고 다른 도구로 나갈 수 있습니다.'
      },
      {
        q: '제 글이 어딘가로 업로드되나요?',
        a: '아니요. Linetta 클라우드는 존재하지 않습니다. MCP 서버는 루프백 전용이고 기본값은 꺼짐이며, Git 동기화는 직접 설정한 원격에만 연결합니다.'
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
    mock: {
      window: 'Linetta',
      breadcrumb: ['空間の戦争', '第1巻', '第1話'],
      rail: 'アウトライン',
      railItems: [
        { label: '第1話', meta: '3,180 / 5,000', active: true },
        { label: '第2話', meta: '1,024 / 5,000' },
        { label: '第3話', meta: '— / 5,000' }
      ],
      kicker: '第1巻 · 第1話',
      title: '第1話',
      prose: [
        'その夜、雨は上に降り、建物じゅうの時計が同じ分で止まることに同意した。',
        'そのノックを聞いたのは十一年ぶりだった。知らせずに入りたいとき、妹が使っていた叩き方だ。'
      ],
      status: '第1話 · 3,180 / 5,000字 · 12秒前に保存',
      badge: '外部エージェントがこの作品を読めます（読み取り専用）'
    }
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
        id: 'outline',
        tab: 'アウトライン',
        kicker: '構造',
        title: '部・章・シーン、あるいは巻・話。',
        body: '作品が実際に持っている形を選ぶだけです。番号とラベルは Linetta がインターフェースの言語に合わせて付けます — 第1巻 · 第1話、Arc 1 · Episode 1、제1부 · 1화。',
        points: [
          'ドラッグで並べ替えると番号が追従します',
          'シーンごとの分量目標とリアルタイムの文字数',
          '重複・孤立ノード・深すぎる階層を点検して自動修復、その修復も取り消せます'
        ]
      },
      {
        id: 'editor',
        tab: '編集画面',
        kicker: '本文',
        title: '本文一枚と、注意を要求しない残りすべて。',
        body: 'アウトラインは手の届くところに、文字数は無視できる位置に置いたまま、シーン単位で書きます。メンションは本文の名前をその人物に結びます。',
        points: [
          'サイドバーではなく本文に付く余白メモ',
          '原稿の全文検索、残りはコマンドパレットで',
          'ZEN は部屋を空にして本文とカウンターだけを残します'
        ]
      },
      {
        id: 'factbook',
        tab: 'ファクトブック',
        kicker: '資料',
        title: '出典付きのメモを、それが要るシーンのとなりに。',
        body: '原稿の中で確かめたい主張を選んで点検します。返ってきたものは出典とともにカードとして保存され、失くすブラウザのタブではなく、その原稿のとなりに残ります。',
        points: [
          'カードは出典の URL を一緒に保持します',
          '作品ごとに溜まり、検索でき、複数のシーンで再利用できます',
          '外部エージェントにはストーリーブリーフの一部として渡ります'
        ]
      },
      {
        id: 'contextual',
        tab: '文脈編集',
        kicker: '一貫性',
        title: '設定を一度直し、食い違うシーンを全部直します。',
        body: '人物の年齢が動き、場所の名が変わり、関係が反転します。Linetta は古い設定を抱えたシーンを探し出し、一つずつ修正を案内します。',
        points: [
          'エンティティ・ファクト・関係から影響を受けるシーンを見つけます',
          '何も書かれる前に一括で確認します',
          '言語モデルを一切使わない、検索と修正の道具です'
        ]
      },
      {
        id: 'plot',
        tab: 'プロット',
        kicker: '糸',
        title: '第3話で張った糸が、第40話までに消えてはいけません。',
        body: 'ストーリーラインとビートがアウトラインを横切る背骨の上に並びます。解決していない約束が、思い出すものではなく見えるものになります。',
        points: [
          'シーンに置かれたビートを持つストーリーライン',
          '未解決と解決済みのビートが一目で',
          '任意のシーンを新しいストーリーラインの始まりとして登録'
        ]
      }
    ],
    alsoLabel: '同じ部屋の中に',
    also: [
      {
        tag: '履歴',
        title: 'シーンごとのスナップショット',
        body: '手動と自動のスナップショットを復元できます。手動のものは期限なく保管されます。'
      },
      {
        tag: '記録',
        title: '執筆のペース',
        body: '7日平均と、そのペースで換算した週あたりの話数。連続記録も罪悪感もありません。'
      },
      {
        tag: 'エンティティ',
        title: '人物・場所・もの',
        body: '設定と関係をメンションで本文に結び、登場するシーンの一覧から辿れます。'
      },
      {
        tag: '同期',
        title: 'フォルダと Git',
        body: '作品を Markdown でフォルダに書き出し、必要なら Git にそのフォルダを運ばせます。'
      }
    ]
  },

  agent: {
    mark: '§ 03 — エージェント',
    heading: 'Linetta はモデルを呼びません。\nあなたのエージェントが Linetta を呼びます。',
    lead: 'アプリの中にプロバイダー設定も、API キー欄も、トークン予算もありません。代わりに Linetta がローカルの MCP サーバーを開き、すでに使っているエージェント — Claude Code、Claude Desktop — がそこに接続します。',
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
        body: '読み取りツール9個。書き込みツールは登録すらされないので、tools/list に現れず、呼び出せません。'
      },
      {
        name: 'フル',
        body: '15個すべて。すべての書き込みは先にスナップショットされ、一手で取り消せます。'
      }
    ],
    toolsLabel: 'RPC 百個ではなく、ツール15個',
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
      { name: 'linetta_get_fact_cards', note: '出典付きの調査ノート' }
    ],
    writeTools: [
      { name: 'linetta_write_scene', note: '読んだ時点の content_version が必須' },
      { name: 'linetta_revise_scene', note: 'シーン全体を送り直さない部分修正' },
      { name: 'linetta_apply_story_ops', note: '構造化された物語の変更を一括で' },
      { name: 'linetta_write_summary', note: 'シーン・コンテナの要約と作品のあらすじ' },
      { name: 'linetta_create_checkpoint', note: '大きな改稿の前のラベル付き復元点' },
      { name: 'linetta_undo_last_change', note: 'バッチ単位で一度に取り消し' }
    ],
    safetyLabel: '安全を保つもの',
    safety: [
      '127.0.0.1 のみにバインドし、DNS リバインディングを防ぐ Origin 検査を行います。どの設定でも LAN バインドやトンネルはありません。',
      '32バイトのベアラートークン。シークレットストアのある OS ではそこに、ない場合は 0600 のファイルに保管し、設定から再発行・失効できます。',
      '楽観的バージョン検査。エージェントが書き直しているシーンをあなたが打っていれば、書き込みは黙って適用されず拒否されます。',
      'シーンを書くたびに事前スナップショット、構造を変えるたびにアウトラインの記録。',
      'すべての呼び出しの活動ログ — 時刻、ツール、作品、対象、結果 — を設定に表示します。',
      '毎分120回の呼び出し上限。暴走したエージェントのループはシーン40本ではなく壁にぶつかります。'
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
        body: '作品、シーン、アウトライン、エンティティ、関係、プロット、バージョンスナップショットが単一のデータベースファイルの中にあります。'
      },
      {
        tag: 'スナップショット',
        title: '消えずに間引かれる履歴',
        body: '手動スナップショットは期限なく保管されます。自動保存のスナップショットは初日の毎保存から30日後には一日一つへ間引かれます。'
      },
      {
        tag: 'backups/',
        title: '検証済みの日次バックアップ',
        body: '日次バックアップと移行前バックアップを14日保管し、起動時に復元できます。'
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
        a: 'もう入っていません。以前はアプリ内にプロバイダー設定とコンパニオンがありましたが、どちらも取り除きました。いま AI との協働は任意の MCP サーバー経由で、すでに使っているエージェントと購読で行います。'
      },
      {
        q: '書きかけの原稿を持ち込めますか。',
        a: 'はい。Markdown の取り込みと書き出しが両方できるので、他の道具から入り、他の道具へ出ていけます。'
      },
      {
        q: '書いたものはどこかにアップロードされますか。',
        a: 'いいえ。Linetta のクラウドは存在しません。MCP サーバーはループバック専用で既定はオフ、Git 同期は自分で設定したリモートにしかつながりません。'
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
