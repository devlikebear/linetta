# Linetta 개인정보 처리방침 / Privacy Policy

_최종 업데이트 / Last updated / 最終更新: 2026-09-03_

---

## 한국어

Linetta("앱")는 장편소설·웹소설 집필을 위한 **로컬 우선(local-first)** 데스크톱 앱입니다. 개발자(Changheon Shin / devlikebear)는 분석·광고·계정 서버를 운영하지 않으며 사용자 데이터를 수집하지 않습니다. 앱 자체는 어떤 언어 모델과도 통신하지 않습니다. 다만 사용자가 MCP 기능을 켜면, 사용자가 직접 실행하는 외부 클라이언트가 아래와 같이 원고를 읽어 갈 수 있습니다.

### 1. 수집하는 정보
**개발자가 수집하는 정보는 없습니다.** 앱은 다음을 하지 않습니다.
- 사용자 계정 또는 로그인이 없습니다.
- 분석(analytics)·추적(tracking)·텔레메트리·광고 식별자를 사용하지 않습니다.
- 크래시 리포팅 등으로 사용자 데이터를 개발자 서버로 보내지 않습니다.

### 2. 데이터 저장 위치
사용자가 작성한 원고·프로젝트·설정 등 기본 데이터는 **사용자 기기에 로컬로** 저장됩니다.
- 집필 데이터: 기기의 로컬 데이터베이스
- MCP 접속 토큰 등 민감 정보: 운영체제의 보안 저장소(예: macOS Keychain)

개발자는 이 데이터에 접근할 수 없습니다.

ChatGPT(Codex)로 로그인하면 접근 토큰과 갱신 토큰이 `<앱 데이터>/codex/auth.json`에 저장됩니다. 이 파일은 소유자만 읽을 수 있는 권한(0600)으로 저장되며, 이는 Codex CLI가 자기 토큰을 보관하는 방식과 같습니다. Linetta는 이 파일을 어디로도 보내지 않으며, OpenAI에 요청을 보낼 때만 사용합니다.

macOS App Store 빌드가 아닌 빌드에서는, 이미 Codex CLI로 로그인해 둔 것을 그대로 인정합니다. `<앱 데이터>/codex/auth.json`이 없고 `~/.codex/auth.json`이 있으면 Linetta가 그 파일을 대신 사용하므로 두 번 로그인하지 않아도 됩니다. macOS App Store 빌드는 샌드박스 컨테이너 밖을 읽을 수 없어 이 파일을 사용하지 않습니다.

접근 토큰이 만료되면 Linetta가 토큰을 갱신하면서 그때 사용 중인 파일을 다시 씁니다. Codex CLI 로그인을 사용하고 있다면 `~/.codex/auth.json`도 여기에 포함됩니다.

설정에서 로그아웃하면 Linetta 자신의 파일인 `<앱 데이터>/codex/auth.json`이 삭제됩니다. `~/.codex/auth.json`에 있는 Codex CLI 로그인은 삭제하지 않으므로, 그쪽은 Codex CLI에서 직접 로그아웃해야 합니다.

macOS App Store 빌드를 포함한 모든 빌드가 이 파일 저장 방식을 사용하며, 이 토큰을 시스템 키체인에 저장하지 않습니다.

### 3. 외부 에이전트 연결 (MCP)
**앱은 어떤 AI 공급자에게도 데이터를 보내지 않습니다.** API 키를 저장하지도, 모델을 호출하지도 않습니다.

사용자가 `설정 → 외부 에이전트 연결(MCP)`에서 명시적으로 동의하고 켜면, 앱이 `127.0.0.1`에만 바인딩되는 로컬 엔드포인트를 엽니다. 사용자가 자기 기기에서 실행하는 MCP 클라이언트(예: Claude Code, Claude Desktop)가 여기에 접속해 원고·작품 구조·팩트 카드를 읽고, 전체 접근 모드에서는 수정할 수 있습니다.

- 이 경로로 전송되는 데이터의 수신자는 **사용자가 선택한 그 클라이언트**이며, 그 클라이언트가 자기 공급자에게 무엇을 보내는지는 해당 제품의 개인정보 처리방침을 따릅니다.
- 엔드포인트는 기본적으로 꺼져 있고, 동의 체크와 활성화를 거쳐야 열립니다. 앱에서 끄면 즉시 닫힙니다.
- 접속에는 앱이 로컬에서 생성한 토큰이 필요하며, 사용자는 언제든 재발급할 수 있습니다.
- 원격 접속은 불가능합니다. 다른 기기에서 이 엔드포인트에 닿을 수 없습니다.
- 에이전트가 수행한 모든 변경은 앱의 활동 기록에 남고, 변경 전 스냅샷이 저장되어 되돌릴 수 있습니다.
- 개발자는 이 데이터를 수신·중계·저장하지 않습니다.

