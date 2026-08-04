use tauri::{
    AppHandle, Emitter, LogicalSize, Manager, PhysicalPosition, PhysicalSize, Size, WebviewUrl, WebviewWindow, WebviewWindowBuilder,
    menu::{Menu, MenuItem},
    tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent},
    WindowEvent,
};

mod activity;
mod voice_sidecar;
mod webview_permissions;

const PET_W: f64 = 280.0;
const PET_H: f64 = 280.0;
const LOGIN_W: f64 = 360.0;
const LOGIN_H: f64 = 420.0;
const CHAT_W: f64 = 320.0;
const CHAT_H: f64 = 440.0;
const CHAT_GAP: f64 = 8.0;

use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Mutex;
use std::time::{Duration, Instant};

static LAST_CHAT_SYNC: Mutex<Option<Instant>> = Mutex::new(None);
const CHAT_SYNC_MIN_MS: u64 = 100;

static LAST_ON_LEFT: AtomicBool = AtomicBool::new(false);
/// 防止 recover / focus 事件重入导致 WebView2 崩溃
static PET_RECOVER_IN_FLIGHT: AtomicBool = AtomicBool::new(false);

fn resolve_pet_window(app: &AppHandle, label: Option<&str>) -> Option<WebviewWindow> {
    if let Some(name) = label {
        if let Some(win) = app.get_webview_window(name) {
            return Some(win);
        }
    }
    app.get_webview_window("pet")
        .or_else(|| app.get_webview_window("main"))
}

/// Windows 最小化时坐标常为 -32000，Tauri center/unminimize 可能无效。
#[cfg(windows)]
fn win32_show_restore(window: &WebviewWindow) {
    const SW_RESTORE: i32 = 9;
    extern "system" {
        fn ShowWindow(hWnd: isize, nCmdShow: i32) -> i32;
    }
    if let Ok(hwnd) = window.hwnd() {
        unsafe {
            ShowWindow(hwnd.0 as isize, SW_RESTORE);
        }
    }
}

#[cfg(not(windows))]
fn win32_show_restore(_window: &WebviewWindow) {}

fn is_window_stuck(window: &WebviewWindow) -> bool {
    match (window.outer_position(), window.outer_size()) {
        (Ok(p), Ok(s)) => p.x < -4000 || p.y < -4000 || s.width < 120 || s.height < 120,
        _ => true,
    }
}

/// 强制恢复桌宠窗口：解除最小化、重置尺寸、移到主屏中央。
fn recover_pet_window_impl(app: &AppHandle) {
    if PET_RECOVER_IN_FLIGHT.swap(true, Ordering::SeqCst) {
        return;
    }

    let result = (|| {
        let window = resolve_pet_window(app, None)?;
        let _ = window.set_resizable(true);
        let _ = window.set_size(Size::Logical(LogicalSize::new(LOGIN_W, LOGIN_H)));

        win32_show_restore(&window);
        let _ = window.unminimize();

        if let Ok(Some(monitor)) = window.primary_monitor() {
            let scale = window.scale_factor().unwrap_or(1.0);
            let w = (LOGIN_W * scale).round() as u32;
            let h = (LOGIN_H * scale).round() as u32;
            let _ = window.set_size(Size::Physical(PhysicalSize::new(w, h)));
            let mon = monitor.position();
            let mon_size = monitor.size();
            let x = mon.x + (mon_size.width as i32 - w as i32) / 2;
            let y = mon.y + (mon_size.height as i32 - h as i32) / 2;
            let _ = window.set_position(PhysicalPosition::new(x, y));
        } else {
            let _ = window.center();
        }

        let _ = window.show();
        let _ = window.set_always_on_top(true);
        let _ = window.set_focus();
        let _ = app.emit("pet-window-recover", ());
        eprintln!("[pet] recover done");
        Some(())
    })();

    PET_RECOVER_IN_FLIGHT.store(false, Ordering::SeqCst);

    if result.is_none() {
        eprintln!("[pet] recover: window not found");
    }
}

fn show_pet_window(app: &AppHandle) {
    recover_pet_window_impl(app);
}

