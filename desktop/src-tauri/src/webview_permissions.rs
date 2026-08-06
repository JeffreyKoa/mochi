//! Windows WebView2：预授权 / 重置麦克风与摄像头（Tauri 桌宠 dev + 打包场景）

use tauri::WebviewWindow;

#[cfg(windows)]
mod imp {
    use super::WebviewWindow;
    use std::collections::HashSet;
    use std::sync::{LazyLock, Mutex};
    use webview2_com::Microsoft::Web::WebView2::Win32::{
        COREWEBVIEW2_PERMISSION_KIND_CAMERA, COREWEBVIEW2_PERMISSION_KIND_MICROPHONE,
        COREWEBVIEW2_PERMISSION_STATE_ALLOW, COREWEBVIEW2_PERMISSION_STATE_DEFAULT,
        ICoreWebView2, ICoreWebView2Profile4, ICoreWebView2_13,
    };
    use webview2_com::PermissionRequestedEventHandler;
    use windows_core::{Interface, PCWSTR, PWSTR};

    /// dev / 打包常见 Origin（WebView2 按站点记权限）
    const MEDIA_ORIGINS: &[&str] = &[
        "http://localhost:1420",
        "http://localhost:1421",
        "http://127.0.0.1:1420",
        "http://127.0.0.1:1421",
        "https://tauri.localhost",
        "http://tauri.localhost",
        "https://asset.localhost",
        "http://asset.localhost",
        "https://ipc.localhost",
        "http://ipc.localhost",
        "https://localhost",
        "http://localhost",
    ];

    static PERMISSION_HANDLER_REGISTERED: LazyLock<Mutex<HashSet<String>>> =
        LazyLock::new(|| Mutex::new(HashSet::new()));

    /// 从 WebView Source URL 提取 origin（scheme + host [+ port]）
    fn origin_from_source(source: &str) -> String {
        let trimmed = source.trim();
        if let Some(scheme_end) = trimmed.find("://") {
            let scheme = &trimmed[..scheme_end];
            let rest = &trimmed[scheme_end + 3..];
            let host_port = rest.split('/').next().unwrap_or(rest);
            if !host_port.is_empty() {
                return format!("{scheme}://{host_port}");
            }
        }
        trimmed.trim_end_matches('/').to_string()
    }

    fn set_permission_for_origin(
        profile4: &ICoreWebView2Profile4,
        kind: webview2_com::Microsoft::Web::WebView2::Win32::COREWEBVIEW2_PERMISSION_KIND,
        state: webview2_com::Microsoft::Web::WebView2::Win32::COREWEBVIEW2_PERMISSION_STATE,
        origin: &str,
    ) {
        let wide: Vec<u16> = origin.encode_utf16().chain(std::iter::once(0)).collect();
        let origin_pw = PCWSTR::from_raw(wide.as_ptr());
        let _ = unsafe { profile4.SetPermissionState(kind, origin_pw, state, None) };
    }

    /// 注册 PermissionRequested：getUserMedia 弹出权限时自动允许麦克风/摄像头
    fn register_permission_handler(core: &ICoreWebView2, window_label: &str) {
        let mut registered = PERMISSION_HANDLER_REGISTERED.lock().unwrap();
        if registered.contains(window_label) {
            return;
        }

        let handler = PermissionRequestedEventHandler::create(Box::new(|_sender, args| {
            if let Some(args) = args {
                let mut kind = COREWEBVIEW2_PERMISSION_KIND_MICROPHONE;
                unsafe { args.PermissionKind(&mut kind)? };
                if kind == COREWEBVIEW2_PERMISSION_KIND_MICROPHONE
                    || kind == COREWEBVIEW2_PERMISSION_KIND_CAMERA
                {
                    unsafe { args.SetState(COREWEBVIEW2_PERMISSION_STATE_ALLOW) }?;
                }
            }
            Ok(())
        }));

        let mut token = Default::default();
        let _ = unsafe { core.add_PermissionRequested(&handler, &mut token) };
        std::mem::forget(handler);
        registered.insert(window_label.to_string());
    }

