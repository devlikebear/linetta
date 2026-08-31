//! Tray residence + login autostart (#81).
//!
//! The MCP server lives inside the engine, and the engine dies with the app.
//! So "MCP is always reachable" translates to: the app keeps running in the
//! system tray when its window closes, and can start hidden at login. Both
//! behaviours are opt-in toggles in Settings.
//!
//! The close-to-tray preference lives in a shell-side JSON file, not in the
//! engine settings row: the close handler needs it synchronously, and it must
//! be readable even when the engine failed to start.

use std::fs;
use std::path::PathBuf;
use std::sync::Mutex;

use tauri::menu::{Menu, MenuItem};
use tauri::tray::{MouseButton, MouseButtonState, TrayIcon, TrayIconBuilder, TrayIconEvent};
use tauri::Manager;

const PREFS_FILE: &str = "shell-prefs.json";

#[derive(Clone, serde::Serialize, serde::Deserialize)]
pub(crate) struct ShellPrefs {
    #[serde(default)]
    pub close_to_tray: bool,
    /// UI language pushed by the webview so tray menu labels match the app.
    #[serde(default)]
    pub language: String,
}

impl Default for ShellPrefs {
    fn default() -> Self {
        Self {
            close_to_tray: false,
            language: String::new(),
        }
    }
}

pub(crate) struct TrayState {
    pub prefs: Mutex<ShellPrefs>,
    pub tray: Mutex<Option<TrayIcon>>,
}

fn prefs_path(app: &tauri::AppHandle) -> Option<PathBuf> {
    app.path().app_config_dir().ok().map(|d| d.join(PREFS_FILE))
}

pub(crate) fn load_prefs(app: &tauri::AppHandle) -> ShellPrefs {
    let Some(path) = prefs_path(app) else {
        return ShellPrefs::default();
    };
    fs::read_to_string(path)
        .ok()
        .and_then(|raw| serde_json::from_str(&raw).ok())
        .unwrap_or_default()
}

fn save_prefs(app: &tauri::AppHandle, prefs: &ShellPrefs) {
    let Some(path) = prefs_path(app) else { return };
    if let Some(dir) = path.parent() {
        let _ = fs::create_dir_all(dir);
    }
    if let Ok(raw) = serde_json::to_string_pretty(prefs) {
        let _ = fs::write(path, raw);
    }
}

/// Menu labels in the writer's language. The engine owns the full i18n
/// catalogue; the tray needs exactly two strings, so they live here rather
/// than dragging a catalogue across the FFI boundary.
fn labels(language: &str) -> (&'static str, &'static str) {
    if language.starts_with("en") {
        ("Open Linetta", "Quit Linetta")
    } else if language.starts_with("ja") {
        ("Linetta を開く", "Linetta を終了")
    } else {
        ("Linetta 열기", "Linetta 종료")
    }
}

fn build_menu(app: &tauri::AppHandle, language: &str) -> tauri::Result<Menu<tauri::Wry>> {
    let (open_label, quit_label) = labels(language);
    let open = MenuItem::with_id(app, "tray-open", open_label, true, None::<&str>)?;
    let quit = MenuItem::with_id(app, "tray-quit", quit_label, true, None::<&str>)?;
    Menu::with_items(app, &[&open, &quit])
}

pub(crate) fn show_main_window(app: &tauri::AppHandle) {
    #[cfg(target_os = "macos")]
    let _ = app.set_activation_policy(tauri::ActivationPolicy::Regular);
    if let Some(window) = app.get_webview_window("main") {
        let _ = window.show();
        let _ = window.unminimize();
        let _ = window.set_focus();
    }
}

pub(crate) fn hide_to_tray(app: &tauri::AppHandle) {
    if let Some(window) = app.get_webview_window("main") {
        let _ = window.hide();
    }
    // Without this the app keeps a Dock icon while "closed", which reads as a
    // stuck app rather than a tray resident.
    #[cfg(target_os = "macos")]
    let _ = app.set_activation_policy(tauri::ActivationPolicy::Accessory);
}

/// Build the tray icon once at startup. The icon itself is permanent; only the
/// menu is rebuilt when the language changes.
pub(crate) fn setup(app: &tauri::AppHandle) -> tauri::Result<()> {
    let prefs = load_prefs(app);
    let menu = build_menu(app, &prefs.language)?;
    let tray = TrayIconBuilder::with_id("linetta-tray")
        .icon(app.default_window_icon().cloned().ok_or_else(|| {
            tauri::Error::AssetNotFound("default window icon".into())
        })?)
        .tooltip("Linetta")
        .menu(&menu)
        .show_menu_on_left_click(false)
        .on_menu_event(|app, event| match event.id().as_ref() {
            "tray-open" => show_main_window(app),
            "tray-quit" => app.exit(0),
            _ => {}
        })
        .on_tray_icon_event(|tray, event| {
            // Left click restores the window (Windows/Linux convention);
            // the menu stays on right click.
            if let TrayIconEvent::Click {
                button: MouseButton::Left,
                button_state: MouseButtonState::Up,
                ..
            } = event
            {
                show_main_window(tray.app_handle());
            }
        })
        .build(app)?;

    let state = app.state::<TrayState>();
    *state.tray.lock().unwrap() = Some(tray);
    Ok(())
}

/// Close-to-tray interception, called from the builder's window event hook.
/// Returns true when the close was converted into a hide.
pub(crate) fn handle_close_requested(app: &tauri::AppHandle) -> bool {
    let close_to_tray = app
        .try_state::<TrayState>()
        .map(|s| s.prefs.lock().unwrap().close_to_tray)
        .unwrap_or(false);
    if close_to_tray {
        hide_to_tray(app);
    }
    close_to_tray
}

#[derive(serde::Serialize)]
pub(crate) struct BackgroundPrefs {
    close_to_tray: bool,
    autostart: bool,
}

#[tauri::command]
pub(crate) fn background_prefs_get(app: tauri::AppHandle) -> BackgroundPrefs {
    let close_to_tray = app
        .try_state::<TrayState>()
        .map(|s| s.prefs.lock().unwrap().close_to_tray)
        .unwrap_or(false);
    let autostart = {
        use tauri_plugin_autostart::ManagerExt;
        app.autolaunch().is_enabled().unwrap_or(false)
    };
    BackgroundPrefs {
        close_to_tray,
        autostart,
    }
}

#[tauri::command]
pub(crate) fn background_prefs_set(
    app: tauri::AppHandle,
    close_to_tray: Option<bool>,
    autostart: Option<bool>,
    language: Option<String>,
) -> Result<BackgroundPrefs, String> {
    if let Some(enabled) = autostart {
        use tauri_plugin_autostart::ManagerExt;
        let launcher = app.autolaunch();
        let result = if enabled {
            launcher.enable()
        } else {
            launcher.disable()
        };
        result.map_err(|e| e.to_string())?;
    }

    let state = app.state::<TrayState>();
    let snapshot = {
        let mut prefs = state.prefs.lock().unwrap();
        if let Some(v) = close_to_tray {
            prefs.close_to_tray = v;
        }
        if let Some(lang) = language {
            prefs.language = lang;
        }
        prefs.clone()
    };
    save_prefs(&app, &snapshot);

    // Rebuild the menu so labels follow the app language.
    if let Ok(menu) = build_menu(&app, &snapshot.language) {
        if let Some(tray) = state.tray.lock().unwrap().as_ref() {
            let _ = tray.set_menu(Some(menu));
        }
    }

    Ok(background_prefs_get(app.clone()))
}
