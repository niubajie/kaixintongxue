// 正式版以 Windows GUI 子系统运行，避免启动时额外弹出一个控制台黑窗；
// debug 版保留控制台，方便查看 sidecar 日志。
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

fn main() {
    // 使用独立的 WebView2 数据目录。
    //
    // 默认目录是 %LOCALAPPDATA%\<identifier>\EBWebView，全机共享：
    // 只要有一个残留的 msedgewebview2 进程占着它（崩溃、强杀后会留下，且以管理员身份
    // 也未必能结束），新实例就会以 0x800700AA「请求的资源在使用中」启动失败。
    //
    // 指到自己的目录之后，多实例、崩溃残留都不会互相影响。
    // 可通过环境变量覆盖，便于排障。
    if std::env::var_os("WEBVIEW2_USER_DATA_FOLDER").is_none() {
        if let Some(local) = std::env::var_os("LOCALAPPDATA") {
            let dir = std::path::PathBuf::from(local).join("Kaixin").join("webview2");
            let _ = std::fs::create_dir_all(&dir);
            std::env::set_var("WEBVIEW2_USER_DATA_FOLDER", &dir);
        }
    }

    kaixin_desktop_lib::run();
}