### 4. 데이터 판매·공유
개발자는 사용자 데이터를 판매하거나 자체 목적으로 공유하지 않습니다. 단, 사용자가 명시적으로 동의해 MCP를 켜면 위 3항과 같이 사용자가 실행하는 클라이언트가 원고를 읽어 갑니다.

### 5. 아동의 개인정보
개발자는 아동을 포함한 사용자의 개인정보를 수집하지 않습니다. 아동 사용자는 MCP로 연결하는 클라이언트의 연령 요건과 보호자 동의 요건도 확인해야 합니다.

### 6. 변경 사항
본 방침이 변경되면 이 페이지를 갱신하고 상단의 "최종 업데이트" 날짜를 수정합니다.

### 7. 문의
문의: devlikebear@gmail.com

---

## English

Linetta (the "App") is a **local-first** desktop app for writing long-form fiction. The developer (Changheon Shin / devlikebear) operates no analytics, advertising, or account server and does not collect user data. The App itself never contacts a language model. If you turn on the optional MCP endpoint, a client you run yourself can read your manuscript as described below.

### 1. Information We Collect
**The developer collects none.** The App:
- has no user accounts or sign-in;
- uses no analytics, tracking, telemetry, or advertising identifiers;
- does not send any user data to the developer (including crash reports).

### 2. Where Data Is Stored
Core data you create — manuscripts, projects, and settings — is stored **locally on your device**.
- Writing data: a local database on your device.
- Sensitive values such as the MCP connection token: your operating system's secure store (e.g., macOS Keychain).

The developer has no access to this data.

Signing in to ChatGPT (Codex) stores an access token and a refresh token at `<app data>/codex/auth.json`. This file is saved with owner-only read permissions (0600) — the same way the official Codex CLI keeps its own token. Linetta sends this file nowhere; it is used only for requests to OpenAI.

Outside the macOS App Store build, Linetta also honours a sign-in you already completed with the Codex CLI: when there is no `<app data>/codex/auth.json` but `~/.codex/auth.json` exists, Linetta uses that file instead, so you do not have to sign in twice. The macOS App Store build cannot read outside its sandbox container and never uses it.

When an access token expires, Linetta refreshes it and rewrites whichever of those two files is in use — including `~/.codex/auth.json`, when the Codex CLI's sign-in is the one being used.

Signing out from Settings deletes Linetta's own file, `<app data>/codex/auth.json`. It does not delete a Codex CLI sign-in at `~/.codex/auth.json`; sign that one out with the Codex CLI itself.

Every build, including the macOS App Store build, uses this file store — Linetta does not put this token in the system keychain.

### 3. Connecting an External Agent (MCP)
**The App sends nothing to any AI provider.** It stores no API keys and calls no models.

When you explicitly consent and enable it under `Settings → Connect an external agent (MCP)`, the App opens a local endpoint bound to `127.0.0.1`. An MCP client you run on the same machine (such as Claude Code or Claude Desktop) can then read your manuscript, story structure, and fact cards, and in full-access mode modify them.

- The recipient of anything read this way is **the client you chose**. What that client sends onward to its own provider is governed by that product's privacy policy.
- The endpoint is off by default and opens only after you tick the consent box and enable it. Turning it off closes it immediately.
- Connecting requires a token the App generates locally, which you can regenerate at any time.
- There is no remote access: the endpoint is unreachable from another machine.
- Every change an agent makes is recorded in the App's activity log and snapshotted beforehand so you can undo it.
- The purpose is to provide generation, revision, summarization, or research assistance you request.
- The developer does not receive, relay, or store this data.

### 4. Selling or Sharing Data
The developer does not sell user data or share it for the developer's own purposes. When you explicitly consent and enable MCP, a client you run reads your manuscript as described in Section 3.

### 5. Children's Privacy
The developer collects no personal data from users, including children. Child users must also satisfy the age and parental-consent requirements of any client they connect over MCP.

### 6. Changes
If this policy changes, we will update this page and revise the "Last updated" date above.

