# 构建后补签：Tauri 的 signCommand 在 NSIS 流程中对 release 副本可能被覆盖，
# 这里在 npm run tauri build 之后兜底签名主程序与 sidecar（幂等，可重复执行）。
# 依赖 sign-windows.ps1 的环境变量约定（KAIXIN_SIGN_PFX / KAIXIN_SIGN_PFX_PASSWORD
# 或 KAIXIN_SIGN_THUMBPRINT）。

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot

$candidates = @(
    (Join-Path $root "src-tauri\target\release\kaixin-desktop.exe"),
    (Join-Path $root "src-tauri\target\release\kaixin-local.exe"),
    (Join-Path $root "src-tauri\binaries\kaixin-local-x86_64-pc-windows-msvc.exe")
)

$signer = Join-Path $PSScriptRoot "sign-windows.ps1"

foreach ($file in $candidates) {
    if (-not (Test-Path -LiteralPath $file)) {
        Write-Warning "跳过不存在的文件：$file"
        continue
    }
    & $signer $file
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }
}

Write-Host "补签完成（分发包仍以 setup.exe 的签名为准）"
