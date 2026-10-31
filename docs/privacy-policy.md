# Linetta 개인정보 처리방침 / Privacy Policy

_최종 업데이트 / Last updated / 最終更新: 2026-10-31_

---

## 한국어

Linetta("앱")는 장편소설·웹소설 집필을 위한 **로컬 우선(local-first)** 데스크톱 앱입니다. 개발자(Changheon Shin / devlikebear)는 앱을 위한 분석·광고·계정 서버를 운영하지 않으며 사용자 데이터를 수집하지 않습니다. 원고가 기기를 떠나는 경로는 사용자가 직접 설정한 것뿐입니다. 사용자가 연결한 AI 프로바이더로 원고를 보내는 내장 에이전트, 사용자가 고른 이미지 프로바이더로 프롬프트와 참조 이미지를 보내는 삽화 생성, 사용자가 직접 실행하는 외부 MCP 클라이언트, 사용자가 지정한 git 원격으로 푸시하는 GitHub 동기화, 사용자가 지정한 폴더(예: 클라우드 드라이브 폴더)로 복사하는 폴더 동기화, 사용자가 고른 회차와 시트·표지만 웹소설 사이트 Linetta Works에 올리는 게시, 그리고 사용자가 올리기로 고른 작품을 기기에서 암호화해 맡기는 비공개 보관함이며, 아래 3항에서 모두 설명합니다. 이 가운데 게시와 비공개 보관함만 개발자가 운영하는 서버로 보냅니다. 비공개 보관함으로 가는 것은 개발자가 읽을 수 없는 암호문입니다.

### 1. 수집하는 정보
**개발자가 수집하는 정보는 없습니다.** 앱은 다음을 하지 않습니다.
- 앱 자체에는 사용자 계정 또는 로그인이 없습니다. 게시와 비공개 보관함을 쓰려면 Linetta Works 사이트의 계정을 앱에 연결해야 하며, 이는 아래 3.5항과 3.7항에서 설명합니다.
- 분석(analytics)·추적(tracking)·텔레메트리·광고 식별자를 사용하지 않습니다.
- 크래시 리포팅 등으로 사용자 데이터를 개발자 서버로 보내지 않습니다.

### 2. 데이터 저장 위치
사용자가 작성한 원고·프로젝트·설정 등 기본 데이터는 **사용자 기기에 로컬로** 저장됩니다.
- 집필 데이터: 기기의 로컬 데이터베이스
- 에이전트가 "연속성 점검"으로 기록한 관찰(모순·새 사실·확인 필요 항목과 그 상태)도 같은 로컬 데이터베이스(`library.db`)에 저장되며, 원고 자체는 바뀌지 않습니다.
- 에이전트가 `linetta_propose_story_ops`로 올린 제안함 후보(인물·장소·관계·팩트 등)도 같은 로컬 데이터베이스에 저장됩니다. 작가가 Story World의 제안함에서 수락하거나, 작가의 지시를 받은 에이전트가 수락하기 전까지는 Story World에 아무것도 반영되지 않습니다.
- 에이전트가 `linetta_save_forecast`로 저장한 갈래 예측(갈래별 전제·비트·위험 등)도 같은 로컬 데이터베이스에 저장됩니다. 개요·원고·Story World는 바뀌지 않으며, 작가가 고른 갈래만 다음 브리프에 계획으로 들어갑니다.
- 삽화(프롬프트와 프로바이더·모델 정보 포함), 인물 참조 이미지, 에이전트의 삽화 요청도 같은 로컬 데이터베이스에 저장됩니다.
- MCP 접속 토큰, AI 프로바이더 API 키 등 민감 정보: 운영체제의 보안 저장소(macOS 키체인, Windows 자격 증명 관리자). `settings.json`에는 저장하지 않습니다.

개발자는 이 데이터에 접근할 수 없습니다.

Linux에는 보안 저장소 백엔드가 없어 API 키를 저장할 수 없습니다. 따라서 Linux에서는 API 키로 연결하는 프로바이더를 설정할 수 없고, 토큰을 아래 파일에 저장하는 ChatGPT(Codex) 로그인과, Linetta가 자격 증명을 보관하지 않는 Claude Code 프로바이더만 사용할 수 있습니다. MCP 접속 토큰에는 그런 대체 로그인 수단이 없으므로, Linux에서는 보안 저장소 대신 `<앱 데이터>/mcp-token` 파일에 소유자만 읽을 수 있는 권한(0600)으로 저장됩니다. 이 토큰은 아래 3.2항의 로컬 엔드포인트를 통해 원고를 읽을 수 있는 권한을 부여하며, 그 엔드포인트를 전체 접근 모드로 설정해 두었다면 쓰기 권한도 함께 부여합니다.

ChatGPT(Codex)로 로그인하면 접근 토큰과 갱신 토큰이 `<앱 데이터>/codex/auth.json`에 저장됩니다. 이 파일은 소유자만 읽을 수 있는 권한(0600)으로 저장되며, 이는 Codex CLI가 자기 토큰을 보관하는 방식과 같습니다. Linetta는 이 파일을 어디로도 보내지 않으며, OpenAI에 요청을 보낼 때만 사용합니다.

macOS App Store 빌드가 아닌 빌드에서는, 이미 Codex CLI로 로그인해 둔 것을 그대로 인정합니다. `<앱 데이터>/codex/auth.json`이 없고 `~/.codex/auth.json`이 있으면 Linetta가 그 파일을 대신 사용하므로 두 번 로그인하지 않아도 됩니다. macOS App Store 빌드는 샌드박스 컨테이너 밖을 읽을 수 없어 이 파일을 사용하지 않습니다.

접근 토큰이 만료되면 Linetta가 토큰을 갱신하면서 그때 사용 중인 파일을 다시 씁니다. Codex CLI 로그인을 사용하고 있다면 `~/.codex/auth.json`도 여기에 포함됩니다.

설정에서 로그아웃하면 Linetta 자신의 파일인 `<앱 데이터>/codex/auth.json`이 삭제됩니다. `~/.codex/auth.json`에 있는 Codex CLI 로그인은 삭제하지 않으므로, 그쪽은 Codex CLI에서 직접 로그아웃해야 합니다.

macOS App Store 빌드를 포함한 모든 빌드가 이 파일 저장 방식을 사용하며, 이 토큰을 시스템 키체인에 저장하지 않습니다.

### 3. 원고가 기기를 떠나는 경우
원고가 기기를 떠날 수 있는 경로는 아래에 적은 것들입니다. 어느 것도 사용자가 직접 설정하기 전에는 아무것도 보내지 않습니다. 내장 에이전트, 삽화 생성, MCP, 게시, 비공개 보관함은 사용자가 쓸 때만 동작하지만, GitHub 동기화와 폴더 동기화는 한 번 설정하고 나면 스스로 동작합니다.

#### 3.1 내장 에이전트 (사용자가 연결한 프로바이더)
Linetta의 내장 에이전트는 사용자가 프로바이더를 직접 연결하기 전까지 호출할 대상이 없습니다. 지원하는 프로바이더는 다섯입니다. ChatGPT(Codex)는 ChatGPT 계정으로 로그인해 연결하고, Claude Code는 이미 설치해 둔 `claude` CLI를 사용자의 Claude 로그인으로 실행하며(macOS App Store 빌드 제외), Anthropic·Google Gemini·OpenAI 호환 엔드포인트는 API 키로 연결합니다.

