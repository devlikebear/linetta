# Linetta 개인정보 처리방침 / Privacy Policy

_최종 업데이트 / Last updated / 最終更新: 2026-09-06_

---

## 한국어

Linetta("앱")는 장편소설·웹소설 집필을 위한 **로컬 우선(local-first)** 데스크톱 앱입니다. 개발자(Changheon Shin / devlikebear)는 분석·광고·계정 서버를 운영하지 않으며 사용자 데이터를 수집하지 않습니다. 원고가 기기를 떠나는 경로는 사용자가 직접 설정한 것뿐입니다. 사용자가 연결한 AI 프로바이더로 원고를 보내는 내장 에이전트, 사용자가 직접 실행하는 외부 MCP 클라이언트, 그리고 사용자가 지정한 git 원격으로 푸시하는 GitHub 동기화이며, 아래 3항에서 모두 설명합니다.

### 1. 수집하는 정보
**개발자가 수집하는 정보는 없습니다.** 앱은 다음을 하지 않습니다.
- 사용자 계정 또는 로그인이 없습니다.
- 분석(analytics)·추적(tracking)·텔레메트리·광고 식별자를 사용하지 않습니다.
- 크래시 리포팅 등으로 사용자 데이터를 개발자 서버로 보내지 않습니다.

### 2. 데이터 저장 위치
사용자가 작성한 원고·프로젝트·설정 등 기본 데이터는 **사용자 기기에 로컬로** 저장됩니다.
- 집필 데이터: 기기의 로컬 데이터베이스
- MCP 접속 토큰, AI 프로바이더 API 키 등 민감 정보: 운영체제의 보안 저장소(macOS 키체인, Windows 자격 증명 관리자). `settings.json`에는 저장하지 않습니다.

개발자는 이 데이터에 접근할 수 없습니다.

Linux에는 보안 저장소 백엔드가 없어 API 키를 저장할 수 없습니다. 따라서 Linux에서는 API 키로 연결하는 프로바이더를 설정할 수 없고, 토큰을 아래 파일에 저장하는 ChatGPT(Codex) 로그인만 사용할 수 있습니다. MCP 접속 토큰에는 그런 대안이 없으므로, Linux에서는 보안 저장소 대신 `<앱 데이터>/mcp-token` 파일에 소유자만 읽을 수 있는 권한(0600)으로 저장됩니다. 이 토큰은 아래 3.2항의 로컬 엔드포인트를 통해 원고를 읽고 쓸 수 있는 전체 권한을 부여합니다.

ChatGPT(Codex)로 로그인하면 접근 토큰과 갱신 토큰이 `<앱 데이터>/codex/auth.json`에 저장됩니다. 이 파일은 소유자만 읽을 수 있는 권한(0600)으로 저장되며, 이는 Codex CLI가 자기 토큰을 보관하는 방식과 같습니다. Linetta는 이 파일을 어디로도 보내지 않으며, OpenAI에 요청을 보낼 때만 사용합니다.

macOS App Store 빌드가 아닌 빌드에서는, 이미 Codex CLI로 로그인해 둔 것을 그대로 인정합니다. `<앱 데이터>/codex/auth.json`이 없고 `~/.codex/auth.json`이 있으면 Linetta가 그 파일을 대신 사용하므로 두 번 로그인하지 않아도 됩니다. macOS App Store 빌드는 샌드박스 컨테이너 밖을 읽을 수 없어 이 파일을 사용하지 않습니다.

접근 토큰이 만료되면 Linetta가 토큰을 갱신하면서 그때 사용 중인 파일을 다시 씁니다. Codex CLI 로그인을 사용하고 있다면 `~/.codex/auth.json`도 여기에 포함됩니다.

설정에서 로그아웃하면 Linetta 자신의 파일인 `<앱 데이터>/codex/auth.json`이 삭제됩니다. `~/.codex/auth.json`에 있는 Codex CLI 로그인은 삭제하지 않으므로, 그쪽은 Codex CLI에서 직접 로그아웃해야 합니다.

macOS App Store 빌드를 포함한 모든 빌드가 이 파일 저장 방식을 사용하며, 이 토큰을 시스템 키체인에 저장하지 않습니다.

