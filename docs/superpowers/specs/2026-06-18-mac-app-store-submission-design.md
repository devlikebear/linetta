# Linetta Mac App Store 제출 (ASC 업로드까지) 설계

- 작성일: 2026-06-18
- 상태: 설계 승인됨 (사용자 리뷰 대기)
- 대상: Linetta 데스크톱 앱(Tauri v2 + Go 사이드카), Mac App Store 배포
- 선행:
  - [Phase 1 — Developer ID 직접 배포](2026-06-16-macos-codesigning-distribution-design.md) (완료, main)
  - [Phase 2-A — App Sandbox 호환화](2026-06-17-macos-app-sandbox-mas-design.md) (완료, main)

## 배경

Phase 2-A에서 App Sandbox 호환 빌드(`mas` 빌드 태그)를 완성하고, 기존 Developer ID
인증서 + sandbox 엔타이틀먼트로 로컬 검증까지 마쳤다. Phase 2-B는 실제 Mac App Store
제출 파이프라인을 만든다. MAS는 Developer ID 배포와 서명·패키징·배포 채널이 다르다:

- 앱 서명: **Apple Distribution** 인증서 (Developer ID 아님)
- **provisioning profile** 내장 필수 (`Contents/embedded.provisionprofile`)
- 엔타이틀먼트에 `com.apple.application-identifier` 필요
- 패키징: `productbuild`로 `.pkg`, **Mac Installer Distribution** 인증서로 서명
- 배포: 공증(notarization) 대신 App Store Connect 업로드 → Apple 심사
- 보유 자산: ASC API 키(.p8, Admin), Team ID `2QW8S2B594`, bundle id `com.devlikebear.linetta`

## 결정 사항 (확정)

- **범위**: App Store Connect **업로드 성공(검증 통과)까지**. 메타데이터/심사 제출은 비범위(수동).
- **Apple 셋업**: **하이브리드** — API로 인증서/App ID/provisioning profile 생성, 앱 레코드는 수동.
- **파이프라인**: **로컬 전용** (`scripts/release-mas-local.sh` + make 타깃). CI는 후속.
- **앱 이름**: App Store Connect 표시 이름은 사용자가 앱 레코드 생성 시 결정(중복 불가).

## 목표 / 비범위

**목표**: MAS용 `.pkg`(Apple Distribution 서명 앱 + MAS provisioning profile 내장 +
Mac Installer Distribution 서명 pkg)를 로컬에서 빌드·검증하고 App Store Connect에
업로드 성공(빌드가 "처리 중"으로 표시)까지.

**비범위**:

- App Store 메타데이터/스크린샷/심사 제출 (수동, 창의적 영역)
- CI 연동 (후속)
- Intel(x86_64) — Apple Silicon 전용 유지
- TestFlight 외부 베타 구성

---

## 1. Apple 셋업 (하이브리드, 1회성)

### API로 생성 (ASC API 키 + 스크립트)

`~/.linetta/apple/config.env`의 기존 자격증명(`APP_STORE_CONNECT_KEY_ID`,
`APP_STORE_CONNECT_ISSUER_ID`, `APPLE_TEAM_ID`, `AUTH_KEY_PATH`)으로 ES256 JWT를
만들어 App Store Connect API를 호출한다.

1. **App ID 등록**: `POST /v1/bundleIds` (identifier `com.devlikebear.linetta`,
   platform MAC_OS). App Sandbox는 별도 capability 활성화가 필요 없을 수 있음(기본
   포함) — 구현 시 `bundleIdCapabilities`로 확인/활성화.
2. **Apple Distribution 인증서**: 로컬에서 RSA keypair + CSR 생성(`openssl`) →
   `POST /v1/certificates` (`certificateType: DISTRIBUTION`) → `.cer` 다운로드 →
   `.p12` 변환(OpenSSL 3는 `-legacy -macalg sha1`) → keychain import.
   **개인키는 `~/.linetta/apple/`에만 보관·백업.**
3. **Mac Installer Distribution 인증서**: 동일 흐름,
   `certificateType: MAC_INSTALLER_DISTRIBUTION`.
4. **MAC_APP_STORE provisioning profile**: `POST /v1/profiles`
   (`profileType: MAC_APP_STORE`, App ID + Distribution 인증서 관계) →
   `embedded.provisionprofile` 다운로드 → `~/.linetta/apple/`에 보관.
5. 인증서 keychain import 후 `security find-identity -v -p codesigning`에
   `Apple Distribution: ...` / `Mac Installer Distribution: ...` (또는 레거시 명칭
   `3rd Party Mac Developer Application/Installer`) 등장 확인.

> 구현 시 각 API 호출 가능 여부를 실제로 검증하고, 거부되면(Phase 1의 Developer ID
> 처럼) developer.apple.com 웹 포털 폴백으로 1회 발급한다.

### 수동 (사용자, App Store Connect 포털)

- **앱 레코드 생성**: App Store Connect → 앱 추가 → 플랫폼 macOS, bundle id
  `com.devlikebear.linetta` 선택, 표시 이름/기본 언어/SKU/가격 설정.
  (이름/가격은 사람 판단이라 수동.) 업로드 전에 반드시 존재해야 한다.
