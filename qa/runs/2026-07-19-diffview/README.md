# QA 2026-07-19 · diff view 全线不可用(用户手机截图报障)

本地 Chromium(Playwright,390×844 与 1280×800/1920×1080)+ scripted
provider 会话复现;远程 env 槽位被并发 session 占用(run 29703154416),
故本轮取证在本地真浏览器完成,远程红转绿另补。

## RED(修复前)
- `red-phone-squashed.png`:.diffwrap 无 overflow-y、外层 overflow-hidden
  定高 → 文件卡被 flex-shrink 压扁成"横条"(190px 内容 → 15px/8px,
  bigmodule 7711px → 628px),整个 review 不可竖向滚动。
  量化:diffwrap {clientH:818, scrollH:818, overflowY:visible, canScroll:false}。

## GREEN(修复后)
- `green-phone-scroll.png`:diffwrap {scrollH:9252, overflowY:auto,
  canScroll:true},0 张压扁卡。
- `green-phone-bottom.png`:滚到底,最后一张卡完整可达,diffbar sticky 在位。
- `green-phone-wrapped.png`:Wrap long lines 修好后无横向溢出
  (此前 .dl min-w-max 令 wrap 形同虚设)。
- `green-desktop-split-dark.png`:split 视图列轨共享(.fd-split 单 grid +
  行盒 display:contents),修改型文件两栏对齐;暗色主题正常。

驱动脚本(确定性管道)与量化输出见本目录 step*.mjs / metrics.json。
