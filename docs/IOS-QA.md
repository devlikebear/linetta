# iPhone and iPad release verification

## Verified locally

- On 2026-09-28, an unsigned Universal simulator app (1.2.2, iOS minimum 15.0)
  built successfully using `make build-mobile-ios-sim`.
- Dedicated iPhone 17 Pro and iPad Pro 11-inch (M5) simulators on iOS 26.5
  installed and launched the app. All five embedded-engine ABI symbols were
  present. Both created a database with 27 tables and passed SQLite quick_check.
- Native screenshots showed the first-work creation dialog on both devices.
  This is startup verification, not proof of the complete editing flow.
- Browser fixtures using the production CSS were checked at 390x844, 932x430,
  834x1194, 1194x834, 1024x1366, and 1366x1024: no horizontal overflow,
  visible outline controls, scrollable toolbar, and fixed inspector positioning.
- On 2026-09-29, `make ci` passed, including Go race tests, packaging checks,
  frontend tests/build, and Rust checks/tests. After CI workflow coverage was
  added, all 579 frontend tests passed.

## Required before an iOS release

- [ ] Create a work, enter a manuscript, wait for autosave, terminate and reopen
  the app, and verify the exact text on both iPhone and iPad.
- [ ] Repeat with Korean composition, rapid scene switching, and backgrounding
  while a save is pending.
- [ ] Rotate with the outline and each inspector open; verify no obscured editor
  or unreachable close controls. Test narrow iPad multitasking windows.
- [ ] Verify the on-screen keyboard and external keyboard on physical devices,
  including safe areas, scrolling to the caret, shortcuts, and keyboard dismissal.
- [ ] Test Apple Pencil/Scribble separately; no compatibility claim is made yet.
- [ ] Produce a signed iOS archive, validate its provisioning and entitlements,
  upload to TestFlight, and verify an installation from TestFlight.

Device Hub UI automation timed out during the follow-up on 2026-09-29, so the
unchecked interactions above remain unverified. Local Apple Distribution
identity and repository Apple signing secret names were present; this does not
establish valid iOS provisioning or a successful TestFlight upload.

See [DEVELOPMENT.md](DEVELOPMENT.md) for toolchain setup and simulator commands.
