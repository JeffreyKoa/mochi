//! 本地 X-ASR / X-TTS sidecar 生命周期：Tauri 启动时拉起，退出时终止。
//! Release：使用 bundle/voice 内置 Python + 模型；Debug：使用仓库 tools/ 下 venv。

#[cfg(windows)]
use std::os::windows::process::CommandExt;

use serde::Serialize;
use std::net::TcpListener;
use std::path::{Path, PathBuf};
use std::process::{Child, Command, Stdio};
use std::sync::Mutex;
use std::time::{Duration, Instant};
use tauri::{AppHandle, Manager};

const XASR_PORT: u16 = 8766;
const XTTS_PORT: u16 = 8767;

/// 单个 sidecar 运行状态（供前端 diag / 设置页展示）。
#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SidecarServiceStatus {
    pub state: String,
    pub managed: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub message: Option<String>,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct VoiceSidecarStatus {
    pub managed: bool,
    pub bundle_mode: String,
    pub xasr: SidecarServiceStatus,
    pub xtts: SidecarServiceStatus,
}

struct SidecarSlot {
    name: &'static str,
    port: u16,
    child: Option<Child>,
    managed: bool,
    state: String,
    message: Option<String>,
}

impl SidecarSlot {
    fn new(name: &'static str, port: u16) -> Self {
        Self {
            name,
            port,
            child: None,
            managed: false,
            state: "stopped".into(),
            message: None,
        }
    }

    fn status(&self) -> SidecarServiceStatus {
        SidecarServiceStatus {
            state: self.state.clone(),
            managed: self.managed,
            message: self.message.clone(),
        }
    }
}

pub struct VoiceSidecarManager {
    xasr: Mutex<SidecarSlot>,
    xtts: Mutex<SidecarSlot>,
    bundle_mode: String,
}

impl VoiceSidecarManager {
    pub fn new(bundle_mode: &str) -> Self {
        Self {
            xasr: Mutex::new(SidecarSlot::new("x-asr", XASR_PORT)),
            xtts: Mutex::new(SidecarSlot::new("x-tts", XTTS_PORT)),
            bundle_mode: bundle_mode.into(),
        }
    }

    /// 后台启动两个 sidecar（setup 中调用，不阻塞 UI）。
    pub fn start_all_async(self: &std::sync::Arc<Self>, app: AppHandle) {
        let mgr = self.clone();
        std::thread::spawn(move || {
            mgr.start_one(&app, true);
            mgr.start_one(&app, false);
        });
    }

    pub fn stop_all(&self) {
        self.stop_slot(&self.xasr);
        self.stop_slot(&self.xtts);
    }

    pub fn status(&self) -> VoiceSidecarStatus {
        let xasr = self.xasr.lock().unwrap();
        let xtts = self.xtts.lock().unwrap();
        let managed = xasr.managed || xtts.managed;
        VoiceSidecarStatus {
            managed,
            bundle_mode: self.bundle_mode.clone(),
            xasr: xasr.status(),
            xtts: xtts.status(),
        }
    }

    pub fn restart_all(&self, app: &AppHandle) {
        self.stop_all();
        self.start_one(app, true);
        self.start_one(app, false);
    }

    fn stop_slot(&self, slot_mutex: &Mutex<SidecarSlot>) {
        let mut slot = slot_mutex.lock().unwrap();
        if let Some(child) = slot.child.take() {
            kill_process_tree(child);
        }
        if slot.managed {
            slot.state = "stopped".into();
        }
        slot.managed = false;
    }

