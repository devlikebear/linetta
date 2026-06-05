# Distribution packaging

Linetta uses GitHub Releases as the canonical source for desktop installers.
The release workflow publishes:

- macOS app archive for the Homebrew cask
- Windows NSIS installer and MSI bundle
- Linux AppImage, deb, and rpm bundles
- `SHA256SUMS`
- a tarball of rendered winget manifests
- a Flathub manifest starter

## macOS Homebrew cask

The Homebrew cask uses `Linetta-macos.app.tar.gz` from the GitHub Release.
Until Linetta is ready for a paid Apple Developer Program membership, the cask
installs the ad-hoc signed app and clears the quarantine attribute in
`postflight`. This avoids the hobby-release Gatekeeper "damaged app" prompt for
Homebrew installs without requiring users to run `xattr` manually.

When the project has enough expected users to justify it, switch to Developer
ID signing and notarization. The release workflow already supports that path:
if the Apple secrets below are configured, the macOS job signs the Tauri app
bundle and sidecar binaries, submits it to Apple's notary service, staples the
ticket, validates the result, and only then creates the Homebrew tarball and
checksum.

Configure these GitHub Actions secrets to enable notarized macOS releases:

- `APPLE_CERTIFICATE`: base64-encoded `.p12` Developer ID Application certificate
- `APPLE_CERTIFICATE_PASSWORD`: password for the exported `.p12`
- `APPLE_ID`: Apple ID email used for notarization
- `APPLE_PASSWORD`: app-specific password for that Apple ID
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
templates in `packaging/winget`. After the first public release, unpack that
tarball into a fork of `microsoft/winget-pkgs` under the package path selected
by `wingetcreate`, validate it, and open the upstream pull request.

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