### 3. 원고가 기기를 떠나는 경우
원고가 기기를 떠날 수 있는 경로는 세 가지입니다. 셋 다 사용자가 직접 설정하기 전에는 아무것도 보내지 않습니다. 내장 에이전트와 MCP는 사용자가 쓸 때만 동작하지만, GitHub 동기화는 한 번 설정하고 나면 하루에 한 번 스스로 동작합니다.

#### 3.1 내장 에이전트 (사용자가 연결한 프로바이더)
Linetta의 내장 에이전트는 사용자가 프로바이더를 직접 연결하기 전까지 호출할 대상이 없습니다. 지원하는 프로바이더는 넷입니다. ChatGPT(Codex)는 ChatGPT 계정으로 로그인해 연결하고, Anthropic·Google Gemini·OpenAI 호환 엔드포인트는 API 키로 연결합니다.

- **어디로 가나.** 사용자가 선택한 프로바이더에만 가고, 그 밖의 어디로도 가지 않습니다. OpenAI 호환 항목은 사용자가 입력한 base URL로 갑니다. 그 주소는 OpenRouter 같은 서비스일 수도 있고 이 기기에서 실행 중인 모델일 수도 있으며, 후자라면 데이터는 기기를 떠나지 않습니다.
- **무엇이 가나.** 에이전트에 입력한 내용, 그 대화의 이전 메시지, 현재 열려 있는 작품과 씬이 갑니다. 여기에 더해, 에이전트가 툴로 읽은 것이 뒤이은 요청과 함께 프로바이더로 갑니다. 현재 열려 있는 씬만이 아니라 그 작품의 어느 씬이든, 씬과 챕터 요약, 아웃라인, 원고 전체를 대상으로 한 검색 결과, 등장인물·플롯·팩트 카드, 그리고 사용자의 작품 목록과 그 제목입니다. 에이전트는 초고를 쓰기 전에 이 맥락을 먼저 읽도록 지시받습니다. 툴이 닿을 수 있는 범위는 설정에서 체크하는 동의 문구가 말하는 것과 같습니다.
- **언제 가나.** 동의한 뒤에만 갑니다. 동의는 프로바이더별이며, 한 곳에 동의한 것이 다른 곳에 동의한 것이 되지 않습니다. 같은 체크박스를 해제하면 클릭 한 번으로 동의가 철회되고, 앱은 그 프로바이더로 보내는 것을 즉시 멈춥니다. 이 확인은 앱이 전송할 수 있는 것을 만들기 전에 이루어지므로, 동의가 없으면 "연결 테스트" 버튼도 거절됩니다. (자격 증명을 입력한 뒤에는 앱이 프로바이더에 사용 가능한 모델 목록을 물을 수 있습니다. 이 요청에는 원고 내용이 담기지 않습니다.)
- **자격 증명.** 2항에 적은 대로 보관합니다. API 키는 운영체제의 보안 저장소에, ChatGPT(Codex) 로그인은 `<앱 데이터>/codex/auth.json`에 보관합니다. 자격 증명은 요청의 일부로 해당 프로바이더에만 전송됩니다.
- **프로바이더가 그 데이터로 무엇을 하는지**는 그 회사의 개인정보 처리방침과 약관 소관이며, 텍스트를 보관하는지 학습에 사용하는지도 여기에 포함됩니다. Linetta가 그 회사를 대신해 약속할 수 없으니, 선택한 프로바이더의 방침을 직접 확인하시기 바랍니다.
- 개발자는 이 데이터를 수신·중계·저장하지 않습니다. 데이터는 사용자의 기기에서 사용자의 프로바이더로 바로 갑니다.

#### 3.2 외부 에이전트 연결 (MCP)
사용자가 `설정 → 외부 에이전트 연결(MCP)`에서 명시적으로 동의하고 켜면, 앱이 `127.0.0.1`에만 바인딩되는 로컬 엔드포인트를 엽니다. 사용자가 자기 기기에서 실행하는 MCP 클라이언트(예: Claude Code, Claude Desktop)가 여기에 접속해 원고·작품 구조·팩트 카드를 읽고, 전체 접근 모드에서는 수정할 수 있습니다.