- **어디로 가나.** 사용자가 선택한 프로바이더에만 가고, 그 밖의 어디로도 가지 않습니다. Claude Code를 고르면 이 기기의 CLI를 거쳐 Anthropic으로 갑니다. OpenAI 호환 항목은 사용자가 입력한 base URL로 갑니다. 그 주소는 OpenRouter 같은 서비스일 수도 있고 이 기기에서 실행 중인 모델일 수도 있으며, 후자라면 데이터는 기기를 떠나지 않습니다.
- **무엇이 가나.** 에이전트에 입력한 내용, 그 대화의 이전 메시지, 현재 열려 있는 작품과 씬이 갑니다. 여기에 더해, 에이전트가 툴로 읽은 것이 뒤이은 요청과 함께 프로바이더로 갑니다. 툴은 현재 열려 있는 작품에 묶여 있지 않습니다. 서재의 모든 작품을 제목·시놉시스·장르와 함께 목록으로 얻을 수 있고, 그중 어느 작품에서든 어느 씬이든 그 원문과 씬·챕터 요약, 아웃라인, 그 작품 원고 전체를 대상으로 한 검색 결과, 등장인물·플롯·팩트 카드, 그리고 에이전트가 기억해 둔 사실을 읽을 수 있습니다. 기억해 둔 사실은 데이터베이스가 아니라 `<앱 데이터>/<프로젝트 id>/memory/experiences.jsonl`에 있으며, 에이전트는 여기에 새로 기록할 수도 있습니다. 에이전트는 초고를 쓰기 전에 이 맥락을 먼저 읽도록 지시받습니다. 툴이 닿을 수 있는 범위는 설정에서 체크하는 동의 문구가 말하는 것과 같습니다. 여기에 더해, **작가 방향**(지금의 초점과 현재 씬의 의도)이 있다면 그것도 매 요청과 함께 갑니다. 이 문서는 사용자만 쓰며, 에이전트가 쓸 수 있는 툴은 없습니다. 여기에 더해, 짧게 다듬어 둔 문서 두 개가 매 요청과 함께 갑니다. 작가 프로필과, 지금 작업 중인 작품의 노트입니다. 둘 다 시스템 프롬프트에 통째로 담기므로 에이전트가 툴을 하나도 호출하지 않아도 전송되며, 에이전트가 직접 기록할 수도 있고 사용자가 `설정 → 기억`에서 직접 고쳐 쓸 수도 있습니다. 작가 프로필은 전역입니다. 열려 있는 작품에 묶여 있지 않아서, 에이전트가 어느 한 작품에서 기록해 둔 내용이 다른 어느 작품에 대한 요청과도 함께 프로바이더로 갑니다. 작품 노트는 그 작품에만 남습니다. 여기에 더해 **스킬** 목록이 같은 방식으로 실립니다. 스킬은 에이전트가 기법을 적어 둔 문서로, 데이터베이스가 아니라 `<앱 데이터>/skills/<이름>/SKILL.md`(모든 작품)와 `<앱 데이터>/skills/works/<프로젝트 id>/<이름>/SKILL.md`(한 작품) 파일로 있습니다. 매 요청과 함께 가는 것은 켜져 있는 스킬의 **이름과 설명뿐**이며, 본문은 에이전트가 `linetta_read_skill`로 그 스킬을 열 때 그 요청과 함께 갑니다. `skills/<이름>/`에 있는 스킬은 작가 프로필과 마찬가지로 전역이어서, 한 작품에서 적어 둔 것이 다른 어느 작품에 대한 요청과도 함께 갑니다. 에이전트는 `linetta_edit_skill`로 **사용자에게 묻지 않고** 스킬을 쓰고 고치고 지웁니다. 승인 절차는 없습니다. 대신 스킬마다 사용자가 썼는지 에이전트가 썼는지 표시되고, 쓰기마다 되돌릴 수 있는 버전이 남으며, 스위치 하나로 스킬을 꺼서 요청에서 빼둘 수 있습니다. 모두 `설정 → 스킬`에 있습니다.
- **Story World 채우기(원고 추출).** Markdown 원고를 가져온 뒤 "Story World 채우기"를 시작하면, 에이전트는 회차별로 씬을 읽어 인물·장소·관계·팩트 후보를 뽑고 `linetta_propose_story_ops`로 제안함에 올립니다. 이 과정에서도 원고 본문은 연결된 프로바이더로 그대로 전송됩니다 — 3.1항의 "무엇이 가나"와 같은 경로입니다. 시작하기 전에 회차 수·글자 수·어림 입력 토큰을 보여주며, 이는 어림값이고 실제 과금은 프로바이더 쪽 약관을 따릅니다. 제안함에 쌓인 후보는 작가가 수락하거나 에이전트에게 수락을 지시해야만 Story World에 반영됩니다.
- **손이 많이 간 턴 뒤의 추가 호출.** 한 턴에 에이전트가 툴을 8번 이상 호출했고 답장이 이미 나갔다면, 앱은 같은 프로바이더와 같은 모델에 별도 요청을 한 번 더 보내, 그 턴에서 적어 둘 기법이 있었는지 묻습니다. 이 요청에는 원고 본문도 사용자의 메시지도 담기지 않습니다. 담기는 것은 그 턴이 호출한 툴의 **이름 목록**과, 이미 있는 스킬의 이름·설명입니다. 대개 한 번의 왕복으로 끝나고, 스킬을 실제로 쓰는 경우 최대 다섯 번까지 갑니다. 기본값은 켜짐이며 프로바이더는 다른 호출과 똑같이 과금합니다. `설정 → 스킬 → 스스로 익히기`에서 끄면 이 호출을 아예 하지 않습니다.
- **이미지.** 에이전트 메시지에 첨부한 이미지는 그 메시지에 답하는 턴의 요청과 함께 프로바이더로 갑니다. 한 턴에 모델을 여러 번 호출하면 호출마다 다시 실립니다. 이후 턴에는 실리지 않으며, 대화 기록에는 이미지가 아니라 장수만 남습니다. 큰 이미지는 보내기 전에 앱이 줄입니다. 사용자가 요청하면 에이전트는 첨부 이미지를 인물 참조 이미지로 저장할 수 있습니다. 또한 에이전트가 `linetta_get_character_visuals`로 이미지를 함께 요청하면 그 인물의 참조 이미지도 프로바이더로 갑니다. Claude Code 프로바이더는 첨부 이미지를 받지 않습니다.
- **Claude Code 프로바이더.** 이 경우 Linetta는 API를 직접 호출하지 않고 이 기기의 `claude` CLI를 실행합니다. Linetta는 그 CLI의 파일·셸·웹 툴과 사용자의 훅·플러그인·다른 MCP 서버를 쓰지 못하게 막고, Linetta 자신의 툴만 허용합니다. CLI가 이 기기에 남기는 기록과 Anthropic이 데이터를 다루는 방식은 Claude Code의 설정과 약관을 따릅니다.
- **언제 가나.** 동의한 뒤에만 갑니다. 동의는 프로바이더별이며, 한 곳에 동의한 것이 다른 곳에 동의한 것이 되지 않습니다. 같은 체크박스를 해제하면 클릭 한 번으로 동의가 철회되고, 앱은 그 프로바이더로 보내는 것을 즉시 멈춥니다. 이 확인은 앱이 전송할 수 있는 것을 만들기 전에 이루어지므로, 동의가 없으면 "연결 테스트" 버튼도 거절됩니다. (자격 증명을 입력한 뒤에는 앱이 프로바이더에 사용 가능한 모델 목록을 물을 수 있습니다. 이 요청에는 원고 내용이 담기지 않습니다.)
- **자격 증명.** 2항에 적은 대로 보관합니다. API 키는 운영체제의 보안 저장소에, ChatGPT(Codex) 로그인은 `<앱 데이터>/codex/auth.json`에 보관합니다. 자격 증명은 요청의 일부로 해당 프로바이더에만 전송됩니다. Claude Code의 로그인은 CLI가 직접 관리하며 Linetta는 저장하지 않습니다.
- **프로바이더가 그 데이터로 무엇을 하는지**는 그 회사의 개인정보 처리방침과 약관 소관이며, 텍스트를 보관하는지 학습에 사용하는지도 여기에 포함됩니다. Linetta가 그 회사를 대신해 약속할 수 없으니, 선택한 프로바이더의 방침을 직접 확인하시기 바랍니다.
- **에이전트는 원고를 직접 바꿉니다.** 씬을 쓰는 것도, 아웃라인을 재구성하는 것도 포함됩니다. 이 권한은 "외부 에이전트 연결 (MCP)"을 읽기 전용으로 두거나 꺼도 바뀌지 않습니다 — 그 설정은 외부 클라이언트에만 적용됩니다. 에이전트가 호출한 모든 툴은 외부 클라이언트의 호출과 함께 활동 기록에 남고, 어느 쪽이 호출했는지도 기록되어 설정의 MCP 항목에서 확인할 수 있습니다.
- 개발자는 이 데이터를 수신·중계·저장하지 않습니다. 데이터는 사용자의 기기에서 사용자의 프로바이더로 바로 갑니다.

#### 3.2 외부 에이전트 연결 (MCP)
사용자가 `설정 → 외부 에이전트 연결(MCP)`에서 명시적으로 동의하고 켜면, 앱이 `127.0.0.1`에만 바인딩되는 로컬 엔드포인트를 엽니다. 사용자가 자기 기기에서 실행하는 MCP 클라이언트(예: Claude Code, Claude Desktop, Codex CLI, Gemini CLI)가 여기에 접속해 원고·작품 구조·팩트 카드를 읽고, 전체 접근 모드에서는 수정할 수 있습니다.

- 이 경로로 전송되는 데이터의 수신자는 **사용자가 선택한 그 클라이언트**이며, 그 클라이언트가 자기 프로바이더에게 무엇을 보내는지는 해당 제품의 개인정보 처리방침을 따릅니다.
- 엔드포인트는 기본적으로 꺼져 있고, 동의 체크와 활성화를 거쳐야 열립니다. 앱에서 끄면 즉시 닫힙니다.
- 접속에는 앱이 로컬에서 생성한 토큰이 필요하며, 사용자는 언제든 재발급할 수 있습니다.
- 원격 접속은 불가능합니다. 다른 기기에서 이 엔드포인트에 닿을 수 없습니다.
- 연결된 클라이언트는 **작가 방향**(지금의 초점과 현재 씬의 의도)도 스토리 브리프 맨 위에서 읽습니다. 두 모드 모두에서, 읽기 전용 모드도 포함해서 읽습니다. 쓸 수는 없습니다 — 이 문서를 쓰는 MCP 툴은 없으며, 앱 안에서 사용자만 씁니다.
- 연결된 클라이언트는 3.1항의 다듬어 둔 문서 두 개 — 작가 프로필과 그 작품의 노트 — 도 스토리 브리프에서 읽습니다. 두 모드 모두에서 읽고, 전체 접근 모드에서만 `linetta_edit_memory` 툴로 쓸 수 있습니다. 읽기 전용 모드에서는 이 툴이 아예 등록되지 않습니다. 작가 프로필은 전역이어서, 클라이언트가 거기에 기록한 것은 모든 작품에서 읽힙니다. 접근을 한 작품으로 제한해 둔 클라이언트는 그 작품의 노트만 쓸 수 있고, 작가 프로필에 대해서는 거절됩니다.
- 스토리 브리프에는 켜져 있는 **스킬의 이름과 설명**도 함께 실립니다. 두 모드 모두에서 실리고, 본문은 클라이언트가 `linetta_read_skill`로 여는 스킬만 전달됩니다. 이 툴은 읽기 툴이라 읽기 전용 모드에서도 등록됩니다. 스킬을 쓰는 것은 `linetta_edit_skill`이며, 다른 쓰기 툴과 마찬가지로 전체 접근 모드에서만 등록됩니다. `skills/<이름>/`의 스킬은 전역이므로, 접근을 한 작품으로 제한해 둔 클라이언트는 그 작품의 스킬만 쓸 수 있고 전역 스킬에 대해서는 작가 프로필과 같은 이유로 거절됩니다.
- 연결된 클라이언트는 인물 시트와 참조 이미지도 읽을 수 있습니다. 두 모드 모두에서 읽을 수 있고, 참조 이미지는 클라이언트가 `linetta_get_character_visuals`로 이미지를 함께 요청할 때만 전달됩니다.
- 전체 접근 모드의 클라이언트는 삽화를 요청할 수 있지만 직접 그릴 수는 없습니다. 요청은 사용자가 앱에서 생성을 누르기 전에는 이미지 프로바이더로 가지 않습니다(3.6항). 또한 사용자가 계정을 연결했다면 회차를 게시할 수 있습니다(3.5항).
- 에이전트가 수행한 모든 변경은 앱의 활동 기록에 남습니다. 씬 본문은 고쳐 쓰기 전에 스냅샷이 저장되어 이전 본문을 복원할 수 있고, 아웃라인 구조 변경은 한 번에 되돌릴 수 있습니다. 등장인물·관계·스토리라인·비트·팩트 카드·기억의 변경은 기록되지만 되돌리는 기능이 없어, 다시 수정해서 바로잡아야 합니다.
- 목적은 사용자가 요청한 생성·수정·요약·조사 보조를 제공하는 것입니다.
- 개발자는 이 데이터를 수신·중계·저장하지 않습니다.