    fn start_one(&self, app: &AppHandle, is_xasr: bool) {
        let slot_mutex = if is_xasr { &self.xasr } else { &self.xtts };
        let mut slot = slot_mutex.lock().unwrap();
        slot.state = "starting".into();
        slot.message = None;

        let port = slot.port;
        if port_in_use(port) {
            slot.state = "external".into();
            slot.managed = false;
            slot.message = Some(format!("port {port} already in use — reusing existing service"));
            eprintln!("[voice-sidecar] {}: port {} busy, reuse", slot.name, port);
            return;
        }

        let launch = match resolve_launch(app, is_xasr) {
            Ok(v) => v,
            Err(e) => {
                slot.state = "skipped".into();
                slot.managed = false;
                slot.message = Some(e);
                return;
            }
        };

        match spawn_hidden(&launch, slot.name) {
            Ok(mut child) => {
                let wait_secs = if is_xasr { 90 } else { 45 };
                match wait_sidecar_ready(port, &mut child, wait_secs) {
                    Ok(()) => {
                        slot.child = Some(child);
                        slot.managed = true;
                        slot.state = "running".into();
                        eprintln!(
                            "[voice-sidecar] {} ready on port {} ({})",
                            slot.name, port, launch.summary
                        );
                    }
                    Err(e) => {
                        kill_process_tree(child);
                        slot.state = "error".into();
                        slot.managed = false;
                        slot.message = Some(e.clone());
                        eprintln!("[voice-sidecar] {} failed: {}", slot.name, e);
                    }
                }
            }
            Err(e) => {
                slot.state = "error".into();
                slot.managed = false;
                slot.message = Some(e.clone());
                eprintln!("[voice-sidecar] {} spawn failed: {}", slot.name, e);
            }
        }
    }
}

struct LaunchSpec {
    program: PathBuf,
    args: Vec<String>,
    cwd: PathBuf,
    env: Vec<(String, String)>,
    summary: String,
}

fn resolve_launch(app: &AppHandle, is_xasr: bool) -> Result<LaunchSpec, String> {
    if let Some(bundle) = resolve_bundled_voice_root(app) {
        return build_bundled_launch(&bundle, is_xasr);
    }
    build_dev_launch(is_xasr)
}

/// Release 安装包 voice 根目录（NSIS 解压到 `$INSTDIR/bundle/voice`，非 resources/ 下）。
pub fn resolve_bundled_voice_root(app: &AppHandle) -> Option<PathBuf> {
    let mut candidates: Vec<PathBuf> = Vec::new();

    if let Ok(resource) = app.path().resource_dir() {
        candidates.push(resource.join("bundle").join("voice"));
        // 部分布局：resources 与 bundle 同级
        if let Some(parent) = resource.parent() {
            candidates.push(parent.join("bundle").join("voice"));
        }
    }
    if let Ok(exe) = std::env::current_exe() {
        if let Some(dir) = exe.parent() {
            candidates.push(dir.join("bundle").join("voice"));
            candidates.push(dir.join("resources").join("bundle").join("voice"));
        }
    }

    for bundled in candidates {
        let python = bundled.join("runtime").join("python.exe");
        if python.exists() {
            eprintln!("[voice-sidecar] bundled voice root: {}", bundled.display());
            return Some(bundled);
        }
    }
    None
}

fn resolve_dev_tools_root() -> PathBuf {
    if let Ok(root) = std::env::var("MOCHI_TOOLS_ROOT") {
        return PathBuf::from(root);
    }
    #[cfg(debug_assertions)]
    {
        PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../tools")
    }
    #[cfg(not(debug_assertions))]
    {
        PathBuf::from("tools")
    }
}

fn sidecar_log_path(name: &str) -> PathBuf {
    let base = std::env::var("LOCALAPPDATA")
        .map(PathBuf::from)
        .unwrap_or_else(|_| PathBuf::from("."));
    base.join("Mochi").join("logs").join(format!("{name}.log"))
}

/// 等待 sidecar 监听端口（模型加载可能 10–30s）。
fn wait_sidecar_ready(port: u16, child: &mut Child, timeout_secs: u64) -> Result<(), String> {
    let deadline = Instant::now() + Duration::from_secs(timeout_secs);
    while Instant::now() < deadline {
        if let Ok(Some(status)) = child.try_wait() {
            let log = sidecar_log_path(if port == XASR_PORT { "x-asr" } else { "x-tts" });
            return Err(format!(
                "sidecar exited early (code={status}); see {}",
                log.display()
            ));
        }
        if port_in_use(port) {
            std::thread::sleep(Duration::from_millis(300));
            return Ok(());
        }
        std::thread::sleep(Duration::from_millis(500));
    }
    Err(format!("sidecar port {port} not ready within {timeout_secs}s"))
}