- 이 경로로 전송되는 데이터의 수신자는 **사용자가 선택한 그 클라이언트**이며, 그 클라이언트가 자기 프로바이더에게 무엇을 보내는지는 해당 제품의 개인정보 처리방침을 따릅니다.
- 엔드포인트는 기본적으로 꺼져 있고, 동의 체크와 활성화를 거쳐야 열립니다. 앱에서 끄면 즉시 닫힙니다.
- 접속에는 앱이 로컬에서 생성한 토큰이 필요하며, 사용자는 언제든 재발급할 수 있습니다.
- 원격 접속은 불가능합니다. 다른 기기에서 이 엔드포인트에 닿을 수 없습니다.
- 에이전트가 수행한 모든 변경은 앱의 활동 기록에 남고, 변경 전 스냅샷이 저장되어 되돌릴 수 있습니다.
- 목적은 사용자가 요청한 생성·수정·요약·조사 보조를 제공하는 것입니다.
- 개발자는 이 데이터를 수신·중계·저장하지 않습니다.

#### 3.3 GitHub 동기화
GitHub 동기화는 `설정 → GitHub 동기화`에서 사용자가 git 폴더를 지정하기 전까지 꺼져 있습니다. 지정하고 나면 스스로 동작해, 하루에 한 번 현재 열려 있는 작품만이 아니라 **모든** 작품을 마크다운 파일로 그 폴더에 내보내 커밋하고 푸시합니다. macOS App Store 빌드에는 이 기능이 없고, 그 밖의 모든 데스크톱 빌드에 포함됩니다.

- **어디로 가나.** 그 git 저장소에 설정된 원격으로만 가고, 그 밖의 어디로도 가지 않습니다. 즉 사용자의 git 호스트에 있는 사용자의 저장소이며, 그 호스트의 개인정보 처리방침과 약관을 따릅니다. 선택한 저장소가 비공개인지 확인하시기 바랍니다. 원격이 없는 저장소라면 로컬에 커밋만 되고 아무 데도 푸시되지 않습니다.
- **무엇이 가나.** 서재에 있는 모든 작품이 원고·아웃라인·설정 자료를 포함해 마크다운으로 내보내집니다.
- **인증**은 SSH 키나 자격 증명 도우미 같은 기존 시스템 git 설정을 그대로 사용합니다. Linetta는 git 자격 증명을 묻지도, 저장하지도 않습니다.
- 그 폴더에 있는 MCP 클라이언트 설정 파일은 의도적으로 스테이징에서 제외하므로, 거기 담긴 접속 토큰이 커밋되거나 푸시되는 일은 없습니다.
- 개발자는 이 데이터를 수신·중계·저장하지 않습니다.

### 4. 데이터 판매·공유
개발자는 사용자 데이터를 판매하거나 자체 목적으로 공유하지 않습니다. 원고가 다른 곳에 닿는 경로는 사용자가 직접 켜는 세 가지뿐입니다. 내장 에이전트는 사용자가 연결한 프로바이더에 원고를 보내고, 사용자가 실행하는 MCP 클라이언트는 원고를 읽어 가며, GitHub 동기화는 사용자의 git 원격으로 원고를 푸시합니다. 셋 다 위 3항과 같습니다.

### 5. 아동의 개인정보
개발자는 아동을 포함한 사용자의 개인정보를 수집하지 않습니다. 아동 사용자는 연결하는 AI 프로바이더와 MCP로 연결하는 클라이언트 각각의 연령 요건 및 보호자 동의 요건도 충족해야 합니다.

### 6. 변경 사항
본 방침이 변경되면 이 페이지를 갱신하고 상단의 "최종 업데이트" 날짜를 수정합니다.

### 7. 문의
문의: devlikebear@gmail.com

---

## English

