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

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
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