#### 3.3 GitHub 동기화
GitHub 동기화는 `설정 → GitHub 동기화`에서 사용자가 git 폴더를 지정하기 전까지 꺼져 있습니다. 지정하고 나면 스스로 동작해, 현재 열려 있는 작품만이 아니라 보관하지 않은 **모든** 작품을 마크다운 파일로 그 폴더에 내보내 커밋하고 푸시합니다. 실행 시점은 앱을 켤 때마다, 그리고 앱을 켜 둔 채로 하루가 지날 때마다이며, 첫 내보내기 전에 기다리는 시간은 없습니다. macOS App Store 빌드에는 이 기능이 없고, 그 밖의 모든 데스크톱 빌드에 포함됩니다.

- **어디로 가나.** 그 git 저장소에 설정된 원격으로만 가고, 그 밖의 어디로도 가지 않습니다. 즉 사용자의 git 호스트에 있는 사용자의 저장소이며, 그 호스트의 개인정보 처리방침과 약관을 따릅니다. 선택한 저장소가 비공개인지 확인하시기 바랍니다. 원격이 없는 저장소라면 로컬에 커밋만 되고 아무 데도 푸시되지 않습니다.
- **무엇이 가나.** 서재에 있는 작품 중 보관하지 않은 모든 작품이 원고·아웃라인·설정 자료를 포함해 마크다운으로 내보내집니다.
- **인증**은 SSH 키나 자격 증명 도우미 같은 기존 시스템 git 설정을 그대로 사용합니다. Linetta는 git 자격 증명을 묻지도, 저장하지도 않습니다.
- Linetta가 안내하는 설정이 그 파일을 거기에 두기 때문에, 그 폴더 맨 위에 있는 `.mcp.json`이라는 이름의 파일 하나는 의도적으로 스테이징에서 제외합니다. 제외 대상은 그 파일 하나뿐이어서, `claude_desktop_config.json`이나 하위 폴더의 `.mcp.json` 같은 다른 MCP 클라이언트 설정 파일은 여느 파일과 똑같이 스테이징됩니다. 스테이징하지 않는 것이 원격에 올라가지 않는다는 뜻도 아니어서, 사용자가 이미 커밋해 둔 파일은 다른 것들과 함께 푸시됩니다. Linetta가 제시하는 설정 스니펫에는 토큰이 담기지 않으므로, 이는 직접 작성한 설정 파일에 해당하는 이야기입니다.
- 개발자는 이 데이터를 수신·중계·저장하지 않습니다.

#### 3.4 폴더 동기화
폴더 동기화는 `설정 → 폴더 동기화`에서 사용자가 켜고 대상 폴더를 지정하기 전까지 꺼져 있습니다. 지정하고 나면 스스로 동작해, 앱을 켤 때마다 그리고 앱을 켜 둔 채로 하루가 지날 때마다 보관하지 않은 모든 작품을 마크다운 파일로 내보내 그 폴더에 복사합니다. 그 폴더는 클라우드 드라이브 폴더인 경우가 많고 앱이 보여주는 예시부터 Dropbox 폴더이며, 그렇다면 서재 전체가 그 서비스로 업로드됩니다. macOS App Store 빌드를 포함한 모든 빌드에 이 기능이 있습니다.

- **어디로 가나.** 사용자가 지정한 폴더로만 가고, 그 밖의 어디로도 가지 않습니다. Linetta는 파일을 쓸 뿐 업로드하지 않습니다. 그 폴더가 클라우드 저장소 서비스의 것이라면 그다음은 그 서비스가 자기 서버로 복사하며, 그 회사의 개인정보 처리방침과 약관을 따릅니다. 평범한 로컬 폴더라면 아무것도 기기를 떠나지 않습니다.
- **무엇이 가나.** 서재에 있는 작품 중 보관하지 않은 모든 작품이 원고·아웃라인·설정 자료를 포함해 마크다운으로 내보내집니다. GitHub 동기화가 쓰는 것과 같은 내용입니다.
- **자격 증명은 필요 없습니다.** Linetta가 요구하는 것은 폴더뿐입니다. 거기 놓인 것을 누가 읽을 수 있는지는 사용자가 고른 폴더와, 그 폴더를 동기화하는 서비스가 있다면 그 서비스가 정합니다.
- 개발자는 이 데이터를 수신·중계·저장하지 않습니다.

#### 3.5 Linetta Works 게시
게시는 `설정 → 게시`에서 사용자가 전송 안내에 동의하고 브라우저로 Linetta Works(works.marvin-42.com) 계정을 연결하기 전까지 꺼져 있으며, 그 전에는 앱이 사이트로 아무 요청도 보내지 않습니다. 연결한 뒤에도 스스로 동작하지 않고, 사용자가 회차의 게시 창에서 "게시"를 누르거나 사용자가 연결한 에이전트에게 회차 게시를 시켰을 때만 그 회차를 보냅니다. 시트와 표지는 사용자가 게시 창에서 "시트와 표지 올리기"를 눌렀을 때만 보내며, 에이전트가 올릴 수 없습니다. iOS 빌드에는 이 기능이 없습니다. Linetta Works는 Linetta 개발자가 운영하는 사이트입니다.

- **어디로 가나.** Linetta Works 서버로 갑니다. 게시한 회차와 공개하기로 고른 시트·표지는 누구나 읽을 수 있게 공개됩니다. 사이트는 게시된 본문, 삽화, 시트, 표지를 내용 심사를 위해 Anthropic API로 보냅니다.
- **무엇이 가나.** 연결할 때는 이 기기의 이름(컴퓨터 이름을 넣은 "Linetta (…)")이 사이트의 기기 목록에 표시되도록 전송됩니다. 게시할 때는 그 회차의 제목과 본문, 그 회차에 넣은 삽화, 회차 순서, 작품의 제목·장르·소개글·연재 상태·작품 주소, 앱 언어, 그리고 회차와 작품을 식별하는 Linetta 내부 ID가 전송됩니다. 시트를 올릴 때는 공개로 켠 시트의 이름, 별칭, 역할, 요약, 속성, 공개한 시트끼리의 관계 이름, 시트의 첫 참조 이미지가 전송되고, 고른 표지 이미지가 함께 전송됩니다. 게시하지 않은 회차, 메모, 공개로 켜지 않은 시트, 관계 메모, 비주얼 메모, 팩트 카드, 에이전트 기억은 보내지 않습니다. 요청에는 앱 버전이 함께 실리며, 사이트는 인터넷 요청이 늘 그렇듯 접속 IP 주소를 받습니다.
- **연결 정보.** 사이트가 한 번 내려주는 게시 토큰은 운영체제의 보안 저장소(macOS 키체인, Windows 자격 증명 관리자)에 저장합니다. 보안 저장소가 없는 Linux에서는 `<앱 데이터>/publish-token` 파일에 소유자만 읽을 수 있는 권한(0600)으로 저장합니다. 이 토큰으로 할 수 있는 일은 그 계정의 작품, 회차, 시트, 표지를 올리고 공개 여부를 바꾸는 것과, 비공개 보관함을 쓰는 계정이면 보관함의 암호문을 올리고 받고 지우는 것입니다. 토큰만으로는 암호문을 읽을 수 없습니다(3.7항). 필명, 사이트 주소처럼 비밀이 아닌 연결 정보는 `settings.json`에 저장합니다.
- **해제와 삭제.** `설정 → 게시`의 "연결 해제"는 이 기기에 저장된 연결 정보를 지웁니다. 사이트의 기기 목록에서 이 기기를 없애려면 사이트의 프로필에서 해제하십시오. 이미 올린 회차는 연결을 해제해도 사이트에 남으며, 앱의 게시 창이나 사이트의 내 작품에서 비공개로 돌리거나 삭제할 수 있습니다. 시트는 공개를 끄고 시트와 표지를 다시 올리면 사이트에서도 내려갑니다.
- **공유.** 공개된 회차의 게시 창에 있는 공유 버튼은 사용자가 누를 때만 동작합니다. X, Threads, 네이버 블로그 버튼은 회차 제목과 주소가 채워진 그 서비스의 공유 페이지를 브라우저에서 엽니다. 카카오톡과 인스타그램 버튼은 제목과 주소를 클립보드에 복사합니다. 인스타그램 버튼은 그 사이트도 함께 엽니다. 그 뒤의 일은 해당 서비스의 개인정보 처리방침과 약관을 따릅니다.

#### 3.6 삽화 생성
삽화 생성은 `설정 → 이미지 생성`에서 ChatGPT나 Gemini를 고르고 그 프로바이더에 동의하기 전까지 꺼져 있습니다. 이 동의는 내장 에이전트의 동의와 별개이며, 없으면 앱은 아무것도 보내지 않습니다. 동의한 뒤에도 사용자가 "생성"을 누를 때만 보냅니다. 에이전트는 삽화를 요청할 수만 있고, 그 요청도 사용자가 생성을 누르기 전에는 보내지 않습니다.

- **어디로 가나.** 고른 프로바이더(ChatGPT 로그인이면 OpenAI, Gemini면 Google)에만 갑니다.
- **무엇이 가나.** 사용자가 확인하고 고칠 수 있는 프롬프트(장면 설명, 인물 외형, 화풍), 고른 인물 참조 이미지, 그리고 같은 작품의 이전 삽화가 갑니다. 이전 삽화는 앱이 최대 2장까지 미리 골라 썸네일로 보여주며, 사용자가 빼거나 전혀 보내지 않을 수 있습니다.
- **자격 증명.** AI 프로바이더 설정의 로그인이나 API 키를 그대로 쓰며, 2항에 적은 대로 보관합니다.
- **프로바이더가 그 데이터로 무엇을 하는지**는 그 회사의 개인정보 처리방침과 약관을 따릅니다. 비용도 마찬가지로, ChatGPT는 요금제 사용량에서 차감되고 Gemini는 Google이 과금합니다.
- 개발자는 이 데이터를 수신·중계·저장하지 않습니다. 삽화를 게시한 경우는 3.5항을 따릅니다.

#### 3.7 비공개 보관함
비공개 보관함은 운영자가 허가한 Linetta Works 계정만 쓸 수 있습니다. 사용자가 사이트에서 동의하고 요청해 허가받은 뒤, `설정 → 비공개 보관함`에서 전송 안내에 동의하고 복구 키를 만들거나 입력하기 전까지 앱은 작품을 보내지 않습니다. 설정한 뒤에도 스스로 올리거나 받지 않습니다. 사용자가 작품의 보관함 창에서 "올리기"나 "받기"를 누를 때만 작품을 주고받습니다. 서재나 작품을 열 때는 새 버전이 있는지 서버에 묻기만 합니다.