Linetta (the "App") is a **local-first** desktop app for writing long-form fiction. The developer (Changheon Shin / devlikebear) operates no analytics, advertising, or account server and does not collect user data. The App takes your writing off this device only along paths you set up yourself: the built-in agent, which sends it to an AI provider you connect; an MCP client you run yourself; and GitHub Sync, which pushes it to a git remote you choose. Section 3 describes all three.

### 1. Information We Collect
**The developer collects none.** The App:
- has no user accounts or sign-in;
- uses no analytics, tracking, telemetry, or advertising identifiers;
- does not send any user data to the developer (including crash reports).

### 2. Where Data Is Stored
Core data you create — manuscripts, projects, and settings — is stored **locally on your device**.
- Writing data: a local database on your device.
- Sensitive values such as the MCP connection token and an AI provider API key: your operating system's secure store (macOS Keychain, Windows Credential Manager). They are never written to `settings.json`.

The developer has no access to this data.

Linux has no secure-store backend, so an API key cannot be stored there. On Linux the API-key providers cannot be configured at all, and only the ChatGPT (Codex) sign-in works, because it stores its tokens in the file described below. The MCP connection token has no such alternative, so on Linux it is written to `<app data>/mcp-token` with owner-only read permissions (0600) instead of a secure store. That token grants full read and write access to your manuscript through the local endpoint described in Section 3.2.

Signing in to ChatGPT (Codex) stores an access token and a refresh token at `<app data>/codex/auth.json`. This file is saved with owner-only read permissions (0600) — the same way the official Codex CLI keeps its own token. Linetta sends this file nowhere; it is used only for requests to OpenAI.

Outside the macOS App Store build, Linetta also honours a sign-in you already completed with the Codex CLI: when there is no `<app data>/codex/auth.json` but `~/.codex/auth.json` exists, Linetta uses that file instead, so you do not have to sign in twice. The macOS App Store build cannot read outside its sandbox container and never uses it.

When an access token expires, Linetta refreshes it and rewrites whichever of those two files is in use — including `~/.codex/auth.json`, when the Codex CLI's sign-in is the one being used.

Signing out from Settings deletes Linetta's own file, `<app data>/codex/auth.json`. It does not delete a Codex CLI sign-in at `~/.codex/auth.json`; sign that one out with the Codex CLI itself.

Every build, including the macOS App Store build, uses this file store — Linetta does not put this token in the system keychain.

### 3. When Your Writing Leaves Your Device
Three paths can take your writing off this device. None of them sends anything until you set it up yourself. The built-in agent and MCP act only when you use them; GitHub Sync, once you have set it up, runs once a day on its own.

#### 3.1 The Built-in Agent (a Provider You Connect)
Linetta's built-in agent has nothing to call until you connect a provider yourself. Four are supported: ChatGPT (Codex), which you connect by signing in with your ChatGPT account, and Anthropic, Google Gemini, and an OpenAI-compatible endpoint, which you connect with an API key.

- **Where it goes.** To the provider you selected, and nowhere else. The OpenAI-compatible option goes to the base URL you enter — that address may be a service such as OpenRouter, or a model running on this machine, in which case the data never leaves it.
- **What goes.** What you type to the agent, the earlier messages in that conversation, and the work and scene you have open. Beyond that, whatever the agent reads through its tools travels to the provider with the request that follows: any scene in the work and not only the one you have open, scene and chapter summaries, the outline, full-text search results from across the work's manuscript, the character, plot, and fact cards, and the list of your works with their titles. The agent is instructed to gather that context before it drafts anything. What the tools can reach is what the consent sentence you tick in Settings describes.
- **When it goes.** Only after you consent. Consent is per provider: agreeing to one is not agreeing to another. Unticking the same box withdraws it in one click, and the App stops sending to that provider at once. The check runs before the App builds anything that can send, so without it even the "Test connection" button is refused. (Once you have entered a credential, the App can ask a provider for its list of available models; that request carries no manuscript content.)
- **Your credential.** Kept as described in Section 2: an API key in your operating system's secure store, a ChatGPT (Codex) sign-in in `<app data>/codex/auth.json`. It is sent only to that provider, as part of the request.
- **What the provider does with that data** is governed by that company's privacy policy and terms, including whether it retains the text or trains on it. Linetta cannot promise anything on that company's behalf; read the policy of the provider you choose.
- The developer does not receive, relay, or store this data. It goes from your device straight to your provider.