fn build_dev_launch(is_xasr: bool) -> Result<LaunchSpec, String> {
    let tools = resolve_dev_tools_root();
    if is_xasr {
        let root = tools.join("x-asr");
        let python = root.join(".venv/Scripts/python.exe");
        let server = root.join("infer/sherpa_streaming_server.py");
        let model_dir = pick_xasr_model_dir(&root)?;
        let chunk = if model_dir.ends_with("480ms-model") {
            "480ms"
        } else {
            "160ms"
        };
        ensure_exists(&python, "X-ASR venv python")?;
        ensure_exists(&server, "X-ASR server script")?;
        Ok(LaunchSpec {
            program: python,
            cwd: root.clone(),
            args: vec![
                server.to_string_lossy().into_owned(),
                "--host".into(),
                "127.0.0.1".into(),
                "--port".into(),
                XASR_PORT.to_string(),
                "--tokens".into(),
                model_dir.join("tokens.txt").to_string_lossy().into_owned(),
                "--encoder".into(),
                model_dir.join(format!("encoder-{chunk}.onnx")).to_string_lossy().into_owned(),
                "--decoder".into(),
                model_dir.join(format!("decoder-{chunk}.onnx")).to_string_lossy().into_owned(),
                "--joiner".into(),
                model_dir.join(format!("joiner-{chunk}.onnx")).to_string_lossy().into_owned(),
                "--provider".into(),
                "cpu".into(),
                "--sample-rate".into(),
                "16000".into(),
                "--feature-dim".into(),
                "80".into(),
                "--num-threads".into(),
                "4".into(),
                "--decoding-method".into(),
                "greedy_search".into(),
                "--model-type".into(),
                "zipformer2".into(),
                "--enable-endpoint-detection".into(),
                "0".into(),
                "--text-format".into(),
                "none".into(),
            ],
            env: vec![],
            summary: "dev x-asr venv".into(),
        })
    } else {
        let root = tools.join("x-tts");
        let python = root.join(".venv/Scripts/python.exe");
        let server = root.join("infer/tts_server.py");
        ensure_exists(&python, "X-TTS venv python")?;
        ensure_exists(&server, "X-TTS server script")?;
        ensure_exists(
            &root.join("models/matcha-zh-en/model-steps-3.onnx"),
            "X-TTS acoustic model",
        )?;
        Ok(LaunchSpec {
            program: python,
            cwd: root.clone(),
            args: vec![
                server.to_string_lossy().into_owned(),
                "--host".into(),
                "127.0.0.1".into(),
                "--port".into(),
                XTTS_PORT.to_string(),
                "--model-dir".into(),
                root.join("models/matcha-zh-en").to_string_lossy().into_owned(),
                "--vocoder".into(),
                root.join("models/vocos-16khz-univ.onnx")
                    .to_string_lossy()
                    .into_owned(),
                "--num-threads".into(),
                "2".into(),
            ],
            env: vec![],
            summary: "dev x-tts venv".into(),
        })
    }
}

fn build_bundled_launch(voice_root: &Path, is_xasr: bool) -> Result<LaunchSpec, String> {
    let python = voice_root.join("runtime/python.exe");
    ensure_exists(&python, "bundled python.exe")?;
    let runtime = voice_root.join("runtime");
    let site_packages = runtime.join("Lib/site-packages");
    let sherpa_lib = site_packages.join("sherpa_onnx/lib");
    let path_prefix = [
        runtime.to_string_lossy().into_owned(),
        sherpa_lib.to_string_lossy().into_owned(),
    ]
    .join(";");
    let system_path = std::env::var("PATH").unwrap_or_default();
    let py_path = (
        "PYTHONPATH".to_string(),
        site_packages.to_string_lossy().into_owned(),
    );
    let path_env = ("PATH".to_string(), format!("{path_prefix};{system_path}"));

    if is_xasr {
        let root = voice_root.join("x-asr");
        let model_dir = pick_xasr_model_dir(&root)?;
        let chunk = if model_dir.ends_with("480ms-model") {
            "480ms"
        } else {
            "160ms"
        };
        Ok(LaunchSpec {
            program: python,
            cwd: root.clone(),
            args: vec![
                "infer/sherpa_streaming_server.py".into(),
                "--host".into(),
                "127.0.0.1".into(),
                "--port".into(),
                XASR_PORT.to_string(),
                "--tokens".into(),
                model_dir.join("tokens.txt").to_string_lossy().into_owned(),
                "--encoder".into(),
                model_dir.join(format!("encoder-{chunk}.onnx")).to_string_lossy().into_owned(),
                "--decoder".into(),
                model_dir.join(format!("decoder-{chunk}.onnx")).to_string_lossy().into_owned(),
                "--joiner".into(),
                model_dir.join(format!("joiner-{chunk}.onnx")).to_string_lossy().into_owned(),
                "--provider".into(),
                "cpu".into(),
                "--sample-rate".into(),
                "16000".into(),
                "--feature-dim".into(),
                "80".into(),
                "--num-threads".into(),
                "4".into(),
                "--decoding-method".into(),
                "greedy_search".into(),
                "--model-type".into(),
                "zipformer2".into(),
                "--enable-endpoint-detection".into(),
                "0".into(),
                "--text-format".into(),
                "none".into(),
            ],
            env: vec![py_path, path_env],
            summary: "bundled x-asr".into(),
        })
    } else {
        let root = voice_root.join("x-tts");
        Ok(LaunchSpec {
            program: python,
            cwd: root.clone(),
            args: vec![
                "infer/tts_server.py".into(),
                "--host".into(),
                "127.0.0.1".into(),
                "--port".into(),
                XTTS_PORT.to_string(),
                "--model-dir".into(),
                "models/matcha-zh-en".into(),
                "--vocoder".into(),
                "models/vocos-16khz-univ.onnx".into(),
                "--num-threads".into(),
                "2".into(),
            ],
            env: vec![py_path, path_env],
            summary: "bundled x-tts".into(),
        })
    }
}