fn hide_pet_to_tray(window: &WebviewWindow) {
    let _ = window.hide();
}

fn resolve_chat_window(app: &AppHandle) -> Result<WebviewWindow, String> {
    if let Some(win) = app.get_webview_window("chat") {
        return Ok(win);
    }
    WebviewWindowBuilder::new(app, "chat", WebviewUrl::default())
        .title("Mochi Chat")
        .inner_size(CHAT_W, CHAT_H)
        .transparent(true)
        .decorations(false)
        .always_on_top(true)
        .skip_taskbar(true)
        .resizable(false)
        .shadow(false)
        .visible(false)
        .build()
        .map_err(|e| e.to_string())
}

fn place_chat_beside_pet_pos(
    app: &AppHandle,
    pet_label: Option<&str>,
    override_pos: Option<PhysicalPosition<i32>>,
) -> Result<(), String> {
    let chat = resolve_chat_window(app)?;
    let pet = resolve_pet_window(app, pet_label).ok_or_else(|| "pet window not found".to_string())?;

    let pet_pos = match override_pos {
        Some(p) => p,
        None => pet.outer_position().map_err(|e| e.to_string())?,
    };
    let scale = pet.scale_factor().unwrap_or(1.0);

    let gap_px = (CHAT_GAP * scale).round() as i32;
    let pet_size = pet.outer_size().unwrap_or(PhysicalSize::new(
        (PET_W * scale).round() as u32,
        (PET_H * scale).round() as u32,
    ));
    let pet_w_px = pet_size.width as i32;
    let pet_h_px = pet_size.height as i32;

    let chat_w_px = (CHAT_W * scale).round() as i32;
    let chat_h_px = (CHAT_H * scale).round() as i32;
    let panel_offset_px = ((CHAT_W + CHAT_GAP) * scale).round() as i32;

    let logical_w = pet
        .outer_size()
        .map(|s| s.width as f64 / scale)
        .unwrap_or(PET_W);
    let expanded = (logical_w - (PET_W + CHAT_GAP + CHAT_W)).abs() < 4.0;

    let currently_left = LAST_ON_LEFT.load(Ordering::Relaxed);
    let mut on_left = false;

    if let Ok(Some(monitor)) = pet.current_monitor() {
        let mon_pos = monitor.position();
        let mon_size = monitor.size();
        let right = mon_pos.x + mon_size.width as i32;
        let expanded_px = ((PET_W + CHAT_GAP + CHAT_W) * scale).round() as i32;
        let pet_screen_x = pet_pos.x;

        let hysteresis = if currently_left { (30.0 * scale).round() as i32 } else { 0 };
        let fits_right = pet_screen_x + expanded_px <= right - 8 - hysteresis;
        let fits_left = pet_screen_x - panel_offset_px >= mon_pos.x + 4;
        if !fits_right && fits_left {
            on_left = true;
        }
    }
    let prev_on_left = LAST_ON_LEFT.swap(on_left, Ordering::Relaxed);
    if prev_on_left != on_left {
        let _ = app.emit("side-panel-side-changed", on_left);
    }

    let pet_screen_x = if expanded && on_left {
        pet_pos.x + panel_offset_px
    } else {
        pet_pos.x
    };

    let mut x = if on_left {
        pet_screen_x - chat_w_px - gap_px
    } else {
        pet_screen_x + pet_w_px + gap_px
    };
    let mut y = pet_pos.y + pet_h_px - chat_h_px;

    if let Ok(Some(monitor)) = pet.current_monitor() {
        let mon_pos = monitor.position();
        let mon_size = monitor.size();
        let right = mon_pos.x + mon_size.width as i32;
        let bottom = mon_pos.y + mon_size.height as i32;
        x = x.clamp(
            mon_pos.x + 4,
            (right - chat_w_px - 4).max(mon_pos.x + 4),
        );
        y = y.clamp(
            mon_pos.y + 4,
            (bottom - chat_h_px - 4).max(mon_pos.y + 4),
        );
    }

    chat.set_position(PhysicalPosition::new(x, y))
        .map_err(|e| e.to_string())?;

    Ok(())
}