#### 3.2 Connecting an External Agent (MCP)
When you explicitly consent and enable it under `Settings → Connect an external agent (MCP)`, the App opens a local endpoint bound to `127.0.0.1`. An MCP client you run on the same machine (such as Claude Code or Claude Desktop) can then read your manuscript, story structure, and fact cards, and in full-access mode modify them.

- The recipient of anything read this way is **the client you chose**. What that client sends onward to its own provider is governed by that product's privacy policy.
- The endpoint is off by default and opens only after you tick the consent box and enable it. Turning it off closes it immediately.
- Connecting requires a token the App generates locally, which you can regenerate at any time.
- There is no remote access: the endpoint is unreachable from another machine.
- Every change an agent makes is recorded in the App's activity log and snapshotted beforehand so you can undo it.
- The purpose is to provide generation, revision, summarization, or research assistance you request.
- The developer does not receive, relay, or store this data.

#### 3.3 GitHub Sync
GitHub Sync is off until you choose a git folder for it under `Settings → GitHub Sync`. Once you have, it runs on its own: once a day it exports **every** work — not only the one you have open — to Markdown files in that folder, commits them, and pushes. The macOS App Store build does not have this feature; every other desktop build ships it.

- **Where it goes.** To whatever remote that git repository is configured to push to, and nowhere else. That is your repository on your git host, under that host's privacy policy and terms. It is worth checking whether the repository you picked is private. A repository with no remote is committed to locally and pushed nowhere.
- **What goes.** Every work in your library, exported as Markdown, including the manuscript, the outline, and your story notes.
- **Authentication** uses your existing system git setup, such as SSH keys or a credential helper. Linetta neither asks for nor stores a git credential.
- An MCP client config file sitting in that folder is deliberately left unstaged, so a connection token inside it is never committed or pushed.
- The developer does not receive, relay, or store this data.

### 4. Selling or Sharing Data
The developer does not sell user data or share it for the developer's own purposes. The only ways your writing reaches anyone else are the three you turn on yourself: the built-in agent sends it to the provider you connected, an MCP client you run reads it, and GitHub Sync pushes it to your git remote. All three are described in Section 3.

### 5. Children's Privacy
The developer collects no personal data from users, including children. Child users must also satisfy the age and parental-consent requirements of any AI provider they connect and of any client they connect over MCP.

### 6. Changes
If this policy changes, we will update this page and revise the "Last updated" date above.

### 7. Contact
Contact: devlikebear@gmail.com

---

## 日本語

Linetta（以下「本アプリ」）は、長編小説・Web小説の執筆のための**ローカルファースト**なデスクトップアプリです。開発者（Changheon Shin / devlikebear）は、分析・広告・アカウント用サーバーを運営せず、ユーザーデータを収集しません。本アプリが原稿を端末の外に出すのは、ユーザー自身が設定した経路に限られます。ユーザーが接続した AI プロバイダーへ原稿を送信する内蔵エージェント、ユーザー自身が実行する外部 MCP クライアント、そしてユーザーが指定した git リモートへプッシュする GitHub 同期の三つで、いずれも第3項で説明します。

### 1. 収集する情報
**開発者が収集する情報はありません。** 本アプリは以下を行いません。
- ユーザーアカウントやログインはありません。
- 分析（analytics）・トラッキング・テレメトリ・広告識別子を使用しません。
- クラッシュレポートなどでユーザーデータを開発者のサーバーへ送信しません。

### 2. データの保存場所
ユーザーが作成した原稿・プロジェクト・設定などの基本データは、**ユーザーの端末内にローカルで**保存されます。
- 執筆データ: 端末内のローカルデータベース
- MCP 接続トークンや AI プロバイダーの API キーなどの機密情報: OS のセキュアストア（macOS キーチェーン、Windows 資格情報マネージャー）。`settings.json` には保存しません。

開発者はこのデータにアクセスできません。

