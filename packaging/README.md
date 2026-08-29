# Distribution packaging

Linetta uses GitHub Releases as the canonical source for desktop installers.
The release workflow publishes:

- macOS app archive for the Homebrew cask
- Windows NSIS installer and MSI bundle
- Linux AppImage, deb, and rpm bundles
- `SHA256SUMS`
- a tarball of rendered winget manifests
- a Flathub manifest starter
- `linetta-mcp-macos`, `linetta-mcp-linux`, `linetta-mcp-windows.exe`

The `linetta-mcp` binaries are the stdio MCP bridge Claude Desktop launches.
Direct-download builds already bundle it inside the app, and the settings pane
prints that bundled path. It also ships standalone because Mac App Store builds
deliberately leave it out: `tauri.mas.conf.json` clears `bundle.resources`, so a
sandboxed install has nothing to point Claude Desktop at. Those writers download
the binary here or install it from Homebrew.

The standalone macOS binary is signed but not notarized, so a browser download
carries a quarantine flag. Homebrew is the path that avoids it.

Windows desktop builds package the embedded Go engine as
`linetta_engine_ffi.dll` and load it at runtime from the Tauri resource
directory. This avoids linking Go's `c-archive` output into the MSVC Tauri
binary while preserving the embedded-engine runtime boundary.

`bundle.targets` in `tauri.conf.json` is `["app", "dmg"]` — macOS only. The
release workflow passes `--bundles` per platform instead, which is why releases
carry NSIS, MSI, AppImage, deb and rpm. It also means a bare `pnpm tauri build`
on Windows or Linux produces the executable and no installer, without saying so.
To get one locally, name the target:

```sh
pnpm tauri build --config src-tauri/tauri.windows.conf.json
pnpm tauri bundle --bundles nsis --config src-tauri/tauri.windows.conf.json
```

## macOS Homebrew cask

The Homebrew cask uses `Linetta-macos.app.tar.gz` from the GitHub Release.
Until Linetta is ready for a paid Apple Developer Program membership, the cask
installs the ad-hoc signed app and clears the quarantine attribute in
`postflight`. This avoids the hobby-release Gatekeeper "damaged app" prompt for
Homebrew installs without requiring users to run `xattr` manually.

When the project has enough expected users to justify it, switch to Developer
ID signing and notarization. The release workflow already supports that path:
if the Apple secrets below are configured, the macOS job signs the Tauri app
bundle with the embedded engine linked in-process, submits it to Apple's notary
service, staples the ticket, validates the result, and only then creates the
Homebrew tarball and checksum.

Configure these GitHub Actions secrets to enable notarized macOS releases:

- `APPLE_CERTIFICATE`: base64-encoded `.p12` Developer ID Application certificate
- `APPLE_CERTIFICATE_PASSWORD`: password for the exported `.p12`
- `APPLE_API_ISSUER`: App Store Connect issuer ID
- `APPLE_API_KEY`: App Store Connect API key ID
- `APPLE_API_KEY_BASE64`: base64-encoded App Store Connect `.p8` private key
- `APPLE_TEAM_ID`: Apple Developer team ID

Optional secrets:

- `APPLE_SIGNING_IDENTITY`: exact keychain identity if more than one Developer ID Application certificate is present
- `KEYCHAIN_PASSWORD`: temporary CI keychain password; generated automatically when omitted

Export the certificate from Keychain Access, then encode it with:

```sh
openssl base64 -A -in DeveloperIDApplication.p12 -out certificate-base64.txt
```

## Windows winget

