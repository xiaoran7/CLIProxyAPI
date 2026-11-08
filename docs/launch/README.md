# 开源宣发物料索引

本目录存放 CLI Proxy API 对外发布、冷启动宣发与社区投递所需的成套文案与模板，
按 [open-setup](../../AGENTS.md) 规范集中管理，不与技术文档混排在 `docs/` 根下。

| 文件 | 用途 | 投放目标 |
|---|---|---|
| [copywriting.md](copywriting.md) | 首发文案包（标题 / 正文 / 标签） | V2EX `/go/share`、`/go/create`，Hacker News Show HN，Reddit r/LocalLLaMA |
| [discussions-qa.md](discussions-qa.md) | GitHub Discussions 种子问答库 | 仓库 Discussions，供首次访客自助排查常见问题 |
| [community-submissions.md](community-submissions.md) | 社区清单与 Awesome PR 模板 | `awesome-claude-code`、`awesome-ai-agents` 等清单仓库 |

## 使用约定

1. **技术事实以 `CHANGELOG.md` 与代码为准**，本目录文案只负责表达，不得出现代码中不存在的功能宣称。
2. 发新版本时先更新 `CHANGELOG.md`，再回头校对上述文案中的版本号、指标数字与功能列表。
3. 文案中的性能数字（TTFT、并发、超时阈值等）必须与同批提交的实测记录一致，严禁沿用上版的旧数字。
4. 上游仓库地址与负责模块的对应关系见 [README.md](../../README.md) 的「上游溯源」一节。
