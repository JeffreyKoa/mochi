//! Windows WebView2：预授权 / 重置麦克风与摄像头（Tauri 桌宠 dev + 打包场景）

use tauri::WebviewWindow;

#[cfg(windows)]
mod imp {
    use super::WebviewWindow;
    use webview2_com::Microsoft::Web::WebView2::Win32::{
        COREWEBVIEW2_PERMISSION_KIND_CAMERA, COREWEBVIEW2_PERMISSION_KIND_MICROPHONE,
        COREWEBVIEW2_PERMISSION_STATE_ALLOW, COREWEBVIEW2_PERMISSION_STATE_DEFAULT,
        ICoreWebView2Profile4, ICoreWebView2_13,
    };
    use windows_core::{Interface, PCWSTR};

    /// dev / 打包常见 Origin（WebView2 按站点记权限）
    const MEDIA_ORIGINS: &[&str] = &[
        "http://localhost:1420",
        "http://localhost:1421",
        "http://127.0.0.1:1420",
        "http://127.0.0.1:1421",
        "https://tauri.localhost",
        "http://tauri.localhost",
    ];

    fn set_permission_state(
        window: &WebviewWindow,
        kind: webview2_com::Microsoft::Web::WebView2::Win32::COREWEBVIEW2_PERMISSION_KIND,
        state: webview2_com::Microsoft::Web::WebView2::Win32::COREWEBVIEW2_PERMISSION_STATE,
    ) {
        let win = window.clone();
        let _ = win.with_webview(move |platform| {
            unsafe {
                let Ok(core) = platform.controller().CoreWebView2() else {
                    return;
                };
                let Ok(core13) = core.cast::<ICoreWebView2_13>() else {
                    return;
                };
                let Ok(profile) = core13.Profile() else {
                    return;
                };
                let Ok(profile4) = profile.cast::<ICoreWebView2Profile4>() else {
                    return;
                };

                for origin in MEDIA_ORIGINS {
                    let wide: Vec<u16> = origin.encode_utf16().chain(std::iter::once(0)).collect();
                    let origin_pw = PCWSTR::from_raw(wide.as_ptr());
                    let _ = profile4.SetPermissionState(kind, origin_pw, state, None);
                }
            }
        });
    }

    fn allow_kind(window: &WebviewWindow, kind: webview2_com::Microsoft::Web::WebView2::Win32::COREWEBVIEW2_PERMISSION_KIND) {
        set_permission_state(window, kind, COREWEBVIEW2_PERMISSION_STATE_ALLOW);
    }

    fn reset_kind(window: &WebviewWindow, kind: webview2_com::Microsoft::Web::WebView2::Win32::COREWEBVIEW2_PERMISSION_KIND) {
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