The release workflow renders `Linetta-winget-manifests.tar.gz` from the
templates in `packaging/winget`. Linetta is not in `microsoft/winget-pkgs` yet,
so `winget install Linetta` does not work; Windows users download from the
GitHub release (#46).

Manual render:

```sh
scripts/render-winget-manifest.sh \
  1.0.0 \
  https://github.com/devlikebear/linetta/releases/download/v1.0.0 \
  Linetta_1.0.0_x64-setup.exe \
  SHA256_FROM_RELEASE \
  dist/winget
```

### Submitting

The first submission needs a public release, because the manifest names an
`InstallerUrl` that must resolve and an `InstallerSha256` that must match what
is behind it — the check winget rejects most often. v1.0.0 is out, so it can go.

1. Take the SHA from the release's `SHA256SUMS`, not from a local build. A
   local build and the released binary are not byte-identical.
2. Render the three manifests with the command above.
3. `winget validate --manifest dist/winget` — schema only, no network.
4. `winget install --manifest dist/winget` on a clean machine, to prove the
   installer switches and `Scope: user` are right.
5. Fork `microsoft/winget-pkgs`, copy the three files to
   `manifests/d/Devlikebear/Linetta/1.0.0/`, and open the upstream PR.

Steps 1-3 are done for 1.0.0, against the published release rather than a local
build:

- `InstallerUrl` resolves (HTTP 200, 20,704,058 bytes).
- `InstallerSha256` is `84C88C01FF0D89F3ED2B6ED7F55DCB3E5E1FF2FC48484BFA06551406A12423E8`,
  which matches both the release `SHA256SUMS` and a fresh download of the file
  the URL actually serves.
- `winget validate --manifest` passes against winget 1.12.

Step 4 is belt-and-braces: the upstream PR runs the manifest on a clean VM as
part of its own validation. Step 5 is the submission itself.

Expect the first review to take a while: the installer is unsigned, so
SmartScreen warns. Signing is not required for a winget submission, but its
absence is the sort of thing a first-time reviewer asks about.

### After the first submission

Every later release needs a version-bump PR upstream. Keep this manual until
the first submission is accepted — the automation is a `wingetcreate update`
step in `release.yml`, but it needs a PAT with fork access to
`microsoft/winget-pkgs` stored as a repository secret, and there is no point
holding that secret for a package that has not been accepted yet.

## Linux Flathub

`packaging/flathub/com.devlikebear.linetta.yml` is a Flathub submission
starter, not a direct submit-ready manifest. Before opening the Flathub PR:

- pin the release commit instead of `REPLACE_WITH_RELEASE_COMMIT`
- generate offline source manifests for Cargo, Go, and pnpm dependencies
- run `flatpak-builder --user --install --force-clean build-dir packaging/flathub/com.devlikebear.linetta.yml`
- update the appstream metadata if the product description or license changes

## Linux direct packages

The release workflow uploads AppImage, deb, and rpm files. Those direct files
are enough for the first Linux release. A signed apt/dnf repository can be added
later without changing the Tauri bundle format.

## Mac App Store review notes

The sandboxed build carries `com.apple.security.network.server` because Linetta
can open a local MCP endpoint for external agents. Points to make in the review
notes, since a listening socket invites questions:

- The listener binds `127.0.0.1` only. It is not reachable from another machine
  and there is no remote or cloud component.
- It is off by default and stays off until the user ticks an explicit consent
  box in Settings and presses Enable. Turning it off stops the listener.
- Every request must carry a bearer token the app generates locally. The user
  can regenerate it at any time, which invalidates the old one.
- It exists so the user can drive their own writing with a client they already
  run on the same Mac, such as Claude Code or Claude Desktop.
- App Store builds do not bundle the `linetta-mcp` bridge binary. Nothing is
  downloaded or executed on the user's behalf.

Sandbox verification still has to happen on a Mac: `make build-mas-local`, then
enable MCP and confirm the loopback listener actually binds under the sandbox.

## Mobile app artifacts

`.github/workflows/mobile-engine.yml` verifies the embedded Go engine under the
`mobile` build tag and uploads raw iOS `xcframework` / Android `jniLibs`
artifacts. This is the CI gate for the shared engine before it is wired into the
Tauri-generated mobile projects.

`.github/workflows/mobile-release.yml` is a manual release path for signed
mobile app packages. It initializes the ignored Tauri mobile project in CI,
builds the embedded engine, builds an iOS `.ipa` or Android `.aab`/`.apk`,
uploads the workflow artifact, and optionally uploads it to an existing GitHub
Release tag when `release_tag` is provided.

Required iOS secrets:

- `APPLE_API_ISSUER`
- `APPLE_API_KEY`
- `APPLE_API_KEY_BASE64`
- `APPLE_TEAM_ID`

Required Android secrets:

- `ANDROID_KEY_BASE64`: base64-encoded upload keystore
- `ANDROID_KEY_ALIAS`
- `ANDROID_KEY_PASSWORD`

The mobile release workflow is intentionally manual because store distribution
still depends on real signing credentials, device/simulator smoke tests, and
store-track decisions. Android signing is patched into the generated Gradle
project by `scripts/patch-tauri-android-signing.sh`, which makes the release
build load `keystore.properties` as described by the Tauri Android signing
guide.

Current local status:

- iOS simulator no-sign app bundle generation, install, and launch are verified
  with Xcode's matching iOS 26.5 simulator runtime. The launched app creates
  `library.db` under the simulator app container, proving the embedded engine
  starts in the iOS sandbox. Re-run this with `make smoke-mobile-ios-sim`.
- Android arm64 debug APK generation is verified with Tauri and the embedded Go
  engine.
- Android release APK/AAB signing is smoke-tested locally with
  `scripts/build-android-release-smoke.sh`, which uses a temporary keystore and
  the same generated Gradle signing hook as CI.
- iOS Rust target linking is verified for simulator and device. Full signed
  `.ipa` export still requires Apple team/certificate/provisioning credentials.
