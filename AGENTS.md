# AGENTS.md — 编码规则（AI 每次动手前必读）

本项目是「智能体平台」，前端 TypeScript（React），后端 Go。
本文件是硬约束，违反即为错误实现。

---

## 一、技术栈锁定（不得替换）

| 层 | 技术 | 备注 |
|---|---|---|
| 前端框架 | React 18 + TypeScript + Vite | 不允许引入 Vue / Svelte / Angular；升级 React 19 需单独立项 |
| 桌面壳 | **Tauri 2** | 业务逻辑全部在 TypeScript，重活放 sidecar（Go），尽量不写 Rust |
| 移动壳 | **Capacitor** | 手机/平板复用同一份母版；不允许改成 React Native（除非走 docs/04 第 10 节的触发条件） |
| 官网 | **Astro** | 独立工程，与产品应用分离 |
| 路由 | **HashRouter** | Tauri 用自定义协议，无 history fallback，用 BrowserRouter 会白屏 |
| 样式 | 原生 CSS + CSS 变量 | 断点：<760px 手机 / 760–1100px 平板 / >1100px 桌面 |
| 状态 | Zustand（客户端状态）+ TanStack Query（服务端数据） | 不允许引入 Redux / MobX |
| 后端语言 | Go | 不允许引入 Node/Python 写业务接口 |
| 后端框架 | Gin | 不允许换成 Fiber / Echo（保持单一选择） |
| 数据库 | PostgreSQL 18 | 不允许引入 MongoDB / MySQL |
| 数据访问 | pgx v5 + 手写 SQL | 不允许引入 GORM 等全功能 ORM |
| 缓存/队列 | Redis + asynq | 第一版可只接 Redis |
| 接口契约 | OpenAPI 3.1 | **先改 yaml，再改实现** |

新增任何依赖前，必须先说明理由并得到确认。

---

## 二、Go 后端分层规则（强制）

```
handler  →  service  →  repository
```

| 层 | 允许做 | 禁止做 |
|---|---|---|
| handler | 解析参数、调用 service、写响应 | 直接访问数据库；写业务判断 |
| service | 业务逻辑、事务、调用外部服务 | 直接读 `http.Request`；直接写 SQL |
| repository | 执行 SQL、组装实体 | 业务判断；调用外部 HTTP |

- **禁止跨层调用**（handler 不得直接调 repository）
- **事务只在 service 层开启**
- 每个请求的 `context.Context` 必须一路向下传，禁止 `context.Background()` 中途替换

### 目录结构

```
server/
├── cmd/api/main.go            程序入口
├── internal/
│   ├── config/                配置加载
│   ├── db/                    连接池
│   ├── httpapi/               路由、中间件、统一响应
│   ├── auth/                  JWT 签发与校验
│   └── <模块>/                每个业务模块一个包
│       ├── model.go           实体与 DTO
│       ├── repository.go      SQL
│       ├── service.go         业务逻辑
│       └── handler.go         HTTP 处理
└── migrations/                版本化 SQL 迁移
```

```
api/openapi.yaml             接口契约（唯一真相，前后端共用，放仓库根目录）
```

---

## 三、接口与响应规范（强制）

### 统一响应体

```json
{ "code": 0, "message": "ok", "data": {}, "traceId": "..." }
```

- 成功：`code = 0`
- 失败：`code != 0`，HTTP 状态码同时正确设置
- **永远不要裸返回数组**

### 错误码分段

| 区间 | 含义 |
|---|---|
| `0` | 成功 |
| `10000–19999` | 通用（参数错误、未登录、无权限、限流） |
| `20000–29999` | 用户与认证 |
| `30000–39999` | 目标 / 计划 / 任务 |
| `40000–49999` | 对话 / 模型调用 |
| `50000–59999` | 知识库 |
| `60000–69999` | 计费与配额 |

### 命名与格式

- JSON 字段：`camelCase`
- 数据库字段：`snake_case`
- 时间：ISO 8601 + UTC（`2026-11-15T07:30:00Z`），数据库用 `timestamptz`
- 金额：**整数分（BIGINT）**，禁止 float
- ID：表内 `BIGINT` 自增主键；对外返回 `uuid`（`uuidv7()`）

---

## 四、安全红线（违反即为严重缺陷）