- 구현 단계에서 단계별 가이드를 제공한다.

## 2. MAS 엔타이틀먼트

신규 `apps/desktop/src-tauri/entitlements/linetta-mas.entitlements`:

- `com.apple.security.app-sandbox` = true
- `com.apple.security.network.client` = true
- `com.apple.security.files.user-selected.read-write` = true
- `com.apple.application-identifier` = `2QW8S2B594.com.devlikebear.linetta`
- `com.apple.developer.team-identifier` = `2QW8S2B594`

사이드카는 기존 `linetta-sidecar.entitlements`(app-sandbox + inherit) 유지.
Phase 2-A의 `linetta.entitlements`(application-identifier 없음)는 로컬 Developer ID
샌드박스 테스트용으로 보존한다(별도 파일로 분리해 서로 간섭 없음).

provisioning profile은 서명 전에 `Linetta.app/Contents/embedded.provisionprofile`로
복사한다. 엔타이틀먼트의 application-identifier와 profile이 일치해야 한다.

## 3. 빌드 + 패키징 + 서명 (로컬 스크립트)

`scripts/release-mas-local.sh` 신설 + `Makefile`에 `release-mas-local` 타깃:

1. `LINETTA_BUILD_TAGS=mas bash scripts/build-engine.sh` (git sync 제외 엔진)
2. `pnpm tauri build --config src-tauri/tauri.mas.conf.json --bundles app`
3. `cp <profile> Linetta.app/Contents/embedded.provisionprofile`
4. 서명 순서(inner→outer):
   - 사이드카: `codesign --sign "Apple Distribution: ..." --entitlements linetta-sidecar.entitlements`
   - 앱: `codesign --sign "Apple Distribution: ..." --entitlements linetta-mas.entitlements`
   - (MAS는 hardened runtime 불필요 — `--options runtime` 생략)
5. 패키징: `productbuild --component Linetta.app /Applications --sign "Mac Installer Distribution: ..." Linetta.pkg`
6. 검증: `xcrun altool --validate-app --type macos --file Linetta.pkg --apiKey <id> --apiIssuer <id>` (또는 `pkgutil --check-signature`)

config.env에 인증서 식별자(`MAS_APP_IDENTITY`, `MAS_INSTALLER_IDENTITY`)와
profile 경로(`MAS_PROFILE_PATH`)를 추가한다.

## 4. 업로드 (로컬)

- `.p8` 키를 altool이 찾는 위치(`~/.appstoreconnect/private_keys/AuthKey_<KEYID>.p8`)에
  배치(또는 심볼릭 링크).
- `xcrun altool --upload-app --type macos --file Linetta.pkg
  --apiKey <KEYID> --apiIssuer <ISSUER>`
- App Store Connect → 앱 → TestFlight/빌드 탭에 빌드가 "처리 중"으로 표시되면 성공.

> altool은 deprecated 경고가 있으나 API 키 업로드에 여전히 동작. 실패 시 Transporter
> 앱으로 폴백.

## 5. 자격증명 관리

- 새 개인키(Apple Distribution, Mac Installer Distribution)와 provisioning profile은
  `~/.linetta/apple/`에만 보관하고 **절대 git에 커밋하지 않는다**.
- `config.env`에 비밀이 아닌 식별자/경로만 추가.
- 개인키 분실 시 인증서 폐기·재발급 필요 — 백업 권고.

## 산출물

- API 발급 스크립트(`scripts/setup-mas-apple.sh` 또는 유사) — App ID/인증서/profile
- `entitlements/linetta-mas.entitlements`
- `scripts/release-mas-local.sh` + `make release-mas-local`
- App Store Connect에 업로드되어 처리 중인 빌드 1개
- (수동) App Store Connect 앱 레코드

## 검증 (완료 기준)

- `security find-identity`에 Apple Distribution + Mac Installer Distribution 등장
- `codesign -d --entitlements -` 로 앱에 application-identifier + app-sandbox 확인
- `Linetta.app/Contents/embedded.provisionprofile` 존재 및 profile↔entitlements 일치
- `productbuild` 산출 `.pkg`가 Mac Installer Distribution으로 서명됨
- `xcrun altool --validate-app` 통과
- `xcrun altool --upload-app` 성공 → ASC에 빌드 "처리 중" 표시

## 주요 리스크

- **application-identifier ↔ provisioning profile 불일치** → 서명/업로드 거부 (가장 흔함)
- **앱 레코드 미생성** → 업로드는 되나 빌드가 매칭 안 됨 (수동 선행 필수)
- **altool API 키 경로** → `~/.appstoreconnect/private_keys/`에 키 없으면 인증 실패
- **인증서 API 발급 제약** → 웹 포털 폴백
- **앱 이름 중복** → 앱 레코드 생성 시 사용자가 유일 이름 선택
- **provisioning profile 만료**(보통 1년) → 재발급 필요

## 다음 (후속, 별도 spec 가능)

- App Store 메타데이터/스크린샷 작성 및 심사 제출 (수동 영역)
- MAS 빌드/업로드 CI 연동 (Distribution 인증서 + profile을 GitHub Secrets로)