fn pick_xasr_model_dir(root: &Path) -> Result<PathBuf, String> {
    let m160 = root.join("models/chunk-160ms-model");
    if m160.join("encoder-160ms.onnx").exists() {
        return Ok(m160);
    }
    let m480 = root.join("models/chunk-480ms-model");
    if m480.join("encoder-480ms.onnx").exists() {
        return Ok(m480);
    }
    Err("X-ASR models not found in bundle/tools".into())
}

fn ensure_exists(path: &Path, label: &str) -> Result<(), String> {
    if path.exists() {
        Ok(())
    } else {
        Err(format!("{label} missing: {}", path.display()))
    }
}

fn port_in_use(port: u16) -> bool {
    TcpListener::bind(("127.0.0.1", port)).is_err()
}

fn spawn_hidden(launch: &LaunchSpec, log_name: &str) -> Result<Child, String> {
    let log_path = sidecar_log_path(log_name);
    if let Some(parent) = log_path.parent() {
        let _ = std::fs::create_dir_all(parent);
    }
    let log_file = std::fs::OpenOptions::new()
        .create(true)
        .append(true)
        .open(&log_path)
        .map_err(|e| format!("open log {}: {e}", log_path.display()))?;

    let mut cmd = Command::new(&launch.program);
    cmd.current_dir(&launch.cwd)
        .args(&launch.args)
        .stdin(Stdio::null())
        .stdout(Stdio::from(log_file.try_clone().map_err(|e| e.to_string())?))
        .stderr(Stdio::from(log_file));
    for (k, v) in &launch.env {
        cmd.env(k, v);
    }
    #[cfg(windows)]
    {
        const CREATE_NO_WINDOW: u32 = 0x0800_0000;
        cmd.creation_flags(CREATE_NO_WINDOW);
    }
    cmd.spawn().map_err(|e| format!("spawn failed: {e}"))
}

fn kill_process_tree(child: Child) {
    let pid = child.id();
    #[cfg(windows)]
    {
        const CREATE_NO_WINDOW: u32 = 0x0800_0000;
        let _ = Command::new("taskkill")
            .args(["/PID", &pid.to_string(), "/T", "/F"])
            .creation_flags(CREATE_NO_WINDOW)
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .status();
    }
    #[cfg(not(windows))]
    {
        let _ = child.kill();
    }
}

#[tauri::command]
pub fn get_voice_sidecar_status(
    manager: tauri::State<'_, std::sync::Arc<VoiceSidecarManager>>,
) -> VoiceSidecarStatus {
    manager.status()
}

#[tauri::command]
pub fn restart_voice_sidecars(
    app: AppHandle,
    manager: tauri::State<'_, std::sync::Arc<VoiceSidecarManager>>,
) -> VoiceSidecarStatus {
    manager.restart_all(&app);
    manager.status()
}
