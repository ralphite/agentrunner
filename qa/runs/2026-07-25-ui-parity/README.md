# UI parity visual evidence

每个可验收 UI 批次只保留一份简短视觉证据：改前/改后截图、受影响文件、
用户可见结果和对应提交/CI。这里不是 review ledger，不记录逐 story 的重复
输出。

| Batch | 状态 | Evidence |
| --- | --- | --- |
| Sidebar current-work hierarchy | current | [before / after / files](sidebar-current-work/README.md) |
| Quiet default session chrome | shipped | [`bbb04316`](https://github.com/ralphite/agentrunner/commit/bbb043168dbb8af8f34f10c8411be04c26f42c2f) |
| Session action feedback | shipped | [`24f420a5`](https://github.com/ralphite/agentrunner/commit/24f420a5a0d29e40d8d1efa2774cd88ea622c6e1) |
| Environment reachability | shipped | [`43d3bb52`](https://github.com/ralphite/agentrunner/commit/43d3bb529a89074d796709c8cb2a4afcbfebc37d) |
| Mobile composer choices | shipped | [`0c484d21`](https://github.com/ralphite/agentrunner/commit/0c484d21) |

旧批次的截图会从已有 `qa/runs/` 逐批补链；从本批开始截图是提交前的硬证据。
