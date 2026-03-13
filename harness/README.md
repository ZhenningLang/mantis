# Development Harness (For AI Coding Agent)

mantis 是一个终端 TUI 应用，用于浏览、搜索和管理 [Droid](https://docs.factory.ai) 的聊天 session。

用户可以在终端中快速查找历史 session、预览对话内容、恢复 / 删除 / 重命名 session。

## 技术栈

- Go 1.25
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — TUI 框架（Elm 架构：Model/Update/View）
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) — 终端样式
- [sahilm/fuzzy](https://github.com/sahilm/fuzzy) — 模糊搜索
- [gopkg.in/yaml.v3](https://github.com/go-yaml/yaml) — 配置解析

## 数据来源

- Sessions: `~/.factory/sessions/` — `.jsonl` + `.settings.json`
- 配置: `~/.mantis/config.yaml` — LLM 配置（可选）
- 摘要缓存: `~/.mantis/summaries/` — LLM 生成的 session 摘要

## 核心功能

| 功能 | 快捷键 | 说明 |
|------|--------|------|
| 模糊搜索 | 直接输入 | 搜索 title + project + 用户消息（有摘要时搜索摘要+关键词） |
| 项目筛选 | Ctrl+P | 按项目过滤 session 列表（支持搜索） |
| 恢复 session | Enter | 退出后 exec `droid -r <id>` |
| 删除 | Ctrl+D | 单条删除（需确认） |
| 批量删除 | Ctrl+X | 进入选择模式，Tab 标记，d 确认 |
| 重命名 | Ctrl+R | 修改 session title |
| 统计面板 | Ctrl+S | 按项目分组统计 |
| 路径切换 | Tab | 在项目短名和完整路径间切换 |

## Smart Search

配置 LLM 后（`mantis config`），启动时后台异步为未索引的 session 生成摘要：
- 策略性选取用户消息（前3条 + 后3条 + 中间采样，最多10条）
- LLM 生成 title + 多主题摘要 + 关键词
- 缓存为 sidecar 文件，只生成一次
- 搜索时匹配摘要和关键词，大幅提升查找精度

## CLI 子命令

```bash
mantis              # 启动 TUI
mantis config       # 配置 LLM（交互式）
mantis status       # 查看摘要索引状态和统计信息
mantis version      # 打印版本
```

## 构建与运行

```bash
go build -o mantis .
./mantis
```

## 参考文档

- 架构：[architecture.md](./architecture.md)
- 代码规范：[guidelines.md](./guidelines.md)
