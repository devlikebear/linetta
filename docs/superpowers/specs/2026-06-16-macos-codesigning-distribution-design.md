# Linetta macOS 코드 서명 & 배포 설계

- 작성일: 2026-06-16
- 상태: 설계 승인 대기
- 대상: Linetta 데스크톱 앱(Tauri + Go 사이드카), macOS 배포

## 배경

Apple Developer Program 등록 완료. macOS 빌드를 정식 서명/공증해 Gatekeeper 경고 없이
설치 가능하게 만들고, 향후 Mac App Store(MAS) 배포까지 확장한다.

코드 인프라는 이미 상당 부분 존재:

- `scripts/sign-notarize-macos-app.sh` — Developer ID 서명 + `notarytool` 공증 + stapling
- `.github/workflows/build.yml` — `APPLE_*` 시크릿이 있으면 CI에서 자동 서명/공증
- 배포 채널: GitHub Releases + Homebrew cask + winget + flathub

빠진 것: **실제 Apple 인증서 발급**, **로컬 서명 플로우**, **서명된 .dmg**, **App Store Connect API 키 기반 인증**.

## 결정 사항 (확정)

- 배포 경로: **Developer ID(직접 배포)와 Mac App Store(MAS) 둘 다** 지원
- 진행 순서: **Phase 1 = Developer ID 먼저**, **Phase 2 = MAS 다음**
- 인증 방식: **App Store Connect API 키(.p8)**
- Developer ID 설치 형식: **서명된 .dmg 추가** (기존 .app.tar.gz 경로는 유지)

## 자격증명 (보유 상태)

- `.p8`: `~/.linetta/apple/AuthKey_Z8W67QU9X9.p8` (권한 600, 디렉터리 700)
- **Key ID**: `Z8W67QU9X9`
- **Issuer ID / Team ID**: 확보됨 — `~/.linetta/apple/config.env`에 보관 (git 미커밋)
- API 키 역할: 관리자(Admin)

비밀이 아닌 메타데이터(Key ID / Issuer ID / Team ID)는 설정 파일로,
`.p8`와 인증서 개인키는 파일/keychain에만 둔다. 어떤 비밀도 git에 커밋하지 않는다.

---

## Phase 1 — Developer ID 직접 배포

### 1. 자격증명 설정

- `~/.linetta/apple/`에 비밀이 아닌 설정 파일(`config.env` 또는 유사)로
  `APP_STORE_CONNECT_KEY_ID`, `APP_STORE_CONNECT_ISSUER_ID`, `APPLE_TEAM_ID`, `AUTH_KEY_PATH` 보관
- `notarytool`용 keychain 프로파일을 1회 저장:
  `xcrun notarytool store-credentials --key <.p8> --key-id <id> --issuer <id>`
  → 이후 공증은 프로파일 이름만으로 인증

### 2. Developer ID Application 인증서 발급

- 로컬에서 keypair + CSR 생성 (`openssl`). **개인키는 로컬에만 보관**
- 발급 경로(우선순위):
  1. App Store Connect API로 `DEVELOPER_ID_APPLICATION` 인증서 생성 시도
  2. **폴백**: 거부되면 developer.apple.com 웹 포털에서 CSR 업로드로 1회 발급
     (Developer ID 인증서는 Account Holder에 묶여 API가 막힐 수 있음)
- 발급된 `.cer`를 keychain에 import → 로컬 개인키와 자동 매칭
- `security find-identity -v -p codesigning`에 `Developer ID Application: ...` 등장 확인

> 구현 시점에 API 발급 가능 여부를 실제로 검증하고, 결과에 따라 경로를 확정한다.

### 3. 로컬 빌드/서명/공증 플로우

- 기존 `scripts/sign-notarize-macos-app.sh` 확장:
  1. **DMG 서명 지원** 추가 (`.app`뿐 아니라 `.dmg`도 서명/공증/staple)
  2. **`notarytool`를 API 키(`--keychain-profile` 또는 `--key`)로 인증** —
     기존 `APPLE_ID`/`APPLE_PASSWORD` 경로는 하위호환으로 유지
- 새 스크립트 `scripts/release-macos-local.sh`:
  엔진 빌드 → `pnpm tauri build`(app+dmg) → 서명 → 공증 → staple → 검증
- `Makefile`에 `release-macos-local` 타깃 추가

### 4. 산출물

- `Linetta.app` — Developer ID 서명됨
- `Linetta_<version>_aarch64.dmg` — 서명 + 공증 + stapled, 더블클릭 설치, Gatekeeper 경고 없음

### 5. 검증 (완료 기준)

- `codesign --verify --deep --strict --verbose=2 Linetta.app` 통과
- `spctl -a -vvv -t exec Linetta.app` → accepted
- `stapler validate` (앱/DMG 모두) 통과
- 새 macOS 환경에서 DMG 설치 후 경고 없이 실행

### 6. CI 연동 (선택)

- 로컬 검증 후, 동일 자격증명을 GitHub Secrets로 옮겨 기존 워크플로의 서명 단계 활성화
- macOS job에 dmg 번들 타깃 + dmg 서명/공증 반영

---

## Phase 2 — Mac App Store (개요, 별도 spec으로 상세화)

리스크가 크므로 별도 설계 문서로 분리한다. 큰 줄기:

- **샌드박스 호환성 감사**: SQLite DB 및 사용자 파일 경로가 앱 컨테이너 안에서 동작하는지 확인,
  사용자 지정 폴더(가져오기/내보내기) 접근은 security-scoped bookmark로 전환
- **엔타이틀먼트**: App Sandbox, Go 사이드카에 `com.apple.security.inherit`,
  LLM API 호출용 `com.apple.security.network.client`
- **인증서(API 발급)**: Apple Distribution + Mac Installer Distribution
- **App Store Connect 앱 레코드** 생성 (bundle id `com.devlikebear.linetta` 등록)
- **패키징/업로드**: `productbuild`로 `.pkg` → `xcrun notarytool`/Transporter 업로드

## 주요 리스크

- Developer ID 인증서 API 발급이 막힐 수 있음 → 웹 포털 폴백으로 해결
- (Phase 2) App Sandbox + Go 사이드카 실행 조합의 심사 통과 리스크
- 자격증명 유출 방지: `.p8`·개인키·`.env`는 절대 커밋하지 않음 (`.gitignore` 확인)

## 비범위 (Out of scope)

- Intel(x86_64) 빌드 — 현재 Apple Silicon 전용 유지
- 앱 메타데이터/스크린샷 작성, 실제 심사 제출 (사용자 수동 영역)