### 7. Contact
Contact: devlikebear@gmail.com

---

## 日本語

Linetta（以下「本アプリ」）は、長編小説・Web小説の執筆のための**ローカルファースト**なデスクトップアプリです。開発者（Changheon Shin / devlikebear）は、分析・広告・アカウント用サーバーを運営せず、ユーザーデータを収集しません。本アプリ自体はいかなる言語モデルとも通信しません。ただし、ユーザーが MCP 機能を有効にすると、ユーザー自身が実行する外部クライアントが以下のとおり原稿を読み取ることがあります。

### 1. 収集する情報
**開発者が収集する情報はありません。** 本アプリは以下を行いません。
- ユーザーアカウントやログインはありません。
- 分析（analytics）・トラッキング・テレメトリ・広告識別子を使用しません。
- クラッシュレポートなどでユーザーデータを開発者のサーバーへ送信しません。

### 2. データの保存場所
ユーザーが作成した原稿・プロジェクト・設定などの基本データは、**ユーザーの端末内にローカルで**保存されます。
- 執筆データ: 端末内のローカルデータベース
- MCP 接続トークンなどの機密情報: OS のセキュアストア（例: macOS Keychain）

開発者はこのデータにアクセスできません。

ChatGPT（Codex）にサインインすると、アクセストークンとリフレッシュトークンが `<アプリデータ>/codex/auth.json` に保存されます。このファイルは所有者のみが読み取れる権限（0600）で保存され、これは Codex CLI が自身のトークンを保管する方式と同じです。Linetta はこのファイルをどこにも送信せず、OpenAI へのリクエストにのみ使用します。

macOS App Store ビルド以外では、すでに Codex CLI で済ませたサインインをそのまま利用します。`<アプリデータ>/codex/auth.json` がなく `~/.codex/auth.json` がある場合、Linetta はそのファイルを代わりに使用するため、二度サインインする必要はありません。macOS App Store ビルドはサンドボックスコンテナの外を読み取れないため、このファイルを使用しません。

アクセストークンの有効期限が切れると、Linetta はトークンを更新し、そのとき使用しているファイルを書き換えます。Codex CLI のサインインを使用している場合は `~/.codex/auth.json` もその対象です。

設定からログアウトすると、Linetta 自身のファイルである `<アプリデータ>/codex/auth.json` が削除されます。`~/.codex/auth.json` にある Codex CLI のサインインは削除しないため、そちらは Codex CLI 自身でログアウトしてください。

macOS App Store ビルドを含むすべてのビルドがこのファイル保存方式を使用し、このトークンをシステムキーチェーンには保存しません。

### 3. 外部エージェントの接続 (MCP)
**本アプリはいかなる AI プロバイダーにもデータを送信しません。** API キーを保存せず、モデルを呼び出しません。

`設定 → 外部エージェント接続 (MCP)` で明示的に同意して有効にすると、`127.0.0.1` のみにバインドされるローカルエンドポイントが開きます。同じ端末で実行している MCP クライアント（Claude Code、Claude Desktop など）が接続し、原稿・作品構造・ファクトカードを読み取り、フルアクセスモードでは変更できます。

- この経路で読み取られたデータの受信者は**ユーザーが選んだそのクライアント**です。そのクライアントが自社プロバイダーへ何を送るかは、当該製品のプライバシーポリシーに従います。
- エンドポイントは既定でオフで、同意チェックと有効化を経てのみ開きます。オフにすると直ちに閉じます。
- 接続には本アプリがローカルで生成したトークンが必要で、いつでも再発行できます。
- リモートからの接続はできません。他の端末からこのエンドポイントには到達できません。
- エージェントが行った変更はすべて活動ログに記録され、変更前のスナップショットが保存されるため元に戻せます。
- 開発者はこのデータを受信・中継・保存しません。

### 4. データの販売・共有
開発者はユーザーデータを販売せず、開発者自身の目的で共有しません。ただし、ユーザーが明示的に同意して MCP を有効にすると、第3項のとおりユーザーが実行するクライアントが原稿を読み取ります。

### 5. 子どものプライバシー
開発者は子どもを含むユーザーの個人情報を収集しません。子どもの利用者は、選択したAIプロバイダーの年齢要件および保護者同意要件も満たす必要があります。

### 6. 変更
本ポリシーが変更された場合、このページを更新し、上部の「最終更新」日を改訂します。

### 7. お問い合わせ
お問い合わせ: devlikebear@gmail.com
