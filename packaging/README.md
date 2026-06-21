# Distribution packaging

Linetta uses GitHub Releases as the canonical source for desktop installers.
The release workflow publishes:

- macOS app archive for the Homebrew cask
- Linux AppImage, deb, and rpm bundles
- `SHA256SUMS`
- a Flathub manifest starter

Windows desktop bundles are temporarily excluded from the canonical release
workflow while the embedded Go engine archive is resolved for the MSVC linker.
When a Windows NSIS installer is present in `dist`, the same workflow also
renders `Linetta-winget-manifests.tar.gz`.

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

When a Windows NSIS installer is included in the GitHub Release assets, the
release workflow renders `Linetta-winget-manifests.tar.gz` from the templates in
`packaging/winget`. After the first public Windows release, unpack that tarball
into a fork of `microsoft/winget-pkgs` under the package path selected by
`wingetcreate`, validate it, and open the upstream pull request.

Manual render example:

```sh
scripts/render-winget-manifest.sh \
  0.4.0 \
  https://github.com/devlikebear/linetta/releases/download/v0.4.0 \
  Linetta_0.4.0_x64-setup.exe \
  SHA256_FROM_RELEASE \
  dist/winget
```

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
