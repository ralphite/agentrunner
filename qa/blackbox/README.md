# qa/blackbox — 黑盒 UI QA

真浏览器(playwright-core + 系统 Chrome/Chromium)驱动真 webui,像用户
一样走主 journey。每步全局断言:

- 无 uncaught page error / console.error(白名单滤噪)
- 无横向溢出(scrollWidth ≤ innerWidth+1)
- DOM 里无原始内部错误文案(exit status N / fatal: / daemon dial: /
  绝对路径类——"吓人红 toast"类缺陷的机器判据)
- 每步截图 → artifact

Journey:home(手机+桌面双上下文)、Scheduled 坏 workspace 输入(断言
友好文案而非原始路径)、真 turn(创建任务→首答→追问)、Changes、
daemon-down 友好化(设 DAEMON_KILL_CMD 时,最后跑)。

本地(无 API key):
    cd qa/blackbox && npm install
    SKIP_TURNS=1 CHROME_PATH=... node drive.mjs http://127.0.0.1:8788 out

CI:Actions → qa-blackbox(真 Gemini,repo secrets)。
退出码 = findings 数;findings.json + 截图上传 artifact。