    fn set_permission_state(
        window: &WebviewWindow,
        kind: webview2_com::Microsoft::Web::WebView2::Win32::COREWEBVIEW2_PERMISSION_KIND,
        state: webview2_com::Microsoft::Web::WebView2::Win32::COREWEBVIEW2_PERMISSION_STATE,
    ) {
        let win = window.clone();
        let label = window.label().to_string();
        let _ = win.with_webview(move |platform| {
            unsafe {
                let Ok(core) = platform.controller().CoreWebView2() else {
                    return;
                };

                register_permission_handler(&core, &label);

                let Ok(core13) = core.cast::<ICoreWebView2_13>() else {
                    return;
                };
                let Ok(profile) = core13.Profile() else {
                    return;
                };
                let Ok(profile4) = profile.cast::<ICoreWebView2Profile4>() else {
                    return;
                };

                // 打包版 Origin 可能与 dev 列表不同，按当前 Source 动态授权
                let mut origins: Vec<String> = MEDIA_ORIGINS.iter().map(|s| (*s).to_string()).collect();
                let mut uri = PWSTR::null();
                if core.Source(&mut uri).is_ok() && !uri.is_null() {
                    let source = uri.to_string().unwrap_or_default();
                    let dynamic = origin_from_source(&source);
                    if !dynamic.is_empty() && !origins.iter().any(|o| o == &dynamic) {
                        origins.push(dynamic);
                    }
                }

                for origin in origins {
                    set_permission_for_origin(&profile4, kind, state, &origin);
                }
            }
        });
    }

    fn allow_kind(
        window: &WebviewWindow,
        kind: webview2_com::Microsoft::Web::WebView2::Win32::COREWEBVIEW2_PERMISSION_KIND,
    ) {
        set_permission_state(window, kind, COREWEBVIEW2_PERMISSION_STATE_ALLOW);
    }

    fn reset_kind(
        window: &WebviewWindow,
        kind: webview2_com::Microsoft::Web::WebView2::Win32::COREWEBVIEW2_PERMISSION_KIND,
    ) {
        set_permission_state(window, kind, COREWEBVIEW2_PERMISSION_STATE_DEFAULT);
        set_permission_state(window, kind, COREWEBVIEW2_PERMISSION_STATE_ALLOW);
    }

    /// 启动时为桌宠 / 聊天 WebView 预授权麦克风与摄像头
    pub fn allow_media(window: &WebviewWindow) {
        allow_kind(window, COREWEBVIEW2_PERMISSION_KIND_MICROPHONE);
        allow_kind(window, COREWEBVIEW2_PERMISSION_KIND_CAMERA);
    }

    /// 重置后再次允许（用户曾在 WebView 里点「阻止」时）
    pub fn reset_media(window: &WebviewWindow) {
        reset_kind(window, COREWEBVIEW2_PERMISSION_KIND_MICROPHONE);
        reset_kind(window, COREWEBVIEW2_PERMISSION_KIND_CAMERA);
    }
}

#[cfg(windows)]
pub use imp::{allow_media, reset_media};

#[cfg(not(windows))]
pub fn allow_media(_window: &WebviewWindow) {}

#[cfg(not(windows))]
pub fn reset_media(_window: &WebviewWindow) {}

/// 为 pet / chat 窗口批量预授权麦克风与摄像头
pub fn allow_media_for_app(windows: &[WebviewWindow]) {
    for w in windows {
        allow_media(w);
    }
}

/// 重置 pet / chat 的 WebView2 媒体权限
pub fn reset_media_for_app(windows: &[WebviewWindow]) {
    for w in windows {
        reset_media(w);
    }
}

/// 重置 pet / chat 的 WebView2 媒体权限（兼容旧 Tauri command 名）
pub fn reset_microphone_for_app(windows: &[WebviewWindow]) {
    reset_media_for_app(windows);
}
