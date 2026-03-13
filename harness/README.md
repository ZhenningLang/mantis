# Development Harness (For AI Coding Agent)

mantis 是一个终端 TUI 应用，用于浏览、搜索和管理 [Droid](https://docs.factory.ai) 的聊天 session。

用户可以在终端中快速查找历史 session、预览对话内容、恢复 / 删除 / 重命名 session。

## 技术栈

- Go 1.25
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — TUI 框架（Elm 架构：Model/Update/View）
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) — 终端样式
- [sahilm/fuzzy](https://github.com/sahilm/fuzzy) — 模糊搜索

## 数据来源

读取 `~/.factory/sessions/` 目录：
- 子目录名编码了项目路径（如 `-Users-jane-Projects-myapp`）
- 每个 session 由 `.jsonl`（对话记录）和 `.settings.json`（元数据）组成
- 根目录下的 `.jsonl` 文件属于 "global" 项目

## 核心功能

| 功能 | 快捷键 | 说明 |
|------|--------|------|
| 模糊搜索 | 直接输入 | 搜索 title + project + 首条用户消息 |
| 恢复 session | Enter | 退出后 exec `droid -r <id>` |
| 删除 | Ctrl+D | 单条删除（需确认） |
| 批量删除 | Ctrl+X | 进入选择模式，Tab 标记，d 确认 |
| 重命名 | Ctrl+R | 修改 session title |
| 统计面板 | Ctrl+S | 按项目分组统计 |
| 路径切换 | Tab | 在项目短名和完整路径间切换 |

## 构建与运行

```bash
go build -o mantis .
./mantis
```

## 参考文档

- 架构：[architecture.md](./architecture.md)
- 代码规范：[guidelines.md](./guidelines.md)