- **어디로 가나.** 개발자가 운영하는 Linetta Works 서버를 거쳐 저장소(Cloudflare R2)에 저장됩니다. 공개되지 않고, 수위 검사를 위해 Anthropic으로 보내지 않습니다.
- **무엇이 가나.** 올린 작품의 제목과 원고, 아웃라인, 세계관과 스토리 데이터, 버전 스냅샷, 그림, 작품별 스킬과 메모리가 이 기기에서 암호화되어 갑니다. 서버에는 암호문과 그 크기, 시각, 임의로 만든 항목·조각 식별값만 남습니다. 개발자는 내용과 제목을 읽을 수 없습니다. 활동 기록, 전역 작가 프로필과 전역 스킬, 프로바이더 설정과 API 키는 가지 않습니다.
- **복구 키.** 암호화에 쓰는 복구 키는 사이트로 보내지 않습니다. 운영체제의 보안 저장소에 저장하고, 보안 저장소가 없는 Linux에서는 `<앱 데이터>/vault-key` 파일에 소유자만 읽을 수 있는 권한(0600)으로 저장합니다. 보안 저장소가 잠겨 있으면 파일로 넘어가지 않습니다. 사이트 연결을 해제해도 지우지 않습니다. 복구 키를 잃으면 개발자도 작품을 되살릴 수 없으므로, 사용자가 이 기기 밖에 직접 보관해야 합니다.
- **받을 때.** 받기 전에 라이브러리 백업을 `backups/<날짜>/`에 만듭니다. 내려받는 동안 암호문을 `<앱 데이터>/vault-incoming/`에 두었다가 끝나면 지웁니다.
- **해제와 삭제.** `설정 → 비공개 보관함`에서 작품을 하나씩 서버에서 지울 수 있고, 사이트의 보관함 화면에서 해지하면 서버의 보관함 데이터가 모두 지워집니다. 사이트 연결을 해제하는 것만으로는 서버의 데이터가 지워지지 않습니다. 서버에 얼마나 남는지와 동의 기록은 Linetta Works의 개인정보처리방침을 따릅니다.
- **보장.** 무료 베타 기능이며 데이터가 유실될 수 있습니다. 보관함을 유일한 사본으로 삼지 마십시오.

### 4. 데이터 판매·공유
개발자는 사용자 데이터를 판매하거나 자체 목적으로 공유하지 않습니다. 원고가 다른 곳에 닿는 경로는 사용자가 직접 켜는 것뿐입니다. 내장 에이전트는 사용자가 연결한 프로바이더에 원고를 보내고, 삽화 생성은 사용자가 고른 이미지 프로바이더에 프롬프트와 참조 이미지를 보내며, 사용자가 실행하는 MCP 클라이언트는 원고를 읽어 가며, GitHub 동기화는 사용자의 git 원격으로 원고를 푸시하고, 폴더 동기화는 사용자가 지정한 폴더로 원고를 복사하며, 게시는 사용자가 고른 회차와 시트·표지를 Linetta Works에 공개하고, 비공개 보관함은 사용자가 고른 작품의 암호문을 Linetta Works에 맡깁니다. 모두 위 3항과 같습니다.

### 5. 아동의 개인정보
개발자는 아동을 포함한 사용자의 개인정보를 수집하지 않습니다. 아동 사용자는 연결하는 AI 프로바이더와 이미지 프로바이더, MCP로 연결하는 클라이언트, 푸시하는 git 호스트, 동기화 폴더를 제공하는 클라우드 서비스, 그리고 게시하거나 보관함을 쓰는 Linetta Works 각각의 연령 요건 및 보호자 동의 요건도 충족해야 합니다.

### 6. 변경 사항
본 방침이 변경되면 이 페이지를 갱신하고 상단의 "최종 업데이트" 날짜를 수정합니다.

### 7. 문의
문의: devlikebear@gmail.com

---

## English

Linetta (the "App") is a **local-first** desktop app for writing long-form fiction. The developer (Changheon Shin / devlikebear) operates no analytics, advertising, or account server for the App and does not collect user data. The App takes your writing off this device only along paths you set up yourself: the built-in agent, which sends it to an AI provider you connect; illustration generation, which sends a prompt and reference images to an image provider you choose; an MCP client you run yourself; GitHub Sync, which pushes it to a git remote you choose; Folder Sync, which copies it into a folder you choose, such as a cloud drive folder; Publishing, which puts only the episodes, sheets and cover you choose on Linetta Works, a site for reading web fiction; and the Private Vault, which seals a work you choose to upload on this device and keeps it there. Section 3 describes each of them. Of these, only Publishing and the Private Vault send anything to a server the developer operates, and what goes to the Private Vault is ciphertext the developer cannot read.

### 1. Information We Collect
**The developer collects none.** The App:
- has no user accounts or sign-in of its own (Publishing and the Private Vault connect an account on the Linetta Works site; see Sections 3.5 and 3.7);
- uses no analytics, tracking, telemetry, or advertising identifiers;
- does not send any user data to the developer (including crash reports).

### 2. Where Data Is Stored
Core data you create — manuscripts, projects, and settings — is stored **locally on your device**.
- Writing data: a local database on your device.
- Observations an agent records while checking a scene's continuity (contradictions, new facts, and open questions, with their status) are stored in the same local database (`library.db`); the manuscript itself is never changed.
- Suggestions an agent submits with `linetta_propose_story_ops` (candidate characters, places, relationships, fact cards, and more) are stored in the same local database too. Nothing reaches Story World until you accept an item from the suggestions inbox, or tell the agent to accept it.
- Narrative forecasts an agent saves with `linetta_save_forecast` (branches with their premise, beats, and risks) are stored in the same local database too. The outline, manuscript, and Story World never change; only the branch you pick enters the next brief as a plan.
- Illustrations (with their prompt, provider and model), character reference images, and an agent's illustration requests are stored in the same local database.
- Sensitive values such as the MCP connection token and an AI provider API key: your operating system's secure store (macOS Keychain, Windows Credential Manager). They are never written to `settings.json`.

The developer has no access to this data.

Linux has no secure-store backend, so an API key cannot be stored there. On Linux the API-key providers cannot be configured at all, and only the ChatGPT (Codex) sign-in, which stores its tokens in the file described below, and the Claude Code provider, for which Linetta keeps no credential, work there. There is no equivalent sign-in for the MCP connection token, so on Linux it is written to `<app data>/mcp-token` with owner-only read permissions (0600) instead of a secure store. That token grants read access to your manuscript through the local endpoint described in Section 3.2, and write access as well when you have set that endpoint to full access.

Signing in to ChatGPT (Codex) stores an access token and a refresh token at `<app data>/codex/auth.json`. This file is saved with owner-only read permissions (0600) — the same way the official Codex CLI keeps its own token. Linetta sends this file nowhere; it is used only for requests to OpenAI.

Outside the macOS App Store build, Linetta also honours a sign-in you already completed with the Codex CLI: when there is no `<app data>/codex/auth.json` but `~/.codex/auth.json` exists, Linetta uses that file instead, so you do not have to sign in twice. The macOS App Store build cannot read outside its sandbox container and never uses it.

When an access token expires, Linetta refreshes it and rewrites whichever of those two files is in use — including `~/.codex/auth.json`, when the Codex CLI's sign-in is the one being used.

Signing out from Settings deletes Linetta's own file, `<app data>/codex/auth.json`. It does not delete a Codex CLI sign-in at `~/.codex/auth.json`; sign that one out with the Codex CLI itself.

Every build, including the macOS App Store build, uses this file store — Linetta does not put this token in the system keychain.

### 3. When Your Writing Leaves Your Device
Your writing can leave this device along the paths described below. None of them sends anything until you set it up yourself. The built-in agent, illustration generation, MCP, Publishing and the Private Vault act only when you use them; GitHub Sync and Folder Sync, once you have set them up, run on their own.

#### 3.1 The Built-in Agent (a Provider You Connect)
Linetta's built-in agent has nothing to call until you connect a provider yourself. Five are supported: ChatGPT (Codex), which you connect by signing in with your ChatGPT account; Claude Code, which runs the `claude` CLI you already have installed under your own Claude sign-in (not in the macOS App Store build); and Anthropic, Google Gemini, and an OpenAI-compatible endpoint, which you connect with an API key.

