//! Self-update from GitHub releases.
//!
//! "Check for updates…" lives in the tray menu and, on macOS, in the app
//! menu. The release workflow signs each installer and publishes a
//! `latest.json` naming them; the updater plugin verifies the signature
//! against the public key in `tauri.conf.json` before installing anything.
//!
//! Everything here is shell-side: the webview's CSP does not let it reach
//! GitHub, and an update must be offerable even when the engine failed to
//! start. Mac App Store builds leave this module out — the store updates
//! those.

use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Mutex;
use std::time::Duration;

use tauri::menu::MenuItem;
use tauri::Manager;
use tauri_plugin_dialog::{DialogExt, MessageDialogButtons, MessageDialogKind};
use tauri_plugin_updater::UpdaterExt;

use crate::tray;

const RELEASES_URL: &str = "https://github.com/devlikebear/linetta/releases/latest";

/// Long enough for the engine and the first project to load, so the check
/// does not compete with startup.
const FIRST_CHECK_AFTER: Duration = Duration::from_secs(30);

#[cfg(target_os = "macos")]
const APP_MENU_ID: &str = "app-check-updates";

/// One check at a time: the menu entry stays clickable while a download runs.
static BUSY: AtomicBool = AtomicBool::new(false);

#[derive(Default)]
pub(crate) struct UpdaterState {
    /// The macOS app-menu entry, kept so its label can follow the language.
    app_item: Mutex<Option<MenuItem<tauri::Wry>>>,
}

struct Text {
    title: &'static str,
    install: &'static str,
    restart: &'static str,
    later: &'static str,
    close: &'static str,
    open_releases: &'static str,
    downloading: &'static str,
    lang: Lang,
}

#[derive(Clone, Copy, PartialEq, Debug)]
enum Lang {
    Ko,
    En,
    Ja,
}

fn lang_of(language: &str) -> Lang {
    if language.starts_with("en") {
        Lang::En
    } else if language.starts_with("ja") {
        Lang::Ja
    } else {
        Lang::Ko
    }
}

pub(crate) fn menu_label(language: &str) -> &'static str {
    match lang_of(language) {
        Lang::En => "Check for Updates…",
        Lang::Ja => "アップデートを確認…",
        Lang::Ko => "업데이트 확인…",
    }
}

fn text(language: &str) -> Text {
    match lang_of(language) {
        Lang::En => Text {
            title: "Linetta Update",
            install: "Update",
            restart: "Install and Restart",
            later: "Later",
            close: "Close",
            open_releases: "Open Release Page",
            downloading: "Linetta — downloading update…",
            lang: Lang::En,
        },
        Lang::Ja => Text {
            title: "Linetta アップデート",
            install: "アップデート",
            restart: "インストールして再起動",
            later: "あとで",
            close: "閉じる",
            open_releases: "リリースページを開く",
            downloading: "Linetta — アップデートをダウンロード中…",
            lang: Lang::Ja,
        },
        Lang::Ko => Text {
            title: "Linetta 업데이트",
            install: "업데이트",
            restart: "설치하고 재시작",
            later: "나중에",
            close: "닫기",
            open_releases: "릴리스 페이지 열기",
            downloading: "Linetta — 업데이트 내려받는 중…",
            lang: Lang::Ko,
        },
    }
}

impl Text {
    fn up_to_date(&self, current: &str) -> String {
        match self.lang {
            Lang::En => format!("Linetta {current} is the latest version."),
            Lang::Ja => format!("Linetta {current} は最新バージョンです。"),
            Lang::Ko => format!("Linetta {current}은(는) 최신 버전입니다."),
        }
    }

    fn available(&self, latest: &str, current: &str) -> String {
        match self.lang {
            Lang::En => format!(
                "Linetta {latest} is available (you have {current}).\nDownload it now?"
            ),
            Lang::Ja => format!(
                "Linetta {latest} が利用できます（現在 {current}）。\n今すぐダウンロードしますか？"
            ),
            Lang::Ko => format!(
                "Linetta {latest}을(를) 사용할 수 있습니다(현재 {current}).\n지금 내려받을까요?"
            ),
        }
    }

    fn ready(&self, latest: &str) -> String {
        match self.lang {
            Lang::En => format!(
                "Linetta {latest} is downloaded.\nInstall it and restart now?"
            ),
            Lang::Ja => format!(
                "Linetta {latest} をダウンロードしました。\n今すぐインストールして再起動しますか？"
            ),
            Lang::Ko => format!(
                "Linetta {latest}을(를) 내려받았습니다.\n지금 설치하고 재시작할까요?"
            ),
        }
    }

