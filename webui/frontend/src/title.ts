function unwrap(value: string): string {
  const trimmed = value.trim().replace(/[.;。；]+$/, "");
  const pairs: Record<string, string> = { "`": "`", '"': '"', "'": "'", "“": "”", "‘": "’" };
  const end = pairs[trimmed[0]];
  return end && trimmed.endsWith(end) ? trimmed.slice(1, -1).trim() : trimmed;
}

// conciseTitle removes instruction boilerplate that otherwise makes dozens of
// session rows begin with the same words. It is deliberately deterministic: the
// durable journal title remains untouched and the full source stays in the
// row tooltip.
export function conciseTitle(raw: string): string {
  let value = (raw || "").replace(/\s+/g, " ").trim();
  if (!value) return value;

  const reply = /^(?:please\s+)?reply\s+with\s+(?:exactly|just)\s*[:：]?\s*/i;
  if (reply.test(value)) value = `Reply · ${unwrap(value.replace(reply, ""))}`;
  else {
    const shell = /^(?:please\s+)?use\s+(?:(?:the\s+)?(?:bash|shell)(?:\s+tool)?)\s+to\s+(?:run|execute)\s+(?:exactly\s*)?[:：]?\s*/i;
    const zhShell = /^用\s*(?:bash|shell)\s*(?:工具)?(?:执行|运行)(?:这条命令)?(?:[，,](?:原样|不加参数)[^:：]*)?\s*[:：]\s*/i;
    if (shell.test(value)) value = unwrap(value.replace(shell, ""));
    else if (zhShell.test(value)) value = unwrap(value.replace(zhShell, ""));
  }

  if (value.length <= 92) return value;
  const window = value.slice(0, 92);
  const breaks = [window.lastIndexOf(". "), window.lastIndexOf("。"), window.lastIndexOf("；"), window.lastIndexOf("; ")];
  const at = Math.max(...breaks);
  return (at >= 36 ? window.slice(0, at + 1) : value.slice(0, 89).trimEnd()) + "…";
}

// displayTitle resolves the label shown for a session: a user rename wins over
// the concise auto-derived title, which wins over the raw id. Renames are a
// local preference (localStorage), mirroring pinned/archived.
export function displayTitle(
  renames: Record<string, string>,
  sid: string,
  rawTitle?: string,
): string {
  const custom = renames[sid];
  if (custom && custom.trim()) return custom.trim();
  return rawTitle ? conciseTitle(rawTitle) : titleFromSessionId(sid);
}

// Old/child sessions can be deep-linked before (or without) a list metadata
// row. Keep the header human-readable instead of leaking the full durable id.
export function titleFromSessionId(sid: string): string {
  const withoutStamp = sid.replace(/^\d{8}-\d{6}-/, "");
  const child = withoutStamp.match(/-sub-(call_\d+(?:_\d+)*)-[a-z0-9]+$/i);
  if (child) return `Sub-agent · ${child[1].replace(/_/g, " ")}`;
  const withoutSuffix = withoutStamp.replace(/-(?:[a-f0-9]{4}|[a-f0-9]{16})$/i, "");
  const label = withoutSuffix
    .replace(/-sub-[^-]+-\d+(?:_\d+)*-/i, " · ")
    .replace(/[-_]+/g, " ")
    .replace(/\s+/g, " ")
    .trim();
  return conciseTitle(label || "Session");
}