- **Where it goes.** To the provider you selected, and nowhere else. With Claude Code it goes to Anthropic through the CLI on this device. The OpenAI-compatible option goes to the base URL you enter — that address may be a service such as OpenRouter, or a model running on this machine, in which case the data never leaves it.
- **What goes.** What you type to the agent, the earlier messages in that conversation, and the work and scene you have open. Beyond that, whatever the agent reads through its tools travels to the provider with the request that follows. The tools are not tied to the work you have open: they can list every work in your library with its title, synopsis, and genres, and from any of those works read any scene, the scene and chapter summaries, the outline, full-text search results from across that work's manuscript, the character, plot, and fact cards, and the facts the agent has been told to remember. Those remembered facts live in `<app data>/<project id>/memory/experiences.jsonl` rather than in the database, and the agent can add to them as well as read them. The agent is instructed to gather that context before it drafts anything. What the tools can reach is what the consent sentence you tick in Settings describes. Beyond that, your **direction** for the work — the current focus and this scene's intent, if you have set either — travels with every request too. You alone write this document; no tool lets the agent write it. Beyond that again, two short curated documents travel with every request. They are a writer profile, and the notes for the work in hand. Both are pasted whole into the system prompt, so they are sent even if the agent calls no tool at all, and either the agent writes them itself or you rewrite them under `Settings → Memory`. The writer profile is global. It is not scoped to the work you have open, so what the agent recorded while working on one work goes to the provider with a request about any other. The work notes stay with their work. A list of **skills** rides along the same way. A skill is a document an agent has written about method; skills live as files rather than in the database, at `<app data>/skills/<name>/SKILL.md` (every work) and `<app data>/skills/works/<project id>/<name>/SKILL.md` (one work). What travels with every request is the **name and description only** of each enabled skill; a body goes to the provider with the request in which the agent opens that skill with `linetta_read_skill`. A skill under `skills/<name>/` is global, like the writer profile: what was recorded while working on one work goes with a request about any other. The agent writes, edits and deletes skills with `linetta_edit_skill` **without asking you**. There is no approval step. What you get instead is a mark on every skill saying whether you or an agent wrote it, a revertible version of every write, and one switch that takes a skill out of the request without deleting it — all under `Settings → Skills`.
- **Filling Story World from an import.** After importing a Markdown manuscript, starting "Fill Story World" has the agent read scenes episode by episode and propose character, place, relationship, and fact candidates with `linetta_propose_story_ops`. The manuscript text travels to your connected provider the same way described in "What goes" above. Before you start, the App shows the episode count, character count, and an estimated input-token figure — an estimate, not a bill; actual cost follows your provider's own terms. Candidates sit in the suggestions inbox and reach Story World only once you accept them or tell the agent to.
- **An extra call after a tool-heavy turn.** When the agent executed eight or more tool calls in a turn, and your reply has already been sent, the App makes one more separate request to the same provider and the same model, asking whether that turn taught it a technique worth recording. That request carries no manuscript text and none of your messages: what it carries is the **list of tool names** the turn called, and the names and descriptions of the skills that already exist. It usually ends in one round trip, and reaches at most five when it does write a skill. It is on by default and your provider bills it like any other call. Switching it off under `Settings → Skills → Learning from its own work` means the call is not made at all.
- **Images.** An image you attach to an agent message goes to the provider with the requests of the turn that answers it. If a turn calls the model several times, the image is sent again with each call. It is not sent in later turns, and the conversation history keeps how many images there were, not the images. The App scales large images down before sending. When you ask, the agent can save an attached image as a character's reference image. If the agent asks `linetta_get_character_visuals` to include images, those characters' reference images go to the provider too. The Claude Code provider does not accept attached images.
- **The Claude Code provider.** Here Linetta does not call an API itself: it runs the `claude` CLI on this device. Linetta blocks that CLI's file, shell and web tools and your hooks, plugins and other MCP servers, and allows only Linetta's own tools. What the CLI keeps on this device, and how Anthropic handles the data, is governed by Claude Code's settings and terms.
- **When it goes.** Only after you consent. Consent is per provider: agreeing to one is not agreeing to another. Unticking the same box withdraws it in one click, and the App stops sending to that provider at once. The check runs before the App builds anything that can send, so without it even the "Test connection" button is refused. (Once you have entered a credential, the App can ask a provider for its list of available models; that request carries no manuscript content.)
- **Your credential.** Kept as described in Section 2: an API key in your operating system's secure store, a ChatGPT (Codex) sign-in in `<app data>/codex/auth.json`. It is sent only to that provider, as part of the request. The CLI manages your Claude Code sign-in itself; Linetta does not store it.
- **What the provider does with that data** is governed by that company's privacy policy and terms, including whether it retains the text or trains on it. Linetta cannot promise anything on that company's behalf; read the policy of the provider you choose.
- **It writes, too.** The agent can write scenes and restructure the outline directly, not only read. This is unaffected by setting "Connect an external agent (MCP)" to read-only or turning it off — that setting governs external clients only. Every tool call the agent makes is recorded in the same activity log as an external client's calls, tagged with which one made it, and viewable under the MCP section of Settings.
- The developer does not receive, relay, or store this data. It goes from your device straight to your provider.

#### 3.2 Connecting an External Agent (MCP)
When you explicitly consent and enable it under `Settings → Connect an external agent (MCP)`, the App opens a local endpoint bound to `127.0.0.1`. An MCP client you run on the same machine (such as Claude Code, Claude Desktop, Codex CLI, or Gemini CLI) can then read your manuscript, story structure, and fact cards, and in full-access mode modify them.

- The recipient of anything read this way is **the client you chose**. What that client sends onward to its own provider is governed by that product's privacy policy.
- The endpoint is off by default and opens only after you tick the consent box and enable it. Turning it off closes it immediately.
- Connecting requires a token the App generates locally, which you can regenerate at any time.
- There is no remote access: the endpoint is unreachable from another machine.
- A connected client also reads your **direction** for the work — the current focus and this scene's intent — at the top of the story brief. It reads this in read-only mode as well as full-access mode. It cannot write it: no MCP tool writes this document, in either mode; only you, inside the App, do.
- A connected client also reads the two curated documents from Section 3.1 — the writer profile and that work's notes — in the story brief. It reads them in read-only and full-access mode alike, and can write them only in full-access mode, with the `linetta_edit_memory` tool. In read-only mode that tool is not registered at all. The writer profile is global, so what a client records there is read on every work. A client whose access you have pinned to a single work can write only that work's notes and is refused on the writer profile.
- The story brief also lists the **name and description** of every enabled skill. It does so in both modes, and a body is handed over only for the skill a client opens with `linetta_read_skill` — a read tool, registered in read-only mode as well. Writing a skill is `linetta_edit_skill`, which like every other write tool is registered only in full-access mode. A skill under `skills/<name>/` is global, so a client pinned to a single work can write only that work's skills and is refused on a global one, for the same reason it is refused on the writer profile.
- A connected client can also read character sheets and reference images, in both modes. Reference images are delivered only when the client asks `linetta_get_character_visuals` to include them.
- A client in full-access mode can request an illustration but cannot draw one: a request goes to no image provider until you press Generate in the App (Section 3.6). It can also publish an episode once you have connected an account (Section 3.5).
- Every change an agent makes is recorded in the App's activity log. A scene's text is snapshotted before it is rewritten, so the previous text can be restored, and an outline restructuring can be reversed in one step. Changes to characters, relationships, storylines, beats, fact cards, and memories are recorded but have no undo; correcting one means making another change.
- The purpose is to provide generation, revision, summarization, or research assistance you request.
- The developer does not receive, relay, or store this data.

#### 3.3 GitHub Sync
GitHub Sync is off until you choose a git folder for it under `Settings → GitHub Sync`. Once you have, it runs on its own: it exports **every** work you have not archived — not only the one you have open — to Markdown files in that folder, commits them, and pushes. It runs each time you start the App, and again each day the App stays open; there is no waiting period before the first export. The macOS App Store build does not have this feature; every other desktop build ships it.

- **Where it goes.** To whatever remote that git repository is configured to push to, and nowhere else. That is your repository on your git host, under that host's privacy policy and terms. It is worth checking whether the repository you picked is private. A repository with no remote is committed to locally and pushed nowhere.
- **What goes.** Every work in your library that you have not archived, exported as Markdown, including the manuscript, the outline, and your story notes.
- **Authentication** uses your existing system git setup, such as SSH keys or a credential helper. Linetta neither asks for nor stores a git credential.
- A file named exactly `.mcp.json` at the top of that folder is deliberately left unstaged, because the setup Linetta recommends puts one there. The exclusion is only that file: any other MCP client config, such as `claude_desktop_config.json` or an `.mcp.json` inside a subfolder, is staged like anything else. Leaving a file unstaged is also not the same as keeping it off the remote — a file you have already committed yourself is pushed with everything else. The snippet Linetta gives you carries no token, so this matters for a config you wrote by hand.
- The developer does not receive, relay, or store this data.

#### 3.4 Folder Sync
Folder Sync is off until you turn it on and choose a destination folder under `Settings → Folder Sync`. Once you have, it runs on its own: each time you start the App, and again each day the App stays open, it exports every work you have not archived to Markdown files and copies them into that folder. That folder is often a cloud drive folder — the example the App itself offers is a Dropbox folder — and if it is, your whole library is uploaded by that service. Every build has this feature, the macOS App Store build included.

- **Where it goes.** Into the folder you chose, and nowhere else. Linetta writes the files; it does not upload them. If that folder belongs to a cloud storage service, that service then copies them to its own servers, under its privacy policy and terms. If it is an ordinary local folder, nothing leaves this device.
- **What goes.** Every work in your library that you have not archived, exported as Markdown, including the manuscript, the outline, and your story notes. It is the same export GitHub Sync writes.
- **No credential is involved.** Linetta asks for nothing but the folder. Who can read what lands there is decided by the folder you picked and by the service, if any, that syncs it.
- The developer does not receive, relay, or store this data.

#### 3.5 Publishing to Linetta Works
Publishing is off until, under `Settings → Publishing`, you agree to what it sends and connect your Linetta Works (works.marvin-42.com) account in the browser; until then the App sends the site no requests at all. Once connected it still does nothing on its own: an episode is sent only when you press "Publish" in its publish dialog, or ask an agent you connected to publish it. Sheets and the cover are sent only when you press "Upload sheets and cover" in the publish dialog; an agent cannot upload them. The iOS build does not have this feature. Linetta Works is operated by the developer of Linetta.

- **Where it goes.** To the Linetta Works server. A published episode, and the sheets and cover you chose to make public, are public for anyone to read. The site sends published text, illustrations, sheets and the cover to the Anthropic API for content review.
- **What goes.** When you connect, this device's name ("Linetta (…)" with your computer's name) is sent so it can appear in your device list on the site. When you publish, the episode's title and text, the illustrations placed in it, its place in the order, the work's title, genres, blurb, serial status and address, the App's language, and the Linetta internal IDs that identify the episode and the work are sent. When you upload sheets, each sheet you turned on sends its name, aliases, role, summary and attributes, the names of its relationships with other public sheets, and its first reference image, and the cover image you picked goes with them. Episodes you have not published, notes, sheets you did not turn on, relationship notes, visual notes, fact cards and agent memories are not sent. Requests carry the App's version, and the site receives your IP address, as with any internet request.
- **Connection details.** The publish token the site hands out once is kept in your operating system's secure storage (the macOS Keychain, the Windows Credential Manager). Linux has none, so there it is kept in `<app data>/publish-token`, readable only by its owner (0600). The token can put that account's works, episodes, sheets and cover on the site and change whether they are public and, for an account that uses the Private Vault, upload, download and delete its ciphertext. The token alone cannot read that ciphertext (Section 3.7). Details that are not secret, such as your pen name and the site address, are kept in `settings.json`.
- **Disconnecting and removing.** "Disconnect" under `Settings → Publishing` erases the connection stored on this device. To remove this device from your device list on the site, disconnect it from your profile there. Episodes you published stay on the site after you disconnect; make them private or delete them from the App's publish dialog or from My works on the site. Turn a sheet off and upload sheets and cover again to take it off the site too.
- **Sharing.** The Share buttons in a public episode's publish dialog act only when you press them. X, Threads and Naver Blog open that service's share page in your browser with the episode's title and address filled in. KakaoTalk and Instagram copy the title and address to your clipboard, and Instagram also opens its site. From there, that service's privacy policy and terms apply.

#### 3.6 Illustration Generation
Illustration generation is off until you choose ChatGPT or Gemini under `Settings → Image generation` and consent for that provider. This consent is separate from the built-in agent's, and without it the App sends nothing. Even then, the App sends only when you press "Generate". An agent can only request an illustration, and that request too goes nowhere until you press Generate.