1. **禁止拼接 SQL**。只用参数化查询（`$1, $2`）或 `sqlc` 生成的代码
2. **禁止在日志打印**手机号、身份证、密码、Token、私域文档正文
3. **每个查询必须带 `user_id` 条件**（防水平越权），除非明确是管理接口
4. **密码**用 `argon2id` 或 `bcrypt`，禁止 MD5 / SHA1
5. **密钥**只从环境变量读，禁止写进代码、Git、数据库
6. **客户端一律不可信**：桌面端 / 移动端里的校验只是体验，服务端必须重做
7. **私域文档**：不提供批量导出接口；每次读取写 `audit_logs`
8. 软删除的表，查询必须带 `deleted_at IS NULL`

---

## 五、外部服务调用规则

**所有模型调用必须经过 `internal/modelgw`（模型网关）。**

- 业务代码禁止直接调某家厂商的 SDK 或 HTTP
- 网关负责：多供应商路由、超时、重试、降级、计费记录
- 每次调用必须写一条 `usage_records`

理由：换模型、控成本、做降级时，只需要改网关一处。

---

## 六、前端规则

- 只通过 `api/` 层的类型化客户端调接口，**禁止组件里裸写 `fetch`**
- 接口类型由 `api/openapi.yaml` 生成，**禁止手写重复类型**
- 所有页面必须处理四种状态：加载中 / 空 / 错误 / 正常
- 不允许 `any`；确实无法确定时用 `unknown` 并显式收窄
- 组件文件用 PascalCase，工具函数用 camelCase
- 只有 Windows 端可用的功能，必须用 `capabilities` 判断后再渲染，不能只靠隐藏按钮

### 平台能力抽象（强制）

- **禁止在组件里直接 `import '@tauri-apps/api'` 或 `@capacitor/*`**
- 所有壳能力必须走 `src/platform` 导出的 `bridge`
  ```
  platform/
  ├── types.ts      DesktopBridge 接口（与壳无关）
  ├── tauri.ts      Tauri 实现（全部用动态 import）
  ├── capacitor.ts  Capacitor 实现（全部用动态 import）
  ├── web.ts        浏览器降级实现
  └── index.ts      探测 tauri / capacitor / web 后导出单例
  ```
- 新增一个壳能力时，顺序是：先加接口 → 再补全部实现 → 最后在组件里用
- 这样同一套页面能同时在桌面、手机、浏览器里跑，换壳零改动

### 路由

- 必须用 `HashRouter`，**禁止 `BrowserRouter`**（Tauri 无 history fallback，刷新即白屏）

### 存储

- **Token 禁止放 localStorage**，一律通过 `bridge.secureStorage` 读写
- 业务数据缓存放 TanStack Query，不放 Zustand

---

## 七、提交前必做

```bash
# 后端
cd server
go vet ./...
go test -race ./...
gofmt -l .          # 有输出说明没格式化

# 前端（以实际仓库为准，当前为 src/kaixin_desktop）
cd <client 目录>
npm run typecheck
npm run build
```

- **测试必须带 `-race`**，这是 Go 项目的硬性要求
- 改动了 `api/openapi.yaml` 必须重新生成两端代码
- 每完成一个可运行的改动就提交一次，提交信息写清楚做了什么

---

## 八、禁止事项（高频踩坑）

| 禁止 | 原因 |
|---|---|
| 一次生成整个功能模块 | 出问题时无法定位，改为小步提交 |
| 在 handler 里写 SQL | 破坏分层，后期无法测试 |
| 用 `interface{}` / `any` 逃避类型 | 等于放弃 TypeScript 和 Go 的最大优势 |
| 引入"以后可能用到"的抽象 | YAGNI，第二次重复时才抽象 |
| 直接调用模型厂商 API | 绕过网关会导致成本失控 |
| 在客户端存密钥 | 桌面端/移动端可被逆向 |
| 在组件里直接调 Tauri API | 破坏平台抽象，Web 端直接跑不起来 |
| 用 BrowserRouter | Tauri 里刷新白屏 |
| 按 `esnext` 打包前端 | WebView2 版本不一，可能在旧机器白屏 |
| 自行实现实时音视频 | 一律用云服务 |
| 修改已有迁移文件 | 只能新增迁移，不能改历史 |
| 直接编辑 `.docx` | 会与 Markdown 源文件脱节，下次生成即被覆盖 |
| 同一信息写在两份文档 | 必然有一份先过期（路线图已踩过这个坑） |

---

## 九、文档维护规则（强制）

### 9.1 文档职责表

