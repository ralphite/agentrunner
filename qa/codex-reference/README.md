# Codex 金标截图参照 (golden reference)

把 **Codex 桌面 app** (`/Applications/ChatGPT.app`, bundle `com.openai.codex`)
各屏的截图放这里,文件名按屏命名,例如:

    home-light-1440.png  home-dark-1440.png
    session-rich-light.png  approval-light.png  settings-appearance.png

`/parity-drive` 每轮的第一步会**优先**用这里的金标图对 live `127.0.0.1:8809`
做像素级并排比对;此目录为空时回退到文字参照
`docs/increments/INC-41-CODEX-UI-REFERENCE.md` + `docs/CODEX-PARITY.md`。

⚠️ headless 循环**自己截不到 Codex app**(无 Computer Use / 无录屏权限)。
这些金标图需由能跑 Computer Use 的**交互 session** 或**真人**捕获后放入。
截图**可以**入库(这是 ground truth 参照,不是临时测试产物)。
