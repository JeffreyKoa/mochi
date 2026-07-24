use tauri::{
    AppHandle, LogicalSize, Manager, PhysicalPosition, Size, WebviewWindow,
    menu::{Menu, MenuItem},
    tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent},
};

const PET_W: f64 = 200.0;
const PET_H: f64 = 220.0;
const CHAT_W: f64 = 320.0;
const CHAT_H: f64 = 440.0;
const CHAT_GAP: f64 = 8.0;

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
    app.get_webview_window("chat")
        .ok_or_else(|| "chat window not found".to_string())
}

fn place_chat_beside_pet(app: &AppHandle, pet_label: Option<&str>) -> Result<(), String> {
    let chat = resolve_chat_window(app)?;
    let pet = resolve_pet_window(app, pet_label).ok_or_else(|| "pet window not found".to_string())?;

    let pet_pos = pet.outer_position().map_err(|e| e.to_string())?;
    let pet_size = pet.outer_size().map_err(|e| e.to_string())?;
    let chat_size = chat.outer_size().map_err(|e| e.to_string())?;

    let mut x = pet_pos.x + pet_size.width as i32 + CHAT_GAP as i32;
    let mut y = pet_pos.y;

    if let Ok(Some(monitor)) = pet.current_monitor() {
        let mon_pos = monitor.position();
        let mon_size = monitor.size();
        let right = mon_pos.x + mon_size.width as i32;
        let bottom = mon_pos.y + mon_size.height as i32;
        if x + chat_size.width as i32 > right - 8 {
            x = pet_pos.x - chat_size.width as i32 - CHAT_GAP as i32;
        }
        x = x.clamp(mon_pos.x + 4, right - chat_size.width as i32 - 4);
        y = y.clamp(mon_pos.y + 4, bottom - chat_size.height as i32 - 4);
    }

    chat.set_position(PhysicalPosition::new(x, y))
        .map_err(|e| e.to_string())?;
    Ok(())
}

#[tauri::command]
fn show_chat_window(app: AppHandle, label: Option<String>) -> Result<(), String> {
    let chat = resolve_chat_window(&app)?;
    place_chat_beside_pet(&app, label.as_deref())?;
    chat.show().map_err(|e| e.to_string())?;
    chat.set_focus().map_err(|e| e.to_string())?;
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
            collapse_pet_chat
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

                // Close button / Alt+F4 → hide to tray instead of destroying the window
                let pet_for_close = pet.clone();
                pet.on_window_event(move |event| {
                    if let tauri::WindowEvent::CloseRequested { api, .. } = event {
                        api.prevent_close();
                        hide_pet_to_tray(&pet_for_close);
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