    fn failed(&self, error: &str) -> String {
        match self.lang {
            Lang::En => format!("Could not update Linetta.\n\n{error}"),
            Lang::Ja => format!("Linetta をアップデートできませんでした。\n\n{error}"),
            Lang::Ko => format!("Linetta를 업데이트하지 못했습니다.\n\n{error}"),
        }
    }

    /// For builds that cannot replace themselves: a development build, or a
    /// package the system's own package manager owns.
    fn manual(&self, current: &str) -> String {
        match self.lang {
            Lang::En => format!(
                "This build of Linetta ({current}) does not update itself.\nGet the latest version from the release page or your package manager."
            ),
            Lang::Ja => format!(
                "このビルドの Linetta（{current}）は自動アップデートに対応していません。\nリリースページまたはパッケージマネージャーから最新版を入手してください。"
            ),
            Lang::Ko => format!(
                "이 빌드의 Linetta({current})는 스스로 업데이트하지 않습니다.\n릴리스 페이지나 패키지 관리자에서 최신 버전을 받으세요."
            ),
        }
    }
}

/// A build can replace itself only when the bundler stamped it with the
/// installer it shipped in; `latest.json` is keyed by that installer.
fn self_updating() -> bool {
    !cfg!(debug_assertions) && tauri::utils::platform::bundle_type().is_some()
}

fn language(app: &tauri::AppHandle) -> String {
    app.try_state::<tray::TrayState>()
        .map(|s| s.prefs.lock().unwrap().language.clone())
        .unwrap_or_default()
}

/// Add "Check for Updates…" under About in the macOS app menu.
#[cfg(target_os = "macos")]
pub(crate) fn setup_app_menu(app: &tauri::AppHandle) -> tauri::Result<()> {
    let menu = tauri::menu::Menu::default(app)?;
    let item = MenuItem::with_id(
        app,
        APP_MENU_ID,
        menu_label(&language(app)),
        true,
        None::<&str>,
    )?;
    if let Some(app_menu) = menu.items()?.first().and_then(|i| i.as_submenu()) {
        app_menu.insert(&item, 1)?;
    }
    app.set_menu(menu)?;
    app.on_menu_event(|app, event| {
        if event.id().as_ref() == APP_MENU_ID {
            check(app, true);
        }
    });
    *app.state::<UpdaterState>().app_item.lock().unwrap() = Some(item);
    Ok(())
}

/// Follow a language change; the tray rebuilds its own menu.
pub(crate) fn relabel(app: &tauri::AppHandle, language: &str) {
    if let Some(state) = app.try_state::<UpdaterState>() {
        if let Some(item) = state.app_item.lock().unwrap().as_ref() {
            let _ = item.set_text(menu_label(language));
        }
    }
}

/// Check once shortly after launch. A newer release is offered, never
/// installed unasked.
pub(crate) fn schedule_startup_check(app: &tauri::AppHandle) {
    if !self_updating() {
        return;
    }
    let app = app.clone();
    tauri::async_runtime::spawn(async move {
        tokio::time::sleep(FIRST_CHECK_AFTER).await;
        check(&app, false);
    });
}

/// `explicit` is the menu entry: it reports "up to date" and errors in a
/// dialog. The startup check stays silent unless there is a release to offer.
pub(crate) fn check(app: &tauri::AppHandle, explicit: bool) {
    if BUSY.swap(true, Ordering::SeqCst) {
        return;
    }
    if explicit {
        tray::show_main_window(app);
    }
    let app = app.clone();
    tauri::async_runtime::spawn(async move {
        run(&app, explicit).await;
        BUSY.store(false, Ordering::SeqCst);
    });
}

