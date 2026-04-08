# 架构文档

## 项目结构

```
 main.go                       — 入口：CLI 分发（TUI / inspect / compress / fork / completion / config / index / status / clean）
internal/
  completion/
    completion.go            — 生成 bash / zsh / fish 子命令补全脚本
  compress/
    types.go                  — 压缩输入、handoff、Droid settings 类型
    resolve.go                — session 前缀解析、Droid settings 解析、compress 主流程
    extract.go                — 从 raw events 提取 anchors / task state / artifact trail
    llm.go                    — 调用 source model 生成结构化 handoff JSON
    session.go                — 渲染 handoff、新建 session 文件
  config/
    config.go                 — 配置加载/保存/交互式引导（~/.mantis/config.yaml）
  llmstream/
    stream.go                 — 默认流式 OpenAI-compatible SSE 客户端（chat/responses）
  session/
    types.go                  — 数据模型（Session, SessionMeta, Settings, TokenUsage, Message, RawEvent）
    loader.go                 — 从 ~/.factory/sessions/ 加载 session / 解析原始事件
  inspect/
    types.go                  — inspect 结构化分析结果模型
    select.go                 — 挑选代表性 session
    static.go                 — 静态分析上下文分布 / 工具体积 / cache / prompt 片段
    agent.go                  — 调用 LLM 生成中文诊断
    report.go                 — 终端报告渲染
    run.go                    — inspect 主流程 + 报告保存
  summary/
    summary.go                — 摘要数据类型 + 读写（~/.mantis/summaries/）
    llm.go                    — 通过默认流式 Chat Completions 生成摘要
    manager.go                — 批量生成 + 进度管理
  action/
    action.go                 — 操作逻辑（Delete, Rename）
  status/
    status.go                 — CLI status 命令（摘要状态 + 统计信息）
  tui/
    app.go                    — 主 Model + Init/Update/View（Bubble Tea 框架）
    list.go                   — 列表渲染 + 模糊搜索
    preview.go                — 预览面板渲染
    helpers.go                — 通用工具函数（格式化、文本提取）
    styles.go                 — 所有 lipgloss 样式定义
    stats.go                  — 统计面板（按项目分组）
harness/                      — AI agent 参考文档（本目录）
```

## 架构模式

采用 Bubble Tea 的 Elm 架构（单向数据流）：

```
User Input → Update(msg) → Model 状态变更 → View() → 渲染输出
```

### Model（app.go）

核心状态：

| 字段 | 类型 | 说明 |
|------|------|------|
| sessions | []Session | 全量 session 列表（按时间倒序） |
| filtered | []int | 当前过滤后的索引数组（指向 sessions） |
| cursor | int | 当前选中行在 filtered 中的位置 |
| search | textinput.Model | 搜索框组件 |
| mode | viewMode | 当前视图模式 |
| fullPath | bool | 项目名显示模式（短名 / 完整路径） |

### 视图模式

```go
viewList          // 主列表（默认）
viewStats         // 统计面板
viewConfirmDelete // 删除确认
viewRename        // 重命名
viewBatchSelect   // 批量选择删除
```

### 布局

```
┌─────────────────────────────────────┐
│ mantis  [搜索框]  [x/y 计数]        │  header
│─────────────────────────────────────│
│ [project] sid  title      ago model │  list (2/3 高度)
│─────────────────────────────────────│
│ Preview: title, project, model...   │  preview (1/3 高度)
│─────────────────────────────────────│
│ 快捷键提示 / 状态信息               │  status bar
└─────────────────────────────────────┘
```

## 数据流

### 加载

```
main.go
  → session.LoadAll()
    → 遍历 ~/.factory/sessions/ 子目录
    → 每个 .jsonl 解析首行 meta + 前 20 条消息
    → 每个 .settings.json 解析 settings
    → 按 ModTime 倒序排列
  → tui.New(sessions)
    → 初始化 filtered = [0, 1, 2, ..., n-1]
```

### inspect 分析

```
main.go
  → config.Load()
  → inspect.Run(cfg)
    → session.LoadAll()
    → inspect.SelectSessions(all, 3)
    → inspect.Analyze(session)              // 读取 raw events 做静态分析
      → session.ParseAllEvents(path)
    → inspect.RunAgentAnalysis(cfg.LLM)     // LLM 生成诊断
      → llmstream.ChatCompletions()         // 默认消费 SSE delta，而不是等待最终 JSON body
    → inspect.PrintReport()
    → 保存到 ~/.mantis/reports/inspect-*.txt
```

