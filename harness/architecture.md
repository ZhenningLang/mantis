# 架构文档

## 项目结构

```
main.go                       — 入口：CLI 分发 + 加载 session → 启动 TUI → 退出后 exec droid -r
internal/
  config/
    config.go                 — 配置加载/保存/交互式引导（~/.mantis/config.yaml）
  session/
    types.go                  — 数据模型（Session, SessionMeta, Settings, TokenUsage, Message）
    loader.go                 — 从 ~/.factory/sessions/ 加载所有 session
  summary/
    summary.go                — 摘要数据类型 + 读写（~/.mantis/summaries/）
    llm.go                    — OpenAI-compatible API 调用
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

### 项目名解析

优先级：session 元数据 cwd > 文件系统探测 > naive 替换

1. **cwd 元数据**（首选）：`.jsonl` 首行的 `cwd` 字段即为真实工作目录，直接用作 `ProjectFull`，`filepath.Base` 取短名
2. **文件系统探测**（兜底）：目录名如 `-Users-jane-Projects-my-app`，`-` 既是路径分隔符也可能是目录名的一部分。`probePath` 从左到右逐段尝试拼接，通过 `os.Stat` 验证路径是否存在
3. **naive 替换**（最终兜底）：目录在本机不存在时，所有 `-` 替换为 `/`
4. 根目录直接存放的 .jsonl → Project 为空，显示为 `"global"`

## 依赖关系

```
main.go
  ├── internal/session   (数据加载，无外部状态依赖)
  └── internal/tui       (UI 层)
        └── internal/action  (操作逻辑，直接操作文件系统)
```

各包职责清晰，session 只负责读取，action 只负责写入，tui 负责交互。