async fn run(app: &tauri::AppHandle, explicit: bool) {
    let t = text(&language(app));
    let current = app.package_info().version.to_string();

    if !self_updating() {
        if explicit && ask(app, &t, t.manual(&current), t.open_releases, t.close).await {
            open_releases(app);
        }
        return;
    }

    let checked = match app.updater() {
        Ok(updater) => updater.check().await,
        Err(e) => Err(e),
    };
    let update = match checked {
        Ok(Some(update)) => update,
        Ok(None) => {
            if explicit {
                inform(app, &t, t.up_to_date(&current)).await;
            }
            return;
        }
        Err(e) => {
            eprintln!("[linetta] update check failed: {e}");
            if explicit {
                offer_release_page(app, &t, &e.to_string()).await;
            }
            return;
        }
    };

    if !explicit && !should_announce(app, &update.version) {
        return;
    }
    if !ask(app, &t, t.available(&update.version, &current), t.install, t.later).await {
        return;
    }

    set_tooltip(app, t.downloading);
    let downloaded = update.download(|_, _| {}, || {}).await;
    set_tooltip(app, "Linetta");
    let bytes = match downloaded {
        Ok(bytes) => bytes,
        Err(e) => {
            eprintln!("[linetta] update download failed: {e}");
            offer_release_page(app, &t, &e.to_string()).await;
            return;
        }
    };

    // Asked again, after the download: installing ends this process, and the
    // writer may have kept typing while it ran.
    if !ask(app, &t, t.ready(&update.version), t.restart, t.later).await {
        return;
    }
    let installed =
        tauri::async_runtime::spawn_blocking(move || update.install(bytes)).await;
    match installed {
        Ok(Ok(())) => app.restart(),
        Ok(Err(e)) => {
            eprintln!("[linetta] update install failed: {e}");
            offer_release_page(app, &t, &e.to_string()).await;
        }
        Err(e) => eprintln!("[linetta] update install task failed: {e}"),
    }
}

/// The startup check offers each release once, and only over a visible
/// window: a tray resident started at login should not open a dialog.
fn should_announce(app: &tauri::AppHandle, version: &str) -> bool {
    let visible = app
        .get_webview_window("main")
        .and_then(|w| w.is_visible().ok())
        .unwrap_or(false);
    if !visible {
        return false;
    }
    let Some(state) = app.try_state::<tray::TrayState>() else {
        return true;
    };
    let snapshot = {
        let mut prefs = state.prefs.lock().unwrap();
        if prefs.update_announced == version {
            return false;
        }
        prefs.update_announced = version.to_string();
        prefs.clone()
    };
    tray::save_prefs(app, &snapshot);
    true
}

fn set_tooltip(app: &tauri::AppHandle, tooltip: &str) {
    if let Some(state) = app.try_state::<tray::TrayState>() {
        if let Some(tray) = state.tray.lock().unwrap().as_ref() {
            let _ = tray.set_tooltip(Some(tooltip));
        }
    }
}

fn open_releases(app: &tauri::AppHandle) {
    use tauri_plugin_opener::OpenerExt;
    if let Err(e) = app.opener().open_url(RELEASES_URL, None::<&str>) {
        eprintln!("[linetta] could not open the release page: {e}");
    }
}

async fn offer_release_page(app: &tauri::AppHandle, t: &Text, error: &str) {
    if ask(app, t, t.failed(error), t.open_releases, t.close).await {
        open_releases(app);
    }
}

async fn ask(app: &tauri::AppHandle, t: &Text, message: String, ok: &str, cancel: &str) -> bool {
    let (tx, rx) = tokio::sync::oneshot::channel();
    dialog(app, t, message)
        .buttons(MessageDialogButtons::OkCancelCustom(
            ok.to_string(),
            cancel.to_string(),
        ))
        .show(move |yes| {
            let _ = tx.send(yes);
        });
    rx.await.unwrap_or(false)
}

async fn inform(app: &tauri::AppHandle, t: &Text, message: String) {
    let (tx, rx) = tokio::sync::oneshot::channel();
    dialog(app, t, message)
        .buttons(MessageDialogButtons::Ok)
        .show(move |_| {
            let _ = tx.send(());
        });
    let _ = rx.await;
}

fn dialog(
    app: &tauri::AppHandle,
    t: &Text,
    message: String,
) -> tauri_plugin_dialog::MessageDialogBuilder<tauri::Wry> {
    let builder = app
        .dialog()
        .message(message)
        .title(t.title)
        .kind(MessageDialogKind::Info);
    match app.get_webview_window("main") {
        Some(window) if window.is_visible().unwrap_or(false) => builder.parent(&window),
        _ => builder,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn language_falls_back_to_korean() {
        assert_eq!(lang_of("en-US"), Lang::En);
        assert_eq!(lang_of("ja"), Lang::Ja);
        assert_eq!(lang_of("ko"), Lang::Ko);
        assert_eq!(lang_of(""), Lang::Ko);
    }

    #[test]
    fn messages_name_both_versions() {
        for language in ["ko", "en", "ja"] {
            let t = text(language);
            let message = t.available("1.5.0", "1.4.0");
            assert!(message.contains("1.5.0") && message.contains("1.4.0"));
            assert!(t.ready("1.5.0").contains("1.5.0"));
            assert!(t.up_to_date("1.4.0").contains("1.4.0"));
        }
    }

    #[test]
    fn a_debug_build_does_not_update_itself() {
        assert!(!self_updating());
    }
}
