# 桌面端技术方案（Tauri 2）

> 决策：**TypeScript 写业务，Tauri 2 当壳，Go sidecar 扛重活，NSIS 打包。**
> 版本 v1.2 ｜ 替代原 Electron 方案
>
> v1.2 修订（2026-11-26）：第六章补「已落地配置」表（updater 公钥、minisign 私钥位置、自签名证书、构建后补签）；
> 发布流程改为可执行命令；新增「安装与快捷方式」小节，明确升级必须走 `setup.exe` 安装器，
> 桌面快捷方式、开始菜单与卸载项均由安装器创建，手工复制 exe 不会生成。
>
> v1.1 修订（评审 P0-2）：CSP 的本地源改为端口通配 `http://127.0.0.1:*`，
> 与 sidecar 随机端口方案对齐；写死端口会导致请求被 CSP 拦截。
>
> **路径说明**：下文以 `client/`、`desktop-app/` 作为示例目录名；
> 当前实际仓库为 `src/kaixin_desktop/`，按对应关系替换即可，配置内容完全一致。

---

## 一、为什么换成这个

| 维度 | Electron（原方案） | Tauri 2（现方案） |
|---|---|---|
| 安装包 | 80–150 MB | **10–30 MB**（不含模型） |
| 内存占用 | 约 150–250 MB | **约 50–80 MB** |
| 冷启动 | 较慢 | 快 |
| 渲染内核 | 自带 Chromium（完全一致） | 系统 WebView2（版本有差异） |
| Node 依赖 | 有（攻击面大） | **没有**（渲染进程无 Node） |
| 更新签名 | 需买代码签名证书（¥1000+/年） | **minisign 免费签名** |
| 开发前置 | 装 Node 即可 | 需装 Rust + MSVC（约 4–6 GB） |

**关键收益：更新签名免费。** Tauri 的 updater 用自己生成的 minisign 密钥对，不需要 Windows 代码签名证书。首次安装仍会被 SmartScreen 拦一次，之后的自动更新经过应用自己下载，不再触发。

---

## 二、目录结构

```
desktop-app/
├── package.json
├── vite.config.ts              前端构建（root 指向 src/renderer）
├── tsconfig.json
├── dist/                       构建产物（git 忽略）
├── scripts/
│   ├── serve-updates.mjs       本地更新服务器
│   └── make-latest-json.mjs    生成 Tauri 的 latest.json
├── src/
│   ├── renderer/
│   │   ├── index.html
│   │   └── src/
│   │       ├── main.tsx
│   │       ├── App.tsx
│   │       ├── styles.css
│   │       ├── lib/api.ts
│   │       ├── store.ts
│   │       ├── components/
│   │       └── platform/       ★ 平台抽象层（新增，核心）
│   │           ├── types.ts
│   │           ├── tauri.ts
│   │           ├── web.ts
│   │           └── index.ts
└── src-tauri/
    ├── Cargo.toml
    ├── build.rs
    ├── tauri.conf.json
    ├── capabilities/
    │   └── default.json
    ├── icons/                  必须放图标，否则打包失败
    └── src/
        ├── main.rs
        └── lib.rs
```

---

## 三、关键配置

### vite.config.ts

```ts
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  root: 'src/renderer',
  base: './',                    // Tauri 用自定义协议，绝对路径会 404
  clearScreen: false,            // 否则 Rust 编译日志被清屏
  plugins: [react()],
  server: {
    port: 1420,
    strictPort: true,            // 别自动换端口，tauri.conf 里写死了 1420
    watch: { ignored: ['**/src-tauri/**'] }  // 否则 Rust 编译触发无限重载
  },
  envPrefix: ['VITE_', 'TAURI_ENV_*'],
  build: {
    outDir: '../../dist',
    emptyOutDir: true,
    // WebView2 版本因机器而异，不能用 esnext
    target: process.env.TAURI_ENV_PLATFORM === 'windows' ? 'chrome105' : 'safari13',
    minify: process.env.TAURI_ENV_DEBUG ? false : 'esbuild',
    sourcemap: !!process.env.TAURI_ENV_DEBUG
  }
})
```

### tauri.conf.json 要点