- **Where it goes.** To the provider you chose, and nowhere else: OpenAI for a ChatGPT sign-in, Google for Gemini.
- **What goes.** The prompt, which you can review and edit (scene description, character looks, art style), the character reference images you chose, and earlier illustrations from the same work. The App preselects up to two earlier illustrations and shows them as thumbnails; you can leave any or all of them out.
- **Your credential.** The sign-in or API key from the AI provider settings, kept as described in Section 2.
- **What the provider does with that data** is governed by that company's privacy policy and terms. So is the cost: ChatGPT counts against your plan's usage, and Google bills Gemini.
- The developer does not receive, relay, or store this data. Section 3.5 applies to an illustration you publish.

#### 3.7 Private Vault
The Private Vault is available only to Linetta Works accounts the operator has approved. The App sends no work until you have agreed and asked on the site, been approved, agreed to what is sent under `Settings → Private Vault`, and made or entered a recovery key. After that it still uploads and downloads nothing by itself: works move only when you press Upload or Download in a work's vault window. Opening the library or a work only asks the server whether a newer version exists.

- **Where it goes.** Through the Linetta Works server the developer operates, into its storage (Cloudflare R2). It is not public and is not sent to Anthropic for content review.
- **What goes.** The title and manuscript of a work you upload, its outline, story data, version snapshots, pictures, and per-work skills and memory, sealed on this device. The server keeps only ciphertext, its size and times, and randomly generated item and blob identifiers. The developer cannot read the content or the title. Activity logs, your global writer profile and global skills, and provider settings and API keys are not sent.
- **Recovery key.** The recovery key behind the encryption is never sent to the site. It is kept in your operating system's secure storage; Linux has none, so there it is kept in `<app data>/vault-key`, readable only by its owner (0600). A locked secure store does not fall back to a file. Disconnecting from the site does not remove it. If you lose the recovery key, not even the developer can recover your works, so keep your own copy off this device.
- **When downloading.** The App backs up your library to `backups/<date>/` first. Ciphertext sits in `<app data>/vault-incoming/` during a download and is removed when it finishes.
- **Removal and deletion.** Under `Settings → Private Vault` you can delete works from the server one at a time, and closing the vault on the site's vault page deletes all your vault data there. Disconnecting from the site does not delete anything on the server. How long the server keeps data, and consent records, are covered by the Linetta Works privacy policy.
- **No guarantee.** This is a free beta and data can be lost. Do not make it your only copy.

### 4. Selling or Sharing Data
The developer does not sell user data or share it for the developer's own purposes. The only ways your writing reaches anyone else are the ones you turn on yourself: the built-in agent sends it to the provider you connected, illustration generation sends a prompt and reference images to the image provider you chose, an MCP client you run reads it, GitHub Sync pushes it to your git remote, Folder Sync copies it into the folder you chose, Publishing makes the episodes, sheets and cover you choose public on Linetta Works, and the Private Vault keeps the ciphertext of works you choose on Linetta Works. Each is described in Section 3.

### 5. Children's Privacy
The developer collects no personal data from users, including children. Child users must also satisfy the age and parental-consent requirements of any AI or image provider they connect, any client they connect over MCP, the git host they push to, any cloud service whose folder they sync into, and Linetta Works if they publish there or use its vault.

### 6. Changes
If this policy changes, we will update this page and revise the "Last updated" date above.

### 7. Contact
Contact: devlikebear@gmail.com

---

## 日本語

Linetta（以下「本アプリ」）は、長編小説・Web小説の執筆のための**ローカルファースト**なデスクトップアプリです。開発者（Changheon Shin / devlikebear）は、本アプリのための分析・広告・アカウント用サーバーを運営せず、ユーザーデータを収集しません。本アプリが原稿を端末の外に出すのは、ユーザー自身が設定した経路に限られます。ユーザーが接続した AI プロバイダーへ原稿を送信する内蔵エージェント、ユーザーが選んだ画像プロバイダーへプロンプトと参照画像を送信する挿絵生成、ユーザー自身が実行する外部 MCP クライアント、ユーザーが指定した git リモートへプッシュする GitHub 同期、ユーザーが指定したフォルダ（例: クラウドドライブのフォルダ）へコピーするフォルダ同期、ユーザーが選んだエピソードとシート・表紙だけを Web 小説サイト Linetta Works に公開する公開機能、そしてユーザーがアップロードを選んだ作品を端末で暗号化して預ける非公開保管庫で、いずれも第3項で説明します。このうち公開機能と非公開保管庫だけが、開発者の運営するサーバーへ送信します。非公開保管庫へ送られるのは、開発者が読めない暗号文です。

### 1. 収集する情報
**開発者が収集する情報はありません。** 本アプリは以下を行いません。
- 本アプリ自体にはユーザーアカウントやログインはありません。公開機能と非公開保管庫を使うには Linetta Works サイトのアカウントを本アプリに接続する必要があり、第3.5項と第3.7項で説明します。
- 分析（analytics）・トラッキング・テレメトリ・広告識別子を使用しません。
- クラッシュレポートなどでユーザーデータを開発者のサーバーへ送信しません。

### 2. データの保存場所
ユーザーが作成した原稿・プロジェクト・設定などの基本データは、**ユーザーの端末内にローカルで**保存されます。
- 執筆データ: 端末内のローカルデータベース
- エージェントが「連続性チェック」で記録した観察（矛盾・新事実・確認が必要な項目とその状態）も、同じローカルデータベース（`library.db`）に保存されます。原稿本文は変更されません。
- エージェントが `linetta_propose_story_ops` で提案した候補（人物・場所・関係・ファクトなど）も同じローカルデータベースに保存されます。提案箱でユーザーが承認するか、ユーザーの指示を受けたエージェントが承認するまで、Story World には何も反映されません。
- エージェントが `linetta_save_forecast` で保存した展開予測（分岐ごとの前提・ビート・リスクなど）も同じローカルデータベースに保存されます。アウトライン・原稿・Story World は変更されず、ユーザーが選んだ分岐だけが次のブリーフに計画として入ります。
- 挿絵（プロンプトとプロバイダー・モデルの情報を含む）、人物の参照画像、エージェントの挿絵依頼も同じローカルデータベースに保存されます。
- MCP 接続トークンや AI プロバイダーの API キーなどの機密情報: OS のセキュアストア（macOS キーチェーン、Windows 資格情報マネージャー）。`settings.json` には保存しません。

開発者はこのデータにアクセスできません。

Linux にはセキュアストアのバックエンドがないため、API キーを保存できません。したがって Linux では API キーで接続するプロバイダーを設定できず、トークンを下記のファイルに保存する ChatGPT（Codex）サインインと、Linetta が認証情報を保管しない Claude Code プロバイダーのみ利用できます。MCP 接続トークンにはそれに相当するサインイン手段がないため、Linux ではセキュアストアの代わりに `<アプリデータ>/mcp-token` へ所有者のみが読み取れる権限（0600）で保存されます。このトークンは、第3.2項のローカルエンドポイントを通じて原稿を読み取る権限を与え、そのエンドポイントを全アクセスモードに設定している場合は書き込み権限も与えます。

ChatGPT（Codex）にサインインすると、アクセストークンとリフレッシュトークンが `<アプリデータ>/codex/auth.json` に保存されます。このファイルは所有者のみが読み取れる権限（0600）で保存され、これは Codex CLI が自身のトークンを保管する方式と同じです。Linetta はこのファイルをどこにも送信せず、OpenAI へのリクエストにのみ使用します。

macOS App Store ビルド以外では、すでに Codex CLI で済ませたサインインをそのまま利用します。`<アプリデータ>/codex/auth.json` がなく `~/.codex/auth.json` がある場合、Linetta はそのファイルを代わりに使用するため、二度サインインする必要はありません。macOS App Store ビルドはサンドボックスコンテナの外を読み取れないため、このファイルを使用しません。

アクセストークンの有効期限が切れると、Linetta はトークンを更新し、そのとき使用しているファイルを書き換えます。Codex CLI のサインインを使用している場合は `~/.codex/auth.json` もその対象です。

設定からログアウトすると、Linetta 自身のファイルである `<アプリデータ>/codex/auth.json` が削除されます。`~/.codex/auth.json` にある Codex CLI のサインインは削除しないため、そちらは Codex CLI 自身でログアウトしてください。

macOS App Store ビルドを含むすべてのビルドがこのファイル保存方式を使用し、このトークンをシステムキーチェーンには保存しません。

### 3. 原稿が端末を離れる場合
原稿が端末を離れる可能性がある経路は、以下に挙げるものです。いずれもユーザー自身が設定するまで何も送信しません。内蔵エージェント、挿絵生成、MCP、公開機能、非公開保管庫はユーザーが使うときにのみ動作しますが、GitHub 同期とフォルダ同期は一度設定すると自動的に動作します。

#### 3.1 内蔵エージェント（ユーザーが接続したプロバイダー）
Linetta の内蔵エージェントは、ユーザー自身がプロバイダーを接続するまで呼び出す先がありません。対応するプロバイダーは五つです。ChatGPT（Codex）は ChatGPT アカウントでサインインして接続し、Claude Code はインストール済みの `claude` CLI をユーザーの Claude サインインで実行し（macOS App Store ビルドを除く）、Anthropic・Google Gemini・OpenAI 互換エンドポイントは API キーで接続します。

