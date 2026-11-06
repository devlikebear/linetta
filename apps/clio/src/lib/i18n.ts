// Korean-only for now. Components read strings through t(key); adding a
// language later means a second catalog object and a picker, not a rewrite.

const ko = {
  "app.name": "Clio",
  "nav.import": "가져오기",
  "nav.voice": "내 말투",

  "engine.checking": "엔진을 확인하는 중…",
  "engine.failedTitle": "엔진을 시작하지 못했어요",
  "engine.retry": "다시 시도",
  "engine.copyDiagnostics": "진단 정보 복사",

  "common.loading": "불러오는 중…",
  "common.error": "문제가 생겼어요: {error}",

  "import.title": "내 글 가져오기",
  "import.intro": "X 보관함(tweets.js), x-posts.json, 또는 ---로 나눈 텍스트 파일을 고르세요. 파일은 이 기기 밖으로 나가지 않아요.",
  "import.file": "파일",
  "import.fileHint": ".json, .js, .txt, .md",
  "import.platform": "플랫폼",
  "import.platform.x": "X",
  "import.platform.threads": "Threads",
  "import.platform.blog": "블로그",
  "import.kind": "기본 종류",
  "import.kind.post": "글",
  "import.kind.reply": "답글",
  "import.kindNote": "JSON 파일에 kind 필드가 있으면 그 값이 우선해요.",
  "import.userId": "X 사용자 id (선택)",
  "import.userIdNote": "tweets.js에서 내가 이어 쓴 스레드를 글로 구분하는 데 써요.",
  "import.previewing": "미리보기를 만드는 중…",
  "import.previewTitle": "미리보기",
  "import.total": "가져올 글 {count}편",
  "import.context.x.post": "X 글",
  "import.context.x.reply": "X 답글",
  "import.context.threads.post": "Threads 글",
  "import.context.threads.reply": "Threads 답글",
  "import.context.blog.post": "블로그 글",
  "import.context.blog.reply": "블로그 답글",
  "import.skippedTitle": "건너뛴 글",
  "import.skip.retweet": "리트윗",
  "import.skip.short": "너무 짧음",
  "import.skip.duplicate": "중복",
  "import.samples": "앞의 5편",
  "import.submit": "가져오기",
  "import.submitting": "가져오는 중…",
  "import.done": "{inserted}편 가져옴, {skipped}편 건너뜀",
  "import.goVoice": "내 말투 보기",
  "import.empty": "가져올 수 있는 글이 없어요.",

  "voice.title": "내 말투",
  "voice.tab.xPost": "X 글",
  "voice.tab.xReply": "X 답글",
  "voice.tab.all": "전체",
  "voice.sample": "표본 {n}편",
  "voice.fallback": "표본이 적어 {context} 기준으로 보여 줌",
  "voice.context.x/post": "X 글",
  "voice.context.x/reply": "X 답글",
  "voice.context.x/*": "X 전체",
  "voice.context.*": "전체",
  "voice.featuresTitle": "특징",
  "voice.col.feature": "항목",
  "voice.col.p10": "P10",
  "voice.col.p50": "P50",
  "voice.col.p90": "P90",
  "voice.feature.length": "길이",
  "voice.feature.lines": "줄 수",
  "voice.feature.segment_length": "문장 길이",
  "voice.feature.punctuated": "마침표 비율",
  "voice.feature.polite": "존댓말",
  "voice.feature.emoji": "이모지",
  "voice.feature.laugh": "ㅋㅋ",
  "voice.feature.cry": "ㅠㅠ",
  "voice.feature.hashtag": "해시태그",
  "voice.feature.ellipsis": "말줄임",
  "voice.feature.exclaim": "느낌표",
  "voice.feature.ai_tells": "AI 표현",
  "voice.politeShare": "존댓말 비율",
  "voice.mixedShare": "존댓말과 반말을 섞는 글",
  "voice.endingsTitle": "자주 쓰는 끝맺음",
  "voice.samplesTitle": "예시 글",
  "voice.sampleMissing": "본문을 불러오지 못한 글 ({id})",
  "voice.empty": "아직 가져온 글이 없어요.",
  "voice.emptyAction": "가져오기로 가기",
} as const;

export type MessageKey = keyof typeof ko;

/** Every key in the catalog; the catalog test uses it. */
export const catalogKeys = Object.keys(ko) as MessageKey[];

/** Looks a string up and fills `{name}` placeholders. A missing key renders as itself. */
export function t(key: MessageKey, vars?: Record<string, string | number>): string {
  const raw: string = (ko as Record<string, string>)[key] ?? key;
  if (!vars) return raw;
  return raw.replace(/\{(\w+)\}/g, (m, name: string) => (name in vars ? String(vars[name]) : m));
}

/** Like t, for keys built at runtime (a skip reason, a context key). Falls back to `fallback`. */
export function tDynamic(key: string, fallback: string): string {
  const raw = (ko as Record<string, string>)[key];
  return raw ?? fallback;
}
