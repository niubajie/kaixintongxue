use std::net::TcpListener;
use std::sync::Mutex;

use tauri::{AppHandle, Manager, State};
use tauri_plugin_shell::process::CommandChild;
use tauri_plugin_shell::ShellExt;

/// 运行时状态：本地 Go 服务的端口与子进程句柄。
struct LocalService {
    port: Mutex<Option<u16>>,
    child: Mutex<Option<CommandChild>>,
}

#[tauri::command]
fn app_version() -> String {
    env!("CARGO_PKG_VERSION").to_string()
}

/// 前端读取本地服务端口；sidecar 启动失败时返回 0，前端据此提示用户。
#[tauri::command]
fn local_service_port(state: State<'_, LocalService>) -> u16 {
    state.port.lock().unwrap().unwrap_or(0)
}

/// 抢占一个空闲端口：绑定到 0 端口后立即释放，返回系统分配的端口。
fn free_port() -> Option<u16> {
    TcpListener::bind("127.0.0.1:0")
        .ok()
        .and_then(|l| l.local_addr().ok())
        .map(|a| a.port())
}

fn spawn_sidecar(app: &AppHandle) -> Result<(u16, CommandChild), String> {
    let port = free_port().ok_or_else(|| "无法分配本地端口".to_string())?;
    let mut command = app
        .shell()
        .sidecar("kaixin-local")
        .map_err(|e| format!("加载 sidecar 失败：{e}"))?;
    command = command.env("KAIXIN_PORT", port.to_string());
    let (mut rx, child) = command
        .spawn()
        .map_err(|e| format!("启动本地服务失败：{e}"))?;

    // 异步丢弃 sidecar 的 stdout/stderr，避免管道写满阻塞子进程。
    tauri::async_runtime::spawn(async move {
        while let Some(line) = rx.recv().await {
            let _ = line;
        }
    });

    Ok((port, child))
}

/// 启动失败兜底：WebView2 异常时 Tauri 会在 run() 内部 panic，
/// 窗口一闪而过、用户看不到任何原因。这里接管 panic：先落日志，再用记事本把原因显示出来。
fn report_startup_failure(detail: &str) {
    // 逐个候选路径尝试，全部失败时把原因打到 stderr（会进控制台/事件日志）。
    let mut candidates: Vec<std::path::PathBuf> = Vec::new();
    if let Ok(exe) = std::env::current_exe() {
        if let Some(dir) = exe.parent() {
            candidates.push(dir.join("kaixin-startup-error.txt"));
        }
    }
    candidates.push(std::env::temp_dir().join("kaixin-startup-error.txt"));
    if let Ok(base) = std::env::var("LOCALAPPDATA") {
        candidates.push(std::path::PathBuf::from(base).join("Kaixin").join("logs").join("startup-error.txt"));
    }

    let body = format!(
        "开心同学 AI 助手 启动失败\n==============================\n\n技术原因：\n{}\n\n如果上面出现 WebView2 / 0x8000FFFF 之类的字样，说明本机的 Microsoft Edge WebView2 Runtime 损坏，界面无法创建。\n\n修复办法（任选其一）：\n1) 打开 Edge → 设置 → 关于 Microsoft Edge，等待更新完成，重启程序；\n2) 运行桌面上的「修复WebView2.ps1」，完成后重启程序；\n3) 从 https://developer.microsoft.com/microsoft-edge/webview2/ 下载 Evergreen Standalone Installer 重新安装。\n",
        detail
    );

    for path in &candidates {
        match std::fs::write(path, &body) {
            Ok(_) => {
                eprintln!("启动失败说明已写入 {}", path.display());
                let _ = std::process::Command::new("notepad").arg(path).spawn();
                return;
            }
            Err(err) => eprintln!("写入 {} 失败: {}", path.display(), err),
        }
    }
    // 全都写不进去时，至少把原因打到 stderr。
    eprintln!("启动失败原因（无法写入日志文件）：{}", detail);
}

/// 安装一个 panic 钩子，保证任何启动期崩溃都能被看到。
fn install_panic_hook() {
    let default_hook = std::panic::take_hook();
    std::panic::set_hook(Box::new(move |info| {
        report_startup_failure(&info.to_string());
        default_hook(info);
    }));
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    install_panic_hook();

    tauri::Builder::default()
        // 单实例插件必须最先注册：第二次启动时不再开新进程，而是聚焦已有窗口。
        // 否则两个实例会各自拉起 sidecar、抢占同一个本地端口，用户只会看到"打不开"。
        .plugin(tauri_plugin_single_instance::init(|app, _args, _cwd| {
            if let Some(window) = app.get_webview_window("main") {
                let _ = window.unminimize();
                let _ = window.show();
                let _ = window.set_focus();
            }
        }))
        .plugin(tauri_plugin_shell::init())
        .setup(|app| {
            let handle = app.handle().clone();
            let service = LocalService {
                port: Mutex::new(None),
                child: Mutex::new(None),
            };
            app.manage(service);

            // 启动时拉起 Go sidecar；失败不阻断窗口，前端会提示。
            match spawn_sidecar(&handle) {
                Ok((port, child)) => {
                    let state: State<'_, LocalService> = handle.state();
                    *state.port.lock().unwrap() = Some(port);
                    *state.child.lock().unwrap() = Some(child);
                    eprintln!("kaixin-local sidecar started on port {port}");
                }
                Err(e) => {
                    eprintln!("kaixin-local sidecar failed: {e}");
                }
            }
            Ok(())
        })
        .on_window_event(|window, event| {
            // 窗口关闭时杀掉 sidecar，避免残留进程占用端口。
            if let tauri::WindowEvent::Destroyed = event {
                let app = window.app_handle();
                if let Some(state) = app.try_state::<LocalService>() {
                    if let Some(child) = state.child.lock().unwrap().take() {
                        let _ = child.kill();
                    }
                }
            }
        })
        .invoke_handler(tauri::generate_handler![app_version, local_service_port])
        .run(tauri::generate_context!())
        .expect("启动开心同学桌面端失败");
}