- **どこへ送られるか。** ユーザーが選んだプロバイダーにのみ送られ、それ以外のどこにも送られません。 Claude Code を選ぶと、この端末の CLI を経由して Anthropic へ送られます。OpenAI 互換の項目は、ユーザーが入力した base URL へ送られます。その宛先は OpenRouter のようなサービスのこともあれば、この端末で実行しているモデルのこともあり、後者ならデータは端末を離れません。
- **何が送られるか。** エージェントに入力した内容、その会話の以前のメッセージ、現在開いている作品とシーンが送られます。さらに、エージェントがツールで読み取ったものが、続くリクエストとともにプロバイダーへ送られます。ツールは現在開いている作品に縛られません。ライブラリにあるすべての作品をタイトル・あらすじ・ジャンルとともに一覧でき、そのどの作品からでも、どのシーンの原文でも、シーンと章の要約、アウトライン、その作品の原稿全体を対象とした検索結果、登場人物・プロット・ファクトカード、そしてエージェントが記憶した事実を読み取れます。記憶した事実はデータベースではなく `<アプリデータ>/<プロジェクト id>/memory/experiences.jsonl` にあり、エージェントは新たに書き加えることもできます。エージェントは、草稿を書く前にこの文脈を読み取るよう指示されています。ツールが到達できる範囲は、設定でチェックする同意文が述べるものと同じです。さらに、短く編まれた文書が二つ、毎回のリクエストとともに送られます。作家プロフィールと、いま作業している作品のノートです。どちらもシステムプロンプトにそのまま含まれるため、エージェントがツールを一つも呼ばなくても送信され、エージェント自身が書き込むこともあれば、ユーザーが `設定 → 記憶` で書き直すこともできます。作家プロフィールはグローバルです。開いている作品に紐づいていないため、エージェントがある作品で記録した内容が、他のどの作品についてのリクエストとともにもプロバイダーへ送られます。作品ノートはその作品に留まります。さらに**スキル**の一覧が同じかたちで載ります。スキルはエージェントが手法を書き留めた文書で、データベースではなくファイルとして `<アプリデータ>/skills/<名前>/SKILL.md`（すべての作品）と `<アプリデータ>/skills/works/<プロジェクト id>/<名前>/SKILL.md`（一つの作品）にあります。毎回のリクエストとともに送られるのは、有効なスキルの**名前と説明だけ**で、本文はエージェントが `linetta_read_skill` でそのスキルを開いたときにそのリクエストとともに送られます。`skills/<名前>/` にあるスキルは作家プロフィールと同じくグローバルで、ある作品で書き留めた内容が、他のどの作品についてのリクエストとともにも送られます。エージェントは `linetta_edit_skill` で、**ユーザーに尋ねることなく**スキルを書き、直し、削除します。承認の手順はありません。代わりに、スキルごとにユーザーが書いたのかエージェントが書いたのかが示され、書き込みのたびに戻せるバージョンが残り、スイッチ一つでスキルをオフにしてリクエストから外せます。すべて `設定 → スキル` にあります。
- **取り込んだ原稿からの Story World 補完。** Markdown 原稿を取り込んだあと「Story World を満たす」を開始すると、エージェントは話数ごとにシーンを読み、人物・場所・関係・ファクトの候補を `linetta_propose_story_ops` で提案箱に提案します。この過程でも原稿本文は接続中のプロバイダーへ送られます — 上記「何が送られるか」と同じ経路です。開始前に話数・文字数・推定入力トークン数が表示されますが、これはあくまで概算で、実際の課金はプロバイダー側の規約に従います。提案箱の候補は、ユーザーが承認するか、エージェントに承認を指示してはじめて Story World に反映されます。
- **手のかかったターンのあとの追加の呼び出し。** 一つのターンでエージェントがツールを8回以上呼び出し、返信がすでに送られたあと、本アプリは同じプロバイダーと同じモデルへ別のリクエストをもう一度だけ送り、そのターンで書き留める価値のある手法があったかを尋ねます。このリクエストに原稿の本文もユーザーのメッセージも含まれません。含まれるのは、そのターンが呼び出したツールの**名前の一覧**と、すでにあるスキルの名前・説明です。たいていは一往復で終わり、実際にスキルを書く場合でも最大で五往復です。既定はオンで、プロバイダーは他の呼び出しと同じように課金します。`設定 → スキル → 自分の仕事から学ぶ` でオフにすると、この呼び出しは行われません。
- **画像。** エージェントのメッセージに添付した画像は、そのメッセージに応答するターンのリクエストとともにプロバイダーへ送られます。一つのターンでモデルを複数回呼び出す場合は、呼び出しごとに再度送られます。以降のターンには送られず、会話履歴には画像ではなく枚数だけが残ります。大きな画像は送信前に本アプリが縮小します。ユーザーが頼めば、エージェントは添付画像を人物の参照画像として保存できます。また、エージェントが `linetta_get_character_visuals` で画像も求めた場合は、その人物の参照画像もプロバイダーへ送られます。Claude Code プロバイダーは添付画像を受け付けません。
- **Claude Code プロバイダー。** この場合、Linetta は API を直接呼び出さず、この端末の `claude` CLI を実行します。Linetta はその CLI のファイル・シェル・Web ツールと、ユーザーのフック・プラグイン・他の MCP サーバーを使えないようにし、Linetta 自身のツールだけを許可します。CLI がこの端末に残す記録と、Anthropic によるデータの扱いは、Claude Code の設定と利用規約に従います。
- **いつ送られるか。** 同意した後に限られます。同意はプロバイダーごとで、一方への同意が他方への同意にはなりません。同じチェックボックスを外せば 1 クリックで同意を撤回でき、本アプリはそのプロバイダーへの送信を直ちに停止します。この確認は、送信できるものを本アプリが組み立てる前に行われるため、同意がなければ「接続テスト」ボタンも拒否されます。（認証情報を入力した後は、本アプリがプロバイダーに利用可能なモデルの一覧を問い合わせることがあります。この要求に原稿の内容は含まれません。）
- **認証情報。** 第2項に記載のとおり保管します。API キーは OS のセキュアストアに、ChatGPT（Codex）のサインインは `<アプリデータ>/codex/auth.json` に保管します。認証情報はリクエストの一部として当該プロバイダーにのみ送信されます。Claude Code のサインインは CLI 自身が管理し、Linetta は保存しません。
- **プロバイダーがそのデータをどう扱うか**は、その企業のプライバシーポリシーおよび利用規約に従い、テキストを保持するか学習に使用するかもここに含まれます。Linetta がその企業に代わって約束することはできませんので、選んだプロバイダーの方針をご自身でご確認ください。
- **書き込みも行います。** エージェントは読み取るだけでなく、シーンを直接執筆し、アウトラインを再構成することもできます。この権限は「外部エージェント接続 (MCP)」を読み取り専用にしても、オフにしても変わりません — その設定は外部クライアントにのみ適用されます。エージェントが呼び出したすべてのツールは、外部クライアントの呼び出しと同じ活動ログに記録され、どちらが呼び出したかも記録されるため、設定の MCP 項目で確認できます。
- 開発者はこのデータを受信・中継・保存しません。データはユーザーの端末からユーザーのプロバイダーへ直接送られます。

#### 3.2 外部エージェントの接続 (MCP)
`設定 → 外部エージェント接続 (MCP)` で明示的に同意して有効にすると、`127.0.0.1` のみにバインドされるローカルエンドポイントが開きます。同じ端末で実行している MCP クライアント（Claude Code、Claude Desktop、Codex CLI、Gemini CLI など）が接続し、原稿・作品構造・ファクトカードを読み取り、フルアクセスモードでは変更できます。

- この経路で読み取られたデータの受信者は**ユーザーが選んだそのクライアント**です。そのクライアントが自社プロバイダーへ何を送るかは、当該製品のプライバシーポリシーに従います。
- エンドポイントは既定でオフで、同意チェックと有効化を経てのみ開きます。オフにすると直ちに閉じます。
- 接続には本アプリがローカルで生成したトークンが必要で、いつでも再発行できます。
- リモートからの接続はできません。他の端末からこのエンドポイントには到達できません。
- 接続したクライアントは、3.1 項の編まれた文書二つ — 作家プロフィールとその作品のノート — もストーリーブリーフで読み取ります。読み取りはどちらのモードでも行われ、書き込みはフルアクセスモードで `linetta_edit_memory` ツールを使う場合に限られます。読み取り専用モードでは、このツールは登録すらされません。作家プロフィールはグローバルであるため、クライアントがそこに記録した内容はすべての作品で読み取られます。アクセスを一つの作品に限定したクライアントは、その作品のノートだけを書き込め、作家プロフィールについては拒否されます。
- ストーリーブリーフには、有効なスキルの**名前と説明**も載ります。どちらのモードでも載り、本文が渡されるのはクライアントが `linetta_read_skill` で開いたスキルだけです。このツールは読み取りツールなので、読み取り専用モードでも登録されます。スキルを書くのは `linetta_edit_skill` で、他の書き込みツールと同じくフルアクセスモードでのみ登録されます。`skills/<名前>/` のスキルはグローバルなので、アクセスを一つの作品に限定したクライアントはその作品のスキルだけを書き込め、グローバルなスキルについては作家プロフィールと同じ理由で拒否されます。
- 接続したクライアントは、人物シートと参照画像も読み取れます。どちらのモードでも読み取れ、参照画像はクライアントが `linetta_get_character_visuals` で画像も求めたときにだけ渡されます。
- フルアクセスモードのクライアントは挿絵を依頼できますが、自分で描くことはできません。依頼は、ユーザーが本アプリで生成を押すまで画像プロバイダーへ送られません（第3.6項）。また、ユーザーがアカウントを接続していれば、エピソードを公開できます（第3.5項）。
- エージェントが行った変更はすべて活動ログに記録されます。シーンの本文は書き換える前にスナップショットが保存されるため以前の本文を復元でき、アウトラインの構造変更は一手で戻せます。登場人物・関係・ストーリーライン・ビート・ファクトカード・記憶の変更は記録されますが取り消す機能はなく、修正するには別の変更を行う必要があります。
- 目的は、ユーザーが要求した生成・修正・要約・調査の補助を提供することです。
- 開発者はこのデータを受信・中継・保存しません。

#### 3.3 GitHub 同期
GitHub 同期は、`設定 → GitHub 同期` でユーザーが git フォルダを指定するまでオフです。指定すると自動的に動作し、現在開いている作品だけでなくアーカイブしていない**すべて**の作品を Markdown ファイルとしてそのフォルダに書き出し、コミットしてプッシュします。実行のタイミングは、本アプリを起動するたびと、本アプリを開いたまま一日が経過するたびで、最初の書き出しまでの待ち時間はありません。macOS App Store ビルドにこの機能はなく、それ以外のすべてのデスクトップビルドに含まれます。

- **どこへ送られるか。** その git リポジトリに設定されたリモートにのみ送られ、それ以外のどこにも送られません。つまりユーザーの git ホスト上にあるユーザーのリポジトリであり、そのホストのプライバシーポリシーおよび利用規約に従います。選んだリポジトリが非公開かどうかをご確認ください。リモートのないリポジトリなら、ローカルにコミットされるだけでどこにもプッシュされません。
- **何が送られるか。** ライブラリにある作品のうちアーカイブしていないすべての作品が、原稿・アウトライン・設定資料を含めて Markdown として書き出されます。
- **認証**は、SSH キーや資格情報ヘルパーなど既存のシステムの git 設定をそのまま使用します。Linetta は git の認証情報を要求も保存もしません。
- Linetta が案内する設定がそのファイルをそこに置くため、そのフォルダの直下にある `.mcp.json` という名前のファイル一つは意図的にステージングから除外します。除外するのはそのファイルだけで、`claude_desktop_config.json` やサブフォルダの `.mcp.json` など他の MCP クライアント設定ファイルは、他のファイルと同じようにステージングされます。ステージングしないことは、リモートに送られないことと同じでもなく、ユーザーがすでにコミットしたファイルは他のものと一緒にプッシュされます。Linetta が提示する設定スニペットにトークンは含まれないため、これは自分で書いた設定ファイルに当てはまる話です。
- 開発者はこのデータを受信・中継・保存しません。