Linux にはセキュアストアのバックエンドがないため、API キーを保存できません。したがって Linux では API キーで接続するプロバイダーを設定できず、トークンを下記のファイルに保存する ChatGPT（Codex）サインインのみ利用できます。MCP 接続トークンにはそのような代替がないため、Linux ではセキュアストアの代わりに `<アプリデータ>/mcp-token` へ所有者のみが読み取れる権限（0600）で保存されます。このトークンは、第3.2項のローカルエンドポイントを通じて原稿を読み書きできる完全な権限を与えます。

ChatGPT（Codex）にサインインすると、アクセストークンとリフレッシュトークンが `<アプリデータ>/codex/auth.json` に保存されます。このファイルは所有者のみが読み取れる権限（0600）で保存され、これは Codex CLI が自身のトークンを保管する方式と同じです。Linetta はこのファイルをどこにも送信せず、OpenAI へのリクエストにのみ使用します。

macOS App Store ビルド以外では、すでに Codex CLI で済ませたサインインをそのまま利用します。`<アプリデータ>/codex/auth.json` がなく `~/.codex/auth.json` がある場合、Linetta はそのファイルを代わりに使用するため、二度サインインする必要はありません。macOS App Store ビルドはサンドボックスコンテナの外を読み取れないため、このファイルを使用しません。

アクセストークンの有効期限が切れると、Linetta はトークンを更新し、そのとき使用しているファイルを書き換えます。Codex CLI のサインインを使用している場合は `~/.codex/auth.json` もその対象です。

設定からログアウトすると、Linetta 自身のファイルである `<アプリデータ>/codex/auth.json` が削除されます。`~/.codex/auth.json` にある Codex CLI のサインインは削除しないため、そちらは Codex CLI 自身でログアウトしてください。

macOS App Store ビルドを含むすべてのビルドがこのファイル保存方式を使用し、このトークンをシステムキーチェーンには保存しません。

### 3. 原稿が端末を離れる場合
原稿が端末を離れる可能性がある経路は三つです。いずれもユーザー自身が設定するまで何も送信しません。内蔵エージェントと MCP はユーザーが使うときにのみ動作しますが、GitHub 同期は一度設定すると 1 日 1 回自動的に動作します。

#### 3.1 内蔵エージェント（ユーザーが接続したプロバイダー）
Linetta の内蔵エージェントは、ユーザー自身がプロバイダーを接続するまで呼び出す先がありません。対応するプロバイダーは四つです。ChatGPT（Codex）は ChatGPT アカウントでサインインして接続し、Anthropic・Google Gemini・OpenAI 互換エンドポイントは API キーで接続します。

- **どこへ送られるか。** ユーザーが選んだプロバイダーにのみ送られ、それ以外のどこにも送られません。OpenAI 互換の項目は、ユーザーが入力した base URL へ送られます。その宛先は OpenRouter のようなサービスのこともあれば、この端末で実行しているモデルのこともあり、後者ならデータは端末を離れません。
- **何が送られるか。** エージェントに入力した内容、その会話の以前のメッセージ、現在開いている作品とシーンが送られます。さらに、エージェントがツールで読み取ったものが、続くリクエストとともにプロバイダーへ送られます。現在開いているシーンに限らずその作品のどのシーンでも、シーンと章の要約、アウトライン、原稿全体を対象とした検索結果、登場人物・プロット・ファクトカード、そしてユーザーの作品一覧とそのタイトルです。エージェントは、草稿を書く前にこの文脈を読み取るよう指示されています。ツールが到達できる範囲は、設定でチェックする同意文が述べるものと同じです。
- **いつ送られるか。** 同意した後に限られます。同意はプロバイダーごとで、一方への同意が他方への同意にはなりません。同じチェックボックスを外せば 1 クリックで同意を撤回でき、本アプリはそのプロバイダーへの送信を直ちに停止します。この確認は、送信できるものを本アプリが組み立てる前に行われるため、同意がなければ「接続テスト」ボタンも拒否されます。（認証情報を入力した後は、本アプリがプロバイダーに利用可能なモデルの一覧を問い合わせることがあります。この要求に原稿の内容は含まれません。）
- **認証情報。** 第2項に記載のとおり保管します。API キーは OS のセキュアストアに、ChatGPT（Codex）のサインインは `<アプリデータ>/codex/auth.json` に保管します。認証情報はリクエストの一部として当該プロバイダーにのみ送信されます。
- **プロバイダーがそのデータをどう扱うか**は、その企業のプライバシーポリシーおよび利用規約に従い、テキストを保持するか学習に使用するかもここに含まれます。Linetta がその企業に代わって約束することはできませんので、選んだプロバイダーの方針をご自身でご確認ください。
- 開発者はこのデータを受信・中継・保存しません。データはユーザーの端末からユーザーのプロバイダーへ直接送られます。