| 配置 | 值 | 原因 |
|---|---|---|
| `build.frontendDist` | `"../dist"` | 指向 Vite 产物 |
| `build.devUrl` | `"http://localhost:1420"` | 开发时加载 dev server |
| `bundle.targets` | `["nsis"]` | 只要 NSIS，不生成 MSI |
| `bundle.windows.nsis.installMode` | `"currentUser"` | 免管理员权限，不弹 UAC |
| `bundle.windows.webviewInstallMode` | `embedBootstrapper` | Win10 没装 WebView2 时自动补，+1.8 MB |
| `app.security.csp` | 见下 | 放配置文件，比 index.html 的 meta 可靠 |
| `bundle.createUpdaterArtifacts` | `true` | 生成 `.sig` 签名文件 |

CSP 建议：

```
default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline';
img-src 'self' data: blob:;
connect-src 'self' http://127.0.0.1:* http://localhost:* ipc: http://ipc.localhost
```

> `style-src` 的 `'unsafe-inline'` 是因为 React 内联 style 属性；如果将来改用外部样式表可以去掉。
>
> **端口必须用通配 `http://127.0.0.1:*`。** sidecar 监听的是启动时分配的空闲端口，
> CSP 无法预先知道具体端口；写死 `:8080` 会导致「服务已启动但所有请求被 CSP 拦截」的静默失败。
> 该规则已在 `tauri.conf.json` 中落地（`connect-src 'self' http://127.0.0.1:* http://localhost:* ...`）。

---

## 四、平台抽象层（最重要的设计）

**目的：同一个页面既能跑在 Tauri 里，也能跑在浏览器里。**

### 接口

```ts
export interface DesktopBridge {
  readonly kind: 'tauri' | 'web'
  readonly capabilities: {
    updater: boolean
    filePicker: boolean
    secureStorage: boolean
  }
  getAppInfo(): Promise<AppInfo>
  getUpdateState(): Promise<UpdateState>
  checkForUpdates(): Promise<void>
  installUpdate(): Promise<void>
  onUpdateState(cb: (s: UpdateState) => void): () => void
  secureStorage: {
    get(key: string): Promise<string | null>
    set(key: string, value: string): Promise<void>
    remove(key: string): Promise<void>
  }
}
```

### 三个实现

| 文件 | 行为 |
|---|---|
| `tauri.ts` | 调用 `@tauri-apps/api` 和插件，**必须用动态 import**，否则 Web 包里会混入 Tauri 代码 |
| `web.ts` | 全部降级：更新返回「不支持」，secureStorage 退回 sessionStorage |
| `index.ts` | 启动时探测 `'__TAURI_INTERNALS__' in window`，选一个导出 |

### 组件里的写法

```tsx
import { bridge } from '../platform'

if (bridge.capabilities.updater) {
  // 渲染更新面板
}
```

**组件永远不知道底层是 Tauri 还是浏览器。** 这是换壳零成本的唯一办法。

---

## 五、sidecar：重活交给 Go

### 什么放进去

| 内容 | 理由 |
|---|---|
| ✅ 本地后端（你的 Go 服务，本地模式） | 私域库、检索、文档解析 |
| ✅ OCR 引擎 | 拍照识字 |
| ⚠️ ffmpeg | 注意许可：GPL 构建不能闭源分发，建议用 LGPL 版 |
| ❌ Ollama + 模型权重 | 会让安装包变成 2–5 GB，改为**检测并引导用户自装** |

### 打包方式

```json
// tauri.conf.json
"bundle": {
  "externalBin": ["binaries/agent-server"]
}
```

文件必须按 target triple 命名：

```
src-tauri/binaries/agent-server-x86_64-pc-windows-msvc.exe
```

构建脚本：

```powershell
GOOS=windows GOARCH=amd64 go build -o ../desktop-app/src-tauri/binaries/agent-server-x86_64-pc-windows-msvc.exe ./cmd/api
```

### 运行时

- 由 Rust 侧用 `tauri_plugin_shell` 启动，或在 `lib.rs` 的 `setup` 里 `spawn`
- 监听**随机端口**，通过 IPC 把端口号告诉前端
- 应用退出时**必须杀掉子进程**（`on_window_event` 里处理 + 超时强杀）

