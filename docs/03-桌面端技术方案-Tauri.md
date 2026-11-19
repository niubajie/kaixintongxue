# 桌面端技术方案（Tauri 2）

> 决策：**TypeScript 写业务，Tauri 2 当壳，Go sidecar 扛重活，NSIS 打包。**
> 版本 v1.1 ｜ 替代原 Electron 方案
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

### 发布流程

```
1. npm run tauri build
2. 产物在 src-tauri/target/release/bundle/nsis/
     AgentPlatform_0.1.0_x64-setup.exe
     AgentPlatform_0.1.0_x64-setup.exe.sig
3. 跑 npm run make:latest 生成 latest.json
4. 把 exe + sig + latest.json 上传到更新服务器
```

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
