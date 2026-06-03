# Distribution packaging

Linetta uses GitHub Releases as the canonical source for desktop installers.
The release workflow publishes:

- macOS app archive for the Homebrew cask
- Windows NSIS installer and MSI bundle
- Linux AppImage, deb, and rpm bundles
- `SHA256SUMS`
- a tarball of rendered winget manifests
- a Flathub manifest starter

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