> 这几个坑（端口冲突、残留进程、权限声明）是 Tauri sidecar 最常见的故障来源，见第八节。

---

## 六、打包与自动更新

### 首次构建前置

1. 装 **Rust**（rustup，stable 工具链）
2. 装 **MSVC Build Tools**（勾选「使用 C++ 的桌面开发」）
3. 准备图标：`src-tauri/icons/` 下至少要有 `32x32.png`、`128x128.png`、`icon.ico`
   - 用一张 1024×1024 的 PNG 生成：`npm run tauri icon path/to/logo.png`

**缺图标会直接构建失败**，这是新手最常见的卡点。

### 更新密钥（免费）

```bash
npm run tauri signer generate -- -w ~/.tauri/agent-platform.key
```

- 私钥 **绝对不能进 Git**
- 公钥填进 `tauri.conf.json` 的 `plugins.updater.pubkey`
- 密码用环境变量 `TAURI_SIGNING_PRIVATE_KEY_PASSWORD` 传给 CI

**本项目已落地配置（2026-11-25）**

| 项 | 实际值 |
|---|---|
| 更新公钥 | 已写入 `tauri.conf.json > plugins.updater.pubkey` |
| 私钥位置 | `%USERPROFILE%\.tauri\kaixin-desktop.key`（仓库外，密码在同目录 `kaixin-desktop.key.password.txt`） |
| 更新清单地址 | `https://update.kaixin.example.com/desktop/latest.json`，发布前替换为真实域名 |
| 更新包签名 | `bundle.createUpdaterArtifacts: true`，构建产出 `*-setup.exe.sig`（minisign） |
| 代码签名 | `bundle.windows.signCommand` → `scripts/sign-windows.ps1`；证书读 `KAIXIN_SIGN_PFX` + `KAIXIN_SIGN_PFX_PASSWORD` 或 `KAIXIN_SIGN_THUMBPRINT`，未配置时跳过且构建继续 |
| 当前证书 | 自签名测试证书 `CN=Kaixin Assistant Self-Signed`，指纹 `966F4BAB005D43DB4D85479F38A3625FC9BBF2F9`，2029-11-25 到期；PFX 与密码在 `%USERPROFILE%\.tauri\codesign\`，已写入用户环境变量 `KAIXIN_SIGN_PFX` / `KAIXIN_SIGN_PFX_PASSWORD` |
| 信任范围 | 仅本机 CurrentUser 的「受信任的根」与「受信任的发布者」；其他机器仍会提示未知发布者 |
| 构建后补签 | `npm run sign:artifacts`：对 release 主程序与 sidecar 兜底签名（Tauri 的 signCommand 对 release 副本可能被覆盖）；分发包以 `setup.exe` 的签名为准 |
| 换正式证书 | 购买 OV/EV 证书或接入 Azure Trusted Signing 后，替换 `KAIXIN_SIGN_PFX` / `KAIXIN_SIGN_PFX_PASSWORD`（或 `KAIXIN_SIGN_THUMBPRINT`）即可，签名流程与脚本不变 |
| 生成清单 | `npm run make:latest -- --notes "本次更新说明"` |
| 构建签名环境变量 | `TAURI_SIGNING_PRIVATE_KEY`（内容或文件路径）、`TAURI_SIGNING_PRIVATE_KEY_PASSWORD` |

### 发布流程

```powershell
# 0. 证书与更新私钥（CI 里用密钥注入，不要写进仓库）
$env:KAIXIN_SIGN_PFX = "C:\path\to\code-sign.pfx"
$env:KAIXIN_SIGN_PFX_PASSWORD = "***"
$env:TAURI_SIGNING_PRIVATE_KEY = "C:\Users\<user>\.tauri\kaixin-desktop.key"
$env:TAURI_SIGNING_PRIVATE_KEY_PASSWORD = "***"

# 1. 构建：前端 → cargo release → NSIS 打包 → minisign 签名
npm run tauri build

# 2. 兜底补签主程序与 sidecar（Tauri 对 release 副本可能不落签）
npm run sign:artifacts

# 3. 校验 Authenticode（应为 Valid）
Get-AuthenticodeSignature src-tauri/target/release/bundle/nsis/KaixinAssistant_<version>_x64-setup.exe