### compress 压缩

```
main.go
  → compress.Run(args)
    → session.LoadAll()
    → ResolveSourceByPrefix()
    → session.ParseAllEvents(path)
    → BuildCompressionInput()
      → 找到最近一次 compressed handoff 作为 anchor
      → 按 token budget 切分 summarized turns / preserved turns
      → 提取 active skills / artifacts / todo / errors
      → 把较老消息按 user-turn 聚合成 compacted history phases
    → 读取 ~/.factory/settings.json 解析压缩 auth + model
    → GenerateHandoff()
      → 统一走默认流式 Chat Completions
      → compress 每 5 秒打印一次 handoff 生成心跳，避免长时间静默
      → 空输出/非法 JSON 直接失败
    → CreateCompressedSession()
      → 在最终 handoff 文本里追加 deterministic recent transcript（从 preserved turns 截取最近 visible turns，并过滤 Droid 内部 BYOK 错误提示）
  → exec droid -r <new-id>
```

### fork 分叉

```
main.go
  → runFork(args)
    → session.LoadAll()
    → compress.ResolveSourceByPrefix()
    → exec droid --fork <full-id>
```

### LLM 流式调用

`internal/llmstream` 统一处理 OpenAI-compatible SSE：

- 请求体默认注入 `stream: true`
- `chat/completions` 聚合 `choices[].delta.content`
- `responses` 聚合 `response.output_text.delta` / `response.output_text.done`
- 兼容本地 CLI Proxy 这类“流式有文本、非流式最终 JSON 为空”的 endpoint

### 搜索过滤

```
用户输入 → search.Value() 变化
  → refilter()
    → filterSessions(sessions, query)
      → fuzzy.FindFrom(query, source)  // source = title + project + firstUserMsg
    → 更新 filtered 索引数组
    → 修正 cursor 位置
```

### 操作

```
Enter   → 设置 resumeID → tea.Quit → main.go exec droid -r <id>
Ctrl+D  → 切换到 viewConfirmDelete → y: action.Delete() 删除文件
Ctrl+R  → 切换到 viewRename → enter: action.Rename() 修改 .jsonl 首行
Ctrl+X  → 切换到 viewBatchSelect → Tab 标记 → d: 批量 action.Delete()
```

## 数据模型

### Session

```go
type Session struct {
    Meta        SessionMeta  // ID, Title, WorkingDirectory
    Settings    Settings     // Model, AutonomyMode, TokenUsage, ActiveTime
    Project     string       // 短名（目录最后一段，如 "ordo_ai"）
    ProjectFull string       // 完整路径（如 "/Users/.../ordo_ai"）
    ModTime     time.Time
    FilePath    string       // .jsonl 文件路径
    Messages    []Message    // 前 20 条消息
    Selected    bool         // 批量选择标记
}
```

### RawEvent（inspect 使用）

`inspect` 不只依赖预览用的 `Message`，还会读取完整 `.jsonl` 原始事件流：

- `RawEvent`：保留 message / tool_result / metadata / callingSessionID
- `RawMessage`：role + 完整 content items
- `RawContentItem`：支持 `text` / `thinking` / `tool_use` / `tool_result`

这层数据使得 inspect 可以精确统计工具返回体积、thinking 开销、system reminder 注入和 subagent session。

### 项目名解析

优先级：session 元数据 cwd > 文件系统探测 > naive 替换

1. **cwd 元数据**（首选）：`.jsonl` 首行的 `cwd` 字段即为真实工作目录，直接用作 `ProjectFull`，`filepath.Base` 取短名
2. **文件系统探测**（兜底）：目录名如 `-Users-jane-Projects-my-app`，`-` 既是路径分隔符也可能是目录名的一部分。`probePath` 从左到右逐段尝试拼接，通过 `os.Stat` 验证路径是否存在
3. **naive 替换**（最终兜底）：目录在本机不存在时，所有 `-` 替换为 `/`
4. 根目录直接存放的 .jsonl → Project 为空，显示为 `"global"`

## 依赖关系

```
main.go
  ├── internal/config    (配置)
  ├── internal/session   (数据加载 / raw event 解析)
  ├── internal/inspect   (静态分析 + LLM 诊断 + 报告)
  ├── internal/summary   (摘要索引)
  ├── internal/status    (CLI 状态输出)
  └── internal/tui       (UI 层)
        └── internal/action  (操作逻辑，直接操作文件系统)
```

各包职责清晰：session 负责读取，inspect 负责分析，summary 负责索引，action 负责写入，tui 负责交互。