#### 3.4 フォルダ同期
フォルダ同期は、`設定 → フォルダ同期` でユーザーが有効にして宛先フォルダを指定するまでオフです。指定すると自動的に動作し、本アプリを起動するたび、そして本アプリを開いたまま一日が経過するたびに、アーカイブしていないすべての作品を Markdown ファイルとして書き出し、そのフォルダにコピーします。そのフォルダはクラウドドライブのフォルダであることが多く、本アプリが示す例からして Dropbox のフォルダであり、その場合はライブラリ全体がそのサービスへアップロードされます。macOS App Store ビルドを含むすべてのビルドにこの機能があります。

- **どこへ送られるか。** ユーザーが指定したフォルダにのみ送られ、それ以外のどこにも送られません。Linetta はファイルを書き出すだけで、アップロードはしません。そのフォルダがクラウドストレージサービスのものであれば、その後はそのサービスが自社のサーバーへコピーし、その企業のプライバシーポリシーおよび利用規約に従います。通常のローカルフォルダであれば、何も端末を離れません。
- **何が送られるか。** ライブラリにある作品のうちアーカイブしていないすべての作品が、原稿・アウトライン・設定資料を含めて Markdown として書き出されます。GitHub 同期が書き出すものと同じ内容です。
- **認証情報は不要です。** Linetta が求めるのはフォルダだけです。そこに置かれたものを誰が読めるかは、ユーザーが選んだフォルダと、それを同期するサービスがあればそのサービスによって決まります。
- 開発者はこのデータを受信・中継・保存しません。

#### 3.5 Linetta Works への公開
公開機能は、`設定 → 公開` でユーザーが送信内容に同意し、ブラウザで Linetta Works（works.marvin-42.com）のアカウントを接続するまでオフで、それまで本アプリはサイトへ一切リクエストを送りません。接続した後も自動的には動作せず、ユーザーがエピソードの公開ウィンドウで「公開」を押したとき、またはユーザーが接続したエージェントにエピソードの公開を指示したときにだけ、そのエピソードを送信します。シートと表紙は、ユーザーが公開ウィンドウで「シートと表紙をアップロード」を押したときにだけ送信し、エージェントはアップロードできません。iOS ビルドにこの機能はありません。Linetta Works は Linetta の開発者が運営するサイトです。

- **どこへ送られるか。** Linetta Works のサーバーへ送られます。公開したエピソードと、公開すると選んだシート・表紙は、誰でも読めるように公開されます。サイトは内容審査のため、公開された本文、挿絵、シート、表紙を Anthropic API へ送信します。
- **何が送られるか。** 接続時には、サイトのデバイス一覧に表示されるよう、この端末の名前（コンピューター名を含む「Linetta (…)」）が送信されます。公開時には、そのエピソードのタイトルと本文、そこに配置した挿絵、エピソードの順番、作品のタイトル・ジャンル・紹介文・連載状態・作品アドレス、本アプリの言語、そしてエピソードと作品を識別する Linetta 内部 ID が送信されます。シートのアップロード時には、公開をオンにしたシートの名前、別名、役割、要約、属性、公開シート同士の関係名、シートの最初の参照画像が送信され、選んだ表紙画像もあわせて送信されます。公開していないエピソード、メモ、公開をオンにしていないシート、関係のメモ、ビジュアルのメモ、ファクトカード、エージェントの記憶は送信しません。リクエストには本アプリのバージョンが含まれ、サイトは通常のインターネット通信と同様に接続元の IP アドレスを受け取ります。
- **接続情報。** サイトが一度だけ発行する公開用トークンは、OS の安全な保管場所（macOS のキーチェーン、Windows の資格情報マネージャー）に保存します。それがない Linux では、`<アプリデータ>/publish-token` ファイルに所有者だけが読める権限（0600）で保存します。このトークンでできるのは、そのアカウントの作品、エピソード、シート、表紙を公開し、公開・非公開を切り替えることと、非公開保管庫を使うアカウントであれば保管庫の暗号文をアップロード、ダウンロード、削除することです。トークンだけでは暗号文を読めません（第3.7項）。ペンネームやサイトのアドレスなど秘密でない接続情報は `settings.json` に保存します。
- **解除と削除。** `設定 → 公開` の「接続を解除」は、この端末に保存された接続情報を削除します。サイトのデバイス一覧からこの端末を外すには、サイトのプロフィールで解除してください。公開済みのエピソードは接続を解除してもサイトに残り、本アプリの公開ウィンドウまたはサイトのマイ作品から非公開にしたり削除したりできます。シートは公開をオフにしてシートと表紙をアップロードし直すと、サイトからも取り下げられます。
- **共有。** 公開中のエピソードの公開ウィンドウにある共有ボタンは、ユーザーが押したときにだけ動作します。X、Threads、Naver ブログのボタンは、エピソードのタイトルとアドレスを入れた各サービスの共有ページをブラウザで開きます。カカオトークと Instagram のボタンは、タイトルとアドレスをクリップボードにコピーします。Instagram のボタンはそのサイトも開きます。その先は各サービスのプライバシーポリシーと利用規約に従います。

#### 3.6 挿絵生成
挿絵生成は、`設定 → 画像生成` で ChatGPT か Gemini を選び、そのプロバイダーに同意するまでオフです。この同意は内蔵エージェントの同意とは別で、同意がなければ本アプリは何も送信しません。同意した後も、ユーザーが「生成」を押したときにだけ送信します。エージェントは挿絵を依頼できるだけで、その依頼もユーザーが生成を押すまで送信されません。

- **どこへ送られるか。** 選んだプロバイダー（ChatGPT サインインなら OpenAI、Gemini なら Google）にのみ送られます。
- **何が送られるか。** ユーザーが確認・編集できるプロンプト（場面の説明、人物の外見、画風）、選んだ人物の参照画像、そして同じ作品の以前の挿絵です。以前の挿絵は本アプリが最大2枚まであらかじめ選んでサムネイルで表示し、ユーザーはその一部または全部を外せます。
- **認証情報。** AI プロバイダー設定のサインインまたは API キーをそのまま使い、第2項に記載のとおり保管します。
- **プロバイダーがそのデータをどう扱うか**は、その企業のプライバシーポリシーおよび利用規約に従います。費用も同様で、ChatGPT はプランの利用量から差し引かれ、Gemini は Google が課金します。
- 開発者はこのデータを受信・中継・保存しません。公開した挿絵には第3.5項が適用されます。

#### 3.7 非公開保管庫
非公開保管庫は、運営者が許可した Linetta Works アカウントだけが使えます。ユーザーがサイトで同意して申請し、許可を受けた後、`設定 → 非公開保管庫` で送信内容に同意し、復旧キーを作成または入力するまで、本アプリは作品を送信しません。設定した後も自動ではアップロードもダウンロードもしません。作品の保管庫ウィンドウでユーザーが「アップロード」または「ダウンロード」を押したときにのみ作品をやり取りします。ライブラリや作品を開くときは、新しいバージョンがあるかをサーバーに問い合わせるだけです。

- **送信先。** 開発者が運営する Linetta Works のサーバーを経由して、そのストレージ（Cloudflare R2）に保存されます。公開されず、内容審査のために Anthropic へ送信されることもありません。
- **送信されるもの。** アップロードした作品のタイトルと原稿、アウトライン、世界観とストーリーのデータ、バージョンスナップショット、画像、作品ごとのスキルとメモリが、この端末で暗号化されて送信されます。サーバーに残るのは暗号文とそのサイズ、時刻、ランダムに生成した項目・断片の識別子だけです。開発者は内容もタイトルも読めません。活動ログ、全体の作家プロフィールと全体のスキル、プロバイダー設定と API キーは送信されません。
- **復旧キー。** 暗号化に使う復旧キーはサイトへ送信しません。OS の安全な保管場所に保存し、それがない Linux では `<アプリデータ>/vault-key` ファイルに所有者だけが読める権限（0600）で保存します。安全な保管場所がロックされていてもファイルには切り替えません。サイトとの接続を解除しても削除しません。復旧キーを紛失すると開発者でも作品を復元できないため、ユーザー自身がこの端末の外に保管してください。
- **ダウンロード時。** ダウンロードの前にライブラリのバックアップを `backups/<日付>/` に作成します。ダウンロード中は暗号文を `<アプリデータ>/vault-incoming/` に置き、完了すると削除します。
- **解除と削除。** `設定 → 非公開保管庫` で作品を一つずつサーバーから削除でき、サイトの保管庫画面で解約するとサーバーの保管庫データがすべて削除されます。サイトとの接続を解除するだけではサーバーのデータは削除されません。サーバーでの保管期間と同意記録については Linetta Works のプライバシーポリシーに従います。
- **保証。** 無料ベータ機能であり、データが失われることがあります。保管庫を唯一のコピーにしないでください。

### 4. データの販売・共有
開発者はユーザーデータを販売せず、開発者自身の目的で共有しません。原稿が他者に届く経路は、ユーザー自身が有効にするものだけです。内蔵エージェントはユーザーが接続したプロバイダーへ原稿を送信し、挿絵生成はユーザーが選んだ画像プロバイダーへプロンプトと参照画像を送信し、ユーザーが実行する MCP クライアントは原稿を読み取り、GitHub 同期はユーザーの git リモートへ原稿をプッシュし、フォルダ同期はユーザーが指定したフォルダへ原稿をコピーし、公開機能はユーザーが選んだエピソードとシート・表紙を Linetta Works で公開し、非公開保管庫はユーザーが選んだ作品の暗号文を Linetta Works に預けます。いずれも第3項のとおりです。

### 5. 子どものプライバシー
開発者は子どもを含むユーザーの個人情報を収集しません。子どもの利用者は、接続する AI プロバイダーと画像プロバイダー、MCP で接続するクライアント、プッシュ先の git ホスト、同期先フォルダを提供するクラウドサービス、そして公開先または保管庫として使う Linetta Works それぞれの年齢要件および保護者同意要件も満たす必要があります。

### 6. 変更
本ポリシーが変更された場合、このページを更新し、上部の「最終更新」日を改訂します。

### 7. お問い合わせ
お問い合わせ: devlikebear@gmail.com
