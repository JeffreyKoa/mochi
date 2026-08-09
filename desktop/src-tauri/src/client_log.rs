//! 桌宠/聊天 WebView 前端日志落盘（与 Go logs/mochi 并列，目录 logs/desktop）。

use std::fs::{self, OpenOptions};
use std::io::Write;
use std::path::{Path, PathBuf};
use std::sync::Mutex;

use chrono::Local;
use tauri::AppHandle;

#[cfg(not(debug_assertions))]
use crate::voice_sidecar;

static CLIENT_LOG_MUTEX: Mutex<()> = Mutex::new(());

/// 开发态（debug 构建）：仓库 logs/desktop；Release 安装版：%LOCALAPPDATA%/Mochi/logs/desktop。
pub fn client_log_dir(app: &AppHandle) -> PathBuf {
    if let Ok(dir) = std::env::var("MOCHI_CLIENT_LOG_DIR") {
        if !dir.trim().is_empty() {
            return PathBuf::from(dir);
        }
    }

    // debug 构建时 target/debug/bundle/voice 可能存在，不能据此判定为 Release
    #[cfg(debug_assertions)]
    {
        let _ = app;
        let dir = PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../logs/desktop");
        return dir.canonicalize().unwrap_or(dir);
    }

    #[cfg(not(debug_assertions))]
    {
        if voice_sidecar::resolve_bundled_voice_root(app).is_some() {
            voice_sidecar::voice_log_dir().join("desktop")
        } else {
            let dir = PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../logs/desktop");
            dir.canonicalize().unwrap_or(dir)
        }
    }
}

/// 应用启动时写一条 Rust 侧日志，确认目录可写（不依赖前端 invoke 权限）。
pub fn bootstrap_startup_log(app: &AppHandle) -> Result<(), String> {
    let dir = client_log_dir(app);
    let ts = Local::now().format("%Y/%m/%d %H:%M:%S");
    let line = format!(
        "{ts} [rust][INFO] client log dir ready path={}",
        dir.display()
    );
    append_lines(app, &[line])?;

    // 便于在仓库 logs/desktop 下确认实际写盘路径
    #[cfg(debug_assertions)]
    {
        let hint = dir.join("_active_log_is_here.txt");
        let _ = std::fs::write(
            &hint,
            format!(
                "Desktop client logs for this dev session are written here.\nFile: desktop-{}.log\n",
                Local::now().format("%Y%m%d")
            ),
        );
    }
    Ok(())
}

fn client_log_path(app: &AppHandle) -> Result<PathBuf, String> {
    let dir = client_log_dir(app);
    fs::create_dir_all(&dir).map_err(|e| format!("create client log dir: {e}"))?;
    let date = Local::now().format("%Y%m%d");
    Ok(dir.join(format!("desktop-{date}.log")))
}

fn append_lines(app: &AppHandle, lines: &[String]) -> Result<(), String> {
    if lines.is_empty() {
        return Ok(());
    }
    let _guard = CLIENT_LOG_MUTEX
        .lock()
        .map_err(|e| format!("client log lock: {e}"))?;
    let path = client_log_path(app)?;
    let mut file = OpenOptions::new()
        .create(true)
        .append(true)
        .open(&path)
        .map_err(|e| format!("open client log {}: {e}", path.display()))?;
    for line in lines {
        writeln!(file, "{line}").map_err(|e| format!("write client log: {e}"))?;
    }
    Ok(())
}

#[tauri::command]
pub fn append_client_logs(app: AppHandle, lines: Vec<String>) -> Result<(), String> {
    append_lines(&app, &lines)
}

#[tauri::command]
pub fn get_client_log_dir(app: AppHandle) -> String {
    client_log_dir(&app).to_string_lossy().into_owned()
}

#[tauri::command]
pub fn open_client_logs(app: AppHandle) -> Result<(), String> {
    let dir = client_log_dir(&app);
    fs::create_dir_all(&dir).map_err(|e| format!("create client log dir: {e}"))?;
    open_dir_in_explorer(&dir)
}

fn open_dir_in_explorer(dir: &Path) -> Result<(), String> {
    #[cfg(windows)]
    {
        std::process::Command::new("explorer")
            .arg(dir)
            .spawn()
            .map_err(|e| format!("open explorer: {e}"))?;
        Ok(())
    }
    #[cfg(not(windows))]
    {
        let _ = dir;
        Err("open_client_logs is only supported on Windows".into())
    }
}
