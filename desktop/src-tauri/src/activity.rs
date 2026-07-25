use std::sync::Mutex;
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};
use serde::Serialize;

const IDLE_BREAK_SECS: u64 = 300;

#[derive(Serialize, Clone)]
pub struct ActivitySnapshot {
    pub idle_seconds: u32,
    pub continuous_active_minutes: u32,
    pub session_active_minutes_today: u32,
}

struct ActivityTracker {
    last_tick: Instant,
    continuous_active_secs: u64,
    session_active_secs_today: u64,
    today_key: String,
}

impl ActivityTracker {
    fn new() -> Self {
        Self {
            last_tick: Instant::now(),
            continuous_active_secs: 0,
            session_active_secs_today: 0,
            today_key: today_key(),
        }
    }

    fn tick(&mut self, idle_secs: u64) {
        let now = Instant::now();
        let elapsed = now.duration_since(self.last_tick).as_secs();
        self.last_tick = now;

        let today = today_key();
        if today != self.today_key {
            self.today_key = today;
            self.session_active_secs_today = 0;
        }

        if idle_secs >= IDLE_BREAK_SECS {
            self.continuous_active_secs = 0;
            return;
        }

        self.continuous_active_secs = self.continuous_active_secs.saturating_add(elapsed);
        self.session_active_secs_today = self.session_active_secs_today.saturating_add(elapsed);
    }

    fn snapshot(&self, idle_secs: u64) -> ActivitySnapshot {
        ActivitySnapshot {
            idle_seconds: idle_secs.min(u32::MAX as u64) as u32,
            continuous_active_minutes: (self.continuous_active_secs / 60).min(u32::MAX as u64) as u32,
            session_active_minutes_today: (self.session_active_secs_today / 60).min(u32::MAX as u64) as u32,
        }
    }
}

fn today_key() -> String {
    let secs = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or(Duration::ZERO)
        .as_secs();
    let day = secs / 86400;
    format!("d{day}")
}

static TRACKER: Mutex<Option<ActivityTracker>> = Mutex::new(None);

fn tracker() -> std::sync::MutexGuard<'static, Option<ActivityTracker>> {
    let mut guard = TRACKER.lock().unwrap_or_else(|e| e.into_inner());
    if guard.is_none() {
        *guard = Some(ActivityTracker::new());
    }
    guard
}

#[cfg(windows)]
fn os_idle_seconds() -> u64 {
    use std::mem::MaybeUninit;
    #[repr(C)]
    struct LastInputInfo {
        cb_size: u32,
        dw_time: u32,
    }
    extern "system" {
        fn GetTickCount() -> u32;
        fn GetLastInputInfo(plii: *mut LastInputInfo) -> i32;
    }
    unsafe {
        let mut info = MaybeUninit::<LastInputInfo>::uninit();
        (*info.as_mut_ptr()).cb_size = std::mem::size_of::<LastInputInfo>() as u32;
        if GetLastInputInfo(info.as_mut_ptr()) == 0 {
            return 0;
        }
        let info = info.assume_init();
        let tick = GetTickCount();
        let elapsed_ms = tick.wrapping_sub(info.dw_time);
        (elapsed_ms as u64) / 1000
    }
}

#[cfg(not(windows))]
fn os_idle_seconds() -> u64 {
    0
}

#[tauri::command]
pub fn get_activity_snapshot() -> ActivitySnapshot {
    let idle = os_idle_seconds();
    let mut guard = tracker();
    if let Some(t) = guard.as_mut() {
        t.tick(idle);
        t.snapshot(idle)
    } else {
        ActivitySnapshot {
            idle_seconds: idle.min(u32::MAX as u64) as u32,
            continuous_active_minutes: 0,
            session_active_minutes_today: 0,
        }
    }
}
