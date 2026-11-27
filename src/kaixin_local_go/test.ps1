# 运行 Go 测试（Windows）。
#
# 为什么不用直接 go test：
#   go test 会把测试二进制放到 %TEMP%\go-buildNNN\b001 下运行，那里没有
#   sherpa-onnx / onnxruntime 的 DLL。Windows 加载器按「exe 目录 → System32」
#   的顺序找依赖，于是命中系统自带的旧版 C:\Windows\System32\onnxruntime.dll
#   （ONNX Runtime 1.0，2019 年），与 sherpa-onnx 需要的版本不兼容，
#   进程启动即报 0xc000007b。
#   把测试二进制编译到本目录（DLL 就在旁边）再运行，加载器会优先用随包发布的
#   新版本，问题消失。
#
# 用法：
#   .\test.ps1                 # 跑全部测试（带 -race，与 AGENTS.md 一致）
#   .\test.ps1 -Run TestPath   # 只跑匹配的测试
#   .\test.ps1 -Verbose        # 输出每个测试的日志
param(
    [string]$Run = "",
    [switch]$Verbose
)

$ErrorActionPreference = "Stop"
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$exe = Join-Path $here "kaixinlocal.test.exe"

Push-Location $here
try {
    go test -c -race -o $exe .
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }
    $testArgs = @("-test.timeout=10m")
    if ($Run) { $testArgs += "-test.run=$Run" }
    if ($Verbose) { $testArgs += "-test.v" }
    & $exe @testArgs
    exit $LASTEXITCODE
} finally {
    Remove-Item $exe -Force -ErrorAction SilentlyContinue
    Pop-Location
}
