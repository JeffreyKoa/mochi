use tauri::{
    AppHandle, Emitter, LogicalSize, Manager, PhysicalPosition, PhysicalSize, Size, WebviewUrl, WebviewWindow, WebviewWindowBuilder,
    menu::{Menu, MenuItem},
    tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent},
};

mod activity;

const PET_W: f64 = 280.0;
const PET_H: f64 = 280.0;
const CHAT_W: f64 = 320.0;
const CHAT_H: f64 = 440.0;
const CHAT_GAP: f64 = 8.0;

use std::sync::atomic::{AtomicBool, Ordering};

static LAST_ON_LEFT: AtomicBool = AtomicBool::new(false);

fn resolve_pet_window(app: &AppHandle, label: Option<&str>) -> Option<WebviewWindow> {
    if let Some(name) = label {
        if let Some(win) = app.get_webview_window(name) {
            return Some(win);
        }
    }
    app.get_webview_window("pet")
        .or_else(|| app.get_webview_window("main"))
}

fn show_pet_window(app: &AppHandle) {
    let Some(window) = resolve_pet_window(app, None) else {
        eprintln!("[tray] pet window not found");
        return;
    };

    if let Err(e) = window.unminimize() {
        eprintln!("[tray] unminimize: {e}");
    }
    if let Err(e) = window.show() {
        eprintln!("[tray] show: {e}");
    }
    if let Err(e) = window.set_always_on_top(true) {
        eprintln!("[tray] always_on_top: {e}");
    }
    if let Err(e) = window.center() {
        eprintln!("[tray] center: {e}");
    }
    if let Err(e) = window.set_focus() {
        eprintln!("[tray] focus: {e}");
    }
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
fn show_chat_window(app: AppHandle, label: Option<String>) -> Result<(), String> {
    let chat = resolve_chat_window(&app)?;
    place_chat_beside_pet(&app, label.as_deref())?;
    let _ = app.emit("side-panel-side-changed", LAST_ON_LEFT.load(Ordering::Relaxed));
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
        ])
        .setup(|app| {
            let pet = app
                .get_webview_window("pet")
                .or_else(|| app.get_webview_window("main"));
            if let Some(pet) = pet {
                let _ = pet.set_shadow(false);
                let _ = pet.center();
                let _ = pet.show();
                let _ = pet.set_always_on_top(true);

                // Close button / Alt+F4 → hide to tray; sync popup on move
                let pet_for_close = pet.clone();
                let app_for_move = app.handle().clone();
                pet.on_window_event(move |event| {
                    match event {
                        tauri::WindowEvent::CloseRequested { api, .. } => {
                            api.prevent_close();
                            hide_pet_to_tray(&pet_for_close);
                        }
                        tauri::WindowEvent::Moved(pos) => {
                            if let Some(chat) = app_for_move.get_webview_window("chat") {
                                if chat.is_visible().unwrap_or(false) {
                                    let _ = place_chat_beside_pet_pos(&app_for_move, None, Some(*pos));
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
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
