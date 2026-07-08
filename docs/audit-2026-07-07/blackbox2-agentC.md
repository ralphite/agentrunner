# 黑盒2 报告 C:给 AI 看图片

结局:成功且 AI 真看见了(逐行复述 6 行含没法猜的 `command.go:1234:15: undefined: EnableTraverseRunHooks2`)。但普通人极可能中途放弃:功能藏得深、路径重、参数顺序坑。

## 问题
### [R2-C-1] 只有 send 能看图,run/new 都不行——普通人第一条路就是死路 🔴
`ar run spec.yaml "看这图" --image x.png` → `flag provided but not defined: -image`;`ar new … --image` 同样报错。`--image` 仅存在于 send。想给 AI 看张图被迫 daemon→new→send 三步 + 常驻进程,重到劝退。

### [R2-C-2] "能传图"几乎无从发现:README 只字不提,help 里只有一句括号 🔴
全产品唯一图片线索是 `ar help` 里 `send … (--image attaches files)` 半句括号,措辞是泛泛 "files" 不是 "image/screenshot"。`grep -i image README.md`=0。普通人搜"图/screenshot"全落空,极可能断定"不支持"就走人。

### [R2-C-3] --image 参数顺序坑:flag 放 session id 后面→光秃秃 usage,不说是顺序错 🟠
`ar send <sid> "msg" --image x.png` → `usage: agentrunner send [flags] <sid> "message"` exit 2 零解释(Go flag 在第一个位置参数处停止)。人类自然语序失败,一度以为 session id 打错。只有 `ar send --image x.png <sid> "msg"`(flag 提前)才行。

### [R2-C-4] run --image(flag 在位置参数后)只回 usage,不点破 run 不支持图 🟡
同命令 flag 在前在后给两种报错(usage vs "not defined"),摸不着头脑是"顺序错"还是"没这功能"。

## 正面(表扬)
- 错路径报错 `open …: no such file or directory`、非图文件 `not an image (detected text/plain; charset=utf-8)`——说人话、不泄漏内部。
- `-image` 标注 "(repeatable)" 多图可传。
- **图 AI 真用视觉看了**:逐行 verbatim 6 行 + 答对包名/Go 版本/末行,`1234:15` 和 `EnableTraverseRunHooks2` 绝无可能蒙——核心能力扎实可信。

## 结局
能力满分(真视觉+报错诚实),但"发现→上手→参数顺序"埋了两颗🔴一颗🟠。普通人大概率卡在"根本不知道能传图"。