fn place_chat_beside_pet(app: &AppHandle, pet_label: Option<&str>) -> Result<(), String> {
    place_chat_beside_pet_pos(app, pet_label, None)
}

#[tauri::command]
fn recover_pet_window(app: AppHandle) -> Result<(), String> {
    recover_pet_window_impl(&app);
    Ok(())
}

#[tauri::command]
fn show_chat_window(app: AppHandle, label: Option<String>) -> Result<(), String> {
    let chat = resolve_chat_window(&app)?;
    place_chat_beside_pet(&app, label.as_deref())?;
    let _ = app.emit("side-panel-side-changed", LAST_ON_LEFT.load(Ordering::Relaxed));
    // 聊天窗每次显示时重新预授权麦克风（独立 WebView2 实例）
    webview_permissions::allow_media(&chat);
    chat.show().map_err(|e| e.to_string())?;
    let _ = chat.set_focus();
    Ok(())
}

#[tauri::command]
fn hide_chat_window(app: AppHandle) -> Result<(), String> {
    if let Some(chat) = app.get_webview_window("chat") {
        chat.hide().map_err(|e| e.to_string())?;
    }
    Ok(())
}

#[tauri::command]
fn expand_pet_for_chat(app: AppHandle, label: Option<String>) -> Result<(), String> {
    let pet = resolve_pet_window(&app, label.as_deref()).ok_or_else(|| "pet window not found".to_string())?;
    let w = PET_W + CHAT_GAP + CHAT_W;
    let h = PET_H.max(CHAT_H);
    pet.set_resizable(true).map_err(|e| e.to_string())?;
    pet.set_size(Size::Logical(LogicalSize::new(w, h)))
        .map_err(|e| e.to_string())?;
    Ok(())
}

#[tauri::command]
fn collapse_pet_chat(app: AppHandle, label: Option<String>) -> Result<(), String> {
    let pet = resolve_pet_window(&app, label.as_deref()).ok_or_else(|| "pet window not found".to_string())?;
    pet.set_size(Size::Logical(LogicalSize::new(PET_W, PET_H)))
        .map_err(|e| e.to_string())?;
    Ok(())
}

/// 拖拽结束后一次性对齐聊天窗（Moved 事件节流期间可能略滞后）
#[tauri::command]
fn sync_chat_beside_pet(app: AppHandle) -> Result<(), String> {
    place_chat_beside_pet(&app, None)
}