#### 3.2 外部エージェントの接続 (MCP)
`設定 → 外部エージェント接続 (MCP)` で明示的に同意して有効にすると、`127.0.0.1` のみにバインドされるローカルエンドポイントが開きます。同じ端末で実行している MCP クライアント（Claude Code、Claude Desktop など）が接続し、原稿・作品構造・ファクトカードを読み取り、フルアクセスモードでは変更できます。

- この経路で読み取られたデータの受信者は**ユーザーが選んだそのクライアント**です。そのクライアントが自社プロバイダーへ何を送るかは、当該製品のプライバシーポリシーに従います。
- エンドポイントは既定でオフで、同意チェックと有効化を経てのみ開きます。オフにすると直ちに閉じます。
- 接続には本アプリがローカルで生成したトークンが必要で、いつでも再発行できます。
- リモートからの接続はできません。他の端末からこのエンドポイントには到達できません。
- エージェントが行った変更はすべて活動ログに記録され、変更前のスナップショットが保存されるため元に戻せます。
- 目的は、ユーザーが要求した生成・修正・要約・調査の補助を提供することです。
- 開発者はこのデータを受信・中継・保存しません。

#### 3.3 GitHub 同期
GitHub 同期は、`設定 → GitHub 同期` でユーザーが git フォルダを指定するまでオフです。指定すると自動的に動作し、1 日 1 回、現在開いている作品だけでなく**すべて**の作品を Markdown ファイルとしてそのフォルダに書き出し、コミットしてプッシュします。macOS App Store ビルドにこの機能はなく、それ以外のすべてのデスクトップビルドに含まれます。

- **どこへ送られるか。** その git リポジトリに設定されたリモートにのみ送られ、それ以外のどこにも送られません。つまりユーザーの git ホスト上にあるユーザーのリポジトリであり、そのホストのプライバシーポリシーおよび利用規約に従います。選んだリポジトリが非公開かどうかをご確認ください。リモートのないリポジトリなら、ローカルにコミットされるだけでどこにもプッシュされません。
- **何が送られるか。** ライブラリにあるすべての作品が、原稿・アウトライン・設定資料を含めて Markdown として書き出されます。
- **認証**は、SSH キーや資格情報ヘルパーなど既存のシステムの git 設定をそのまま使用します。Linetta は git の認証情報を要求も保存もしません。
- そのフォルダにある MCP クライアントの設定ファイルは意図的にステージングから除外するため、そこに含まれる接続トークンがコミット・プッシュされることはありません。
- 開発者はこのデータを受信・中継・保存しません。

### 4. データの販売・共有
開発者はユーザーデータを販売せず、開発者自身の目的で共有しません。原稿が他者に届く経路は、ユーザー自身が有効にする三つだけです。内蔵エージェントはユーザーが接続したプロバイダーへ原稿を送信し、ユーザーが実行する MCP クライアントは原稿を読み取り、GitHub 同期はユーザーの git リモートへ原稿をプッシュします。いずれも第3項のとおりです。

### 5. 子どものプライバシー
開発者は子どもを含むユーザーの個人情報を収集しません。子どもの利用者は、接続する AI プロバイダーおよび MCP で接続するクライアントそれぞれの年齢要件および保護者同意要件も満たす必要があります。

### 6. 変更
本ポリシーが変更された場合、このページを更新し、上部の「最終更新」日を改訂します。

### 7. お問い合わせ
お問い合わせ: devlikebear@gmail.com
