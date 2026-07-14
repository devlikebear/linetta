# Linetta 분석 개요

- 기준: `1020096a14b65c7bd21ca081947f1747978e3b93`
- 구조: React/Tauri Rust/Go FFI/SQLite 단일 로컬 앱
- Blocking: pending debounce가 최신 scene ID와 결합해 다른 씬을 덮어쓸 수 있음
- 배포 gate: 제3자 AI 전송의 앱 내 명시 고지/동의와 privacy policy 정합성
- 전체 보고서: `full-audit.md`