/// 重置 WebView2 内 localhost 麦克风权限（曾点「阻止」后从前端调用）
#[tauri::command]
fn reset_microphone_permission(app: AppHandle) -> Result<(), String> {
    let mut wins = Vec::new();
    if let Some(w) = resolve_pet_window(&app, None) {
        wins.push(w);
    }
    if let Some(w) = app.get_webview_window("chat") {
        wins.push(w);
    }
    if wins.is_empty() {
        return Err("webview not found".into());
    }
    webview_permissions::reset_microphone_for_app(&wins);
    Ok(())
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_autostart::init(
            tauri_plugin_autostart::MacosLauncher::LaunchAgent,
            Some(vec!["--minimized"]),
        ))
        .invoke_handler(tauri::generate_handler![
            show_chat_window,
            hide_chat_window,
            expand_pet_for_chat,
            collapse_pet_chat,
            activity::get_activity_snapshot,
            reset_microphone_permission,
            sync_chat_beside_pet,
            recover_pet_window,
            voice_sidecar::get_voice_sidecar_status,
            voice_sidecar::restart_voice_sidecars,
        ])
        .setup(|app| {
            // 本地语音 sidecar：Release 用内置 bundle，Debug 用仓库 tools/
            let bundle_mode = if voice_sidecar::resolve_bundled_voice_root(app.handle()).is_some() {
                "release"
            } else {
                "dev"
            };
            let sidecar_mgr = Arc::new(voice_sidecar::VoiceSidecarManager::new(bundle_mode));
            sidecar_mgr.start_all_async(app.handle().clone());
            app.manage(sidecar_mgr);

            let pet = app
                .get_webview_window("pet")
                .or_else(|| app.get_webview_window("main"));
            if let Some(pet) = pet {
                let _ = pet.set_shadow(false);
                recover_pet_window_impl(app.handle());

                // Close button / Alt+F4 → hide to tray; sync popup on move
                let pet_for_close = pet.clone();
                let app_for_move = app.handle().clone();
                let app_for_recover = app.handle().clone();
                let pet_for_recover = pet.clone();
                pet.on_window_event(move |event| {
                    match event {
                        tauri::WindowEvent::CloseRequested { api, .. } => {
                            api.prevent_close();
                            hide_pet_to_tray(&pet_for_close);
                        }
                        WindowEvent::Focused(true) => {
                            // 仅在窗口坐标异常时 recover，避免 focus 循环触发 Win32/WebView2 崩溃
                            if is_window_stuck(&pet_for_recover) {
                                recover_pet_window_impl(&app_for_recover);
                            }
                        }
                        tauri::WindowEvent::Moved(pos) => {
                            if let Some(chat) = app_for_move.get_webview_window("chat") {
                                if chat.is_visible().unwrap_or(false) {
                                    let now = Instant::now();
                                    let mut last = LAST_CHAT_SYNC.lock().unwrap();
                                    let due = last
                                        .map(|t| now.duration_since(t) >= Duration::from_millis(CHAT_SYNC_MIN_MS))
                                        .unwrap_or(true);
                                    if due {
                                        let _ = place_chat_beside_pet_pos(&app_for_move, None, Some(*pos));
                                        *last = Some(now);
                                    }
                                }
                            }
                            let _ = app_for_move.emit("pet-window-moved", ());
                        }
                        _ => {}
                    }
                });
            }

            if let Some(chat) = app.get_webview_window("chat") {
                let _ = chat.set_shadow(false);
                let chat_for_close = chat.clone();
                chat.on_window_event(move |event| {
                    if let tauri::WindowEvent::CloseRequested { api, .. } = event {
                        api.prevent_close();
                        let _ = chat_for_close.hide();
                    }
                });
            }

            // Windows：WebView2 预授权麦克风（避免桌宠 getUserMedia 默认被拒）
            {
                let mut mic_wins = Vec::new();
                if let Some(w) = app.get_webview_window("pet").or_else(|| app.get_webview_window("main")) {
                    mic_wins.push(w);
                }
                if let Some(w) = app.get_webview_window("chat") {
                    mic_wins.push(w);
                }
                webview_permissions::allow_media_for_app(&mic_wins);
            }

            let show = MenuItem::with_id(app, "show", "显示 Mochi", true, None::<&str>)?;
            let recenter = MenuItem::with_id(app, "recenter", "找回 Mochi（居中）", true, None::<&str>)?;
            let quit = MenuItem::with_id(app, "quit", "退出", true, None::<&str>)?;
            let menu = Menu::with_items(app, &[&show, &recenter, &quit])?;

            let _tray = TrayIconBuilder::new()
                .icon(app.default_window_icon().unwrap().clone())
                .tooltip("Mochi")
                .menu(&menu)
                .show_menu_on_left_click(false)
                .on_menu_event(|app, event| match event.id.as_ref() {
                    "show" => show_pet_window(app),
                    "recenter" => show_pet_window(app),
                    "quit" => {
                        if let Some(mgr) = app.try_state::<Arc<voice_sidecar::VoiceSidecarManager>>() {
                            mgr.stop_all();
                        }
                        app.exit(0);
                    }
                    _ => {}
                })
                .on_tray_icon_event(|tray, event| {
                    if let TrayIconEvent::Click {
                        button: MouseButton::Left,
                        button_state: MouseButtonState::Up,
                        ..
                    } = event
                    {
                        show_pet_window(tray.app_handle());
                    }
                })
                .build(app)?;

            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("error while building tauri application")
        .run(|app_handle, event| {
            if let tauri::RunEvent::Exit = event {
                if let Some(mgr) = app_handle.try_state::<Arc<voice_sidecar::VoiceSidecarManager>>() {
                    mgr.stop_all();
                }
            }
        });
}
