import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const root = new URL(".", import.meta.url);

// QA-0719 · diff review 滚动几何契约。
//
// 真机实测(用户手机 390×844 与本地 Chromium 390×844/1280×800 双复现):
// `.diffwrap` 是 flex 列但没有 overflow-y,而它住在 overflow-hidden 的
// 定高 `.changes-panel` 里——内容装不下时浏览器不给滚动条,而是按
// flex-shrink 把全部文件卡压扁(`.filediff` 的 overflow-hidden 让 flex
// 自动最小尺寸归零):collapsed 卡压成 3-15px 的"横条"、打开的大文件
// 7711px 内容只剩一屏且不可滚——整个 review 不可用。
//
// 契约:`.diffwrap` 必须自己是竖向滚动容器(它的 `.diffbar` sticky top-0
// 一直假定这一点),文件卡必须拒绝参与 flex 收缩。jsdom 不做真布局,
// 所以钉在样式源文本上——iosViewport.test.js 同法。
describe("diff review scroll geometry contract", () => {
  const css = readFileSync(new URL("tw.css", root), "utf8");

  it(".diffwrap is the review's vertical scroll container", () => {
    const rule = css.match(/\.diffwrap\s*\{[^}]*\}/)?.[0];
    expect(rule).toBeTruthy();
    expect(rule).toContain("overflow-y-auto");
  });

  it(".filediff cards never flex-shrink into stripes", () => {
    const rule = css.match(/\.filediff\s*\{[^}]*\}/)?.[0];
    expect(rule).toBeTruthy();
    expect(rule).toContain("shrink-0");
  });

  // Wrap 开关曾经无效:`.dl { min-w-max }` 的 max-content 不看软换行
  // 机会,行照旧按完整行宽撑开。wrap 模式必须撤销行容器的 max-content
  // 最小宽,代码列(minmax(0,1fr))才可能真正换行。
  it("wrap mode releases the rows' max-content min-width", () => {
    const rule = css.match(/\.diff-wrap \.dl,[^{]*\{[^}]*\}/)?.[0];
    expect(rule).toBeTruthy();
    expect(rule).toContain("min-w-0");
  });

  // split 视图曾经每行各开一个 min-w-max grid:列宽按本行内容独立解析,
  // 右列起始 x 逐行漂移(短行 344px、长行 946px),长行代码被推出视区,
  // 看起来像隔行消失。列轨必须由 .fd-split 统一持有,行盒退成
  // display:contents。
  it("split view shares one set of column tracks across all rows", () => {
    const grid = css.match(/\.fd-split\s*\{[^}]*\}/)?.[0];
    expect(grid).toBeTruthy();
    expect(grid).toContain("grid");
    expect(grid).toContain("max-content");
    const row = css.match(/\.fd-split \.dls\s*\{[^}]*\}/)?.[0];
    expect(row).toBeTruthy();
    expect(row).toContain("display: contents");
  });
});
