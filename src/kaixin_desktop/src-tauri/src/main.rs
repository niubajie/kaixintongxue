// 正式版以 Windows GUI 子系统运行，避免启动时额外弹出一个控制台黑窗；
// debug 版保留控制台，方便查看 sidecar 日志。
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

fn main() {
    kaixin_desktop_lib::run();
}