| 文档 | 只管什么 | 不写什么 |
|---|---|---|
| `docs/00-方案体检与技术架构.md` | 能力清单、可行性分级、成本、风险 | 不写排期（指向 04） |
| `docs/02-模型清单与接入方案.md` | 模型库、价格、选型策略 | 不写架构 |
| `docs/03-桌面端技术方案-Tauri.md` | Tauri 配置、sidecar、打包发布 | 不写页面实现 |
| `docs/04-全端技术方案定稿.md` | **决策、端矩阵、路线图（唯一权威）** | 不写模型价格 |
| `docs/05-多模型辩论模块方案.md` | 该模块的完整设计 | 不重复全局架构 |
| `docs/06-模型与软件部署归属表.md` | 每个组件装在哪端、为什么 | 不写业务功能设计 |
| `docs/07-技能部署功能方案.md` | 硬件检测 / 软件匹配 / 方案生成 / 安装分级 | 不写全局架构 |
| `docs/08-数据库设计与管理.md` | 双库架构、调参、连接池、迁移、备份、保留 | 不写表结构细节（见 migrations） |
| `docs/09-技能实施规范.md` | 六个能力原语、技能包结构、接入流程、上线验收 | 不写具体技能的业务逻辑 |
| `docs/10-网站对接与授权方案.md` | 页面地图、授权形态、取数三级优先、具体站点落地 | 不写通用原语规则（见 09） |
| `docs/11-红线分级与纠偏.md` | 七条红线、L0–L3、四个检查点、四维偏差、纠偏状态机 | 不写需求清单（见需求书） |
| `docs/12-成本计费与降级.md` | 成本口径、三档方案、测算模板、计费正确性、九类降级 | 不写模型价格（见 02） |
| `docs/13-验收差距与里程碑.md` | 验收指标、一票否决、测试方法、差距 G 清单、M0–M4 | **不写路线图**（见 04） |
| `README.md` | 怎么跑起来 | 不写方案论证 |
| `AGENTS.md` | 编码与文档规则 | 不写业务设计 |

### 9.2 单一真相原则

同一信息只能有**一个权威位置**。其他文档需要时写「见 `docs/0X`」引用，不复制。

> 反例：路线图曾同时写在 `docs/00` 和 `docs/04`，两套阶段名不同、内容冲突。
> 后来删掉 `docs/00` 那份才消除隐患。

### 9.3 改动连带清单（改之前先看这张表）

| 你改了什么 | 必须同步 |
|---|---|
| 能力判定 ✅ / ⚠️ / ❌ | `docs/00` 底部的统计数字（19 项里可做 X / 降级 Y / 放弃 Z） |
| 路线图阶段 | **只在 `docs/04` 改**，其他地方引用，不另写一份 |
| 技术选型（语言 / 框架 / 壳） | grep 旧技术名，见 9.5 |
| 文档版本号 | ① 头部「版本 vX」② 变更记录加一行 ③ Word 封面版本 ④ `build_plan_docx.py` 的 `REVISIONS` |
| 新增一个模块 | 新建 `docs/0X` + 在 `docs/04` 路线图加阶段 + 加进 `CHAPTERS` 列表 |

### 9.4 改完必须重新生成 Word

**禁止直接编辑 `.docx`。** 正确流程：

```bash
python build_plan_docx.py
python verify_docx.py "docs/开心同学AI助手-技术方案书.docx"
python check_docx_text.py "docs/开心同学AI助手-技术方案书.docx"
```

三条都要过：

- `build` 无报错
- `verify` 的章节数 / 表格数符合预期，`空表: 0`
- `check` 除代码块外无 `**`、反引号、`](` 等 Markdown 残留

### 9.5 提交前自检（三条 grep）

```bash
grep -rn "Electron\|Taro\|小程序\|四端" docs/
grep -rn "动漫\|IP 开会" docs/
grep -rn "v1\.[0-9]" docs/*.md
```

**剩余命中必须全部落在「历史说明」语境里**（变更记录、迁移对照表、为什么砍掉），否则说明没改干净。

### 9.6 检查清单（每次改文档照着走）

```
1. 改判定 → 同步统计数字
2. 改路线 → 只改 docs/04，别处引用
3. 换技术 → grep 旧技术名，逐一处理命中
4. 改版本 → 四处同步（头部 / 变更记录 / Word 封面 / REVISIONS）
5. 改完   → 重跑 build → verify → check
```
