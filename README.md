# 开心同学 AI 助手

一个面向普通人的本地 AI 智能体桌面应用：在 Windows 桌面上用自然语言描述需求，
由 AI 拆解任务、挑选技能、准备软件与网站，最终在本机把环境装好。

当前状态：桌面端可用，本地模型（Ollama）与云端 OpenAI 兼容接口均可接入。

## 仓库结构

```
src/
├── kaixin_desktop/   桌面端（Tauri 2 + React 18 + TypeScript）
├── kaixin_local_go/  本地服务 sidecar（Go + SQLite，监听 127.0.0.1）
├── kaixin_server_go/ 云端服务（Go + PostgreSQL）
├── kaixin_web/       网页端（同一套页面，可跑在浏览器）
├── kaixin_mobile/    手机端（Capacitor，复用网页端）
├── kaixin_admin/     管理后台（React）
└── kaixin_site/      官网（Astro，纯静态）
docs/                 技术方案、功能基线与验收清单
```

## 本地跑起来

### 1. 本地服务（Go）

```bash
cd src/kaixin_local_go
go run .
# 默认监听 http://127.0.0.1:5085
```

### 2. 桌面端

```bash
cd src/kaixin_desktop
npm install
npm run dev          # 浏览器调试
npm run tauri dev    # 桌面窗口
```

打包前需要把本地服务编译成 sidecar 放到 `src-tauri/binaries/`：

```bash
cd src/kaixin_local_go
go build -o ../kaixin_desktop/src-tauri/binaries/kaixin-local-x86_64-pc-windows-msvc.exe .
```

### 3. 网页端 / 管理后台

```bash
cd src/kaixin_web && npm install && npm run dev
cd src/kaixin_admin && npm install && npm run dev
```

## 模型接入

进入「系统设置 → 模型接入」：

| 类型 | 地址 | 说明 |
|---|---|---|
| 本机 Ollama | `http://127.0.0.1:11434/v1` | 无需密钥，免费 |
| DeepSeek | `https://api.deepseek.com/v1` | 需要 API Key |
| 通义千问 | `https://dashscope.aliyuncs.com/compatible-mode/v1` | 需要 API Key |

API Key 只保存在本机数据库，不会下发到页面。

## 文档

- [技术方案总览](docs/04-全端技术方案定稿.md)
- [功能需求基线与差距](docs/13-功能需求基线与差距.md)
- [技能部署功能方案](docs/07-技能部署功能方案.md)
- [数据库设计与管理](docs/08-数据库设计与管理.md)
- [人工验收清单](docs/16-人工验收清单.md)

编码与文档规则见 [AGENTS.md](AGENTS.md)。

## 许可

私有项目，未经授权请勿分发。
