# QA-90：INC-100 shared-store Web UI Gate B

- URL：`http://127.0.0.1:8809/`
- production：`cadd08e7-215130`
- health：`daemonUp=true`、`versionMatch=true`
- store：`~/.local/share/agentrunner/`
- 浏览器 console：0 warning / 0 error
- deploy：仅切换 `ar-live`/`arwebui-live` 并重启 Web UI；未重启 daemon。
- 既有定时 session
  `20260723-043024-qa88-schedule-restart-6d2120bc409214e4`
  重启前后均为 `running`。

## text + attachment

- session：
  `20260724-045524-read-the-attached-file-and-rep-9c8e4e58d33a7be3`
- `input_received`：恰好 1 次，`seq=3`，`delivery_seq=1`
- opening text：
  `Read the attached file and reply exactly INC100-TEXT-ATTACH-OK. Do not use tools.`
- opening content：同 event 含 1 个 `file` part 与 files projection
- reply：`INC100-TEXT-ATTACH-OK`
- workspace diff：`No changes since the latest human turn began.`
- 原始 journal：`text-and-attachment-events.jsonl`
- 截图：`text-and-attachment-webui.png`

## attachment only

- session：
  `20260724-045551-please-review-the-attached-fil-7856f1cf1e40e1e6`
- `input_received`：恰好 1 次，`seq=3`，`delivery_seq=1`
- opening text：`Please review the attached file(s).`
- opening content：同 event 含 1 个 `file` part 与 files projection
- 无第二次 `input_received` / send
- 中性 caption 使 agent 继续探索其独立 worktree；为避免无价值消耗，QA 只
  `interrupt` 该 turn。session、journal、worktree 均保留。
- workspace diff：`No changes since the latest human turn began.`
- 原始 journal：`attachment-only-events.jsonl`
- 截图：`attachment-only-webui.png`

两条证据共同证明：Home 的 text+attachment 与 attachment-only 都由一次 create
形成唯一 opening turn，附件不是创建后的第二条消息。