# 4. 生成更新清单
npm run make:latest -- --notes "本次更新说明"

# 5. 上传 exe + .sig + latest.json 到更新服务器
```

产物在 `src-tauri/target/release/bundle/nsis/`：

```
KaixinAssistant_<version>_x64-setup.exe
KaixinAssistant_<version>_x64-setup.exe.sig
latest.json
```

### 安装与快捷方式（必须走安装器）

- **升级一律用 `setup.exe` 覆盖安装**：交互双击，或静默 `setup.exe /S`。不要手工把 exe 复制到安装目录。
- 安装器负责写入：程序目录、**桌面快捷方式**、开始菜单项、卸载项（`HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\KaixinAssistant`）与版本信息。
- 手工复制只替换主程序，不会创建或更新快捷方式、开始菜单和卸载信息。此前桌面一直没有快捷方式，就是因为更新时只复制了 exe、从未运行安装器。
- 快捷方式名称取 `productName`（当前 `KaixinAssistant`）。要显示中文名，改 `tauri.conf.json` 的 `productName` 后重新打包；已安装的机器也可直接重命名 `.lnk`。
- 静默安装后自检：`Get-ChildItem "$env:USERPROFILE\Desktop" -Filter *.lnk`，并读取目标的 `TargetPath` 确认指向 `kaixin-desktop.exe`。

`latest.json` 结构：

```json
{
  "version": "0.1.1",
  "notes": "修复若干问题",
  "pub_date": "2026-11-15T10:00:00Z",
  "platforms": {
    "windows-x86_64": {
      "signature": "<.sig 文件内容>",
      "url": "https://your-server/AgentPlatform_0.1.1_x64-setup.exe"
    }
  }
}
```

`make-latest-json.mjs` 就是自动拼这个文件，省掉手工复制签名的错误。

---

## 七、从 Electron 迁移的差异清单

| 项 | Electron | Tauri 2 |
|---|---|---|
| 入口 | `main` 字段指向 `out/main/index.js` | `src-tauri/src/main.rs` |
| 渲染加载 | `loadFile` / `loadURL` | `build.frontendDist` / `build.devUrl` |
| 进程通信 | preload + `contextBridge` | `invoke` + capability 声明 |
| 更新清单 | `latest.yml` | **`latest.json`** |
| 更新签名 | 代码签名证书 | **minisign 密钥** |
| 打包配置 | `electron-builder.yml` | `tauri.conf.json` |
| 安装模式 | NSIS oneClick 配置 | `bundle.windows.nsis.installMode` |
| 图标 | `build/icon.ico` | `src-tauri/icons/`（多种尺寸） |
| CSP | index.html meta 或 session | `app.security.csp` |

---

## 八、已知风险

| 风险 | 影响 | 应对 |
|---|---|---|
| Rust 工具链安装失败（MSVC 缺组件） | 构建完全卡住 | 先只装「C++ 桌面开发」工作负载，别装全家桶 |
| WebView2 版本差异导致渲染不一致 | 个别用户界面错位 | 代码保守（target chrome105），避开新 CSS 特性 |
| sidecar 子进程残留 | 任务管理器里越积越多 | 退出钩子 + 超时 kill + 启动时清理同名进程 |
| sidecar 权限未声明 | 静默失败，无报错 | `capabilities/default.json` 里显式加 `shell:allow-execute` |
| 图标缺失 | 构建直接失败 | 第一步就先跑 `tauri icon` |
| 私钥泄露 | 任何人都能伪造更新包 | 私钥只放本地和 CI secrets，绝不进仓库 |
| Rust 编译慢 | 首次 5–15 分钟 | 正常，增量编译很快 |

---

## 九、验证顺序（建议照做）

```
1. 装 Rust + MSVC
2. npm install
3. 放图标 → npm run tauri icon
4. npm run dev            ← 看到窗口就成功
5. npm run build          ← 出安装包
6. 装 0.1.0 → 升版本 → 再 build → make:latest → serve:updates
7. 打开已装的 0.1.0  → 应提示更新
```

第 4 步能通，后面都是配置问题；第 4 步不通，通常是 Rust 或 MSVC 环境。
