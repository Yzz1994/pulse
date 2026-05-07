// 复制文本到剪贴板。
// 在 secure context（HTTPS / localhost）下使用 navigator.clipboard，
// 否则降级到 document.execCommand('copy')，保证 HTTP 远程访问也能用。
export async function copyText(text: string): Promise<boolean> {
  if (typeof navigator !== "undefined" && navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // 继续走降级
    }
  }
  if (typeof document === "undefined") return false;
  const ta = document.createElement("textarea");
  ta.value = text;
  ta.setAttribute("readonly", "");
  ta.style.cssText = "position:fixed;top:0;left:-9999px;opacity:0";
  document.body.appendChild(ta);
  const prevSelection = document.getSelection()?.rangeCount
    ? document.getSelection()!.getRangeAt(0)
    : null;
  ta.select();
  ta.setSelectionRange(0, ta.value.length);
  let ok = false;
  try {
    ok = document.execCommand("copy");
  } catch {
    ok = false;
  } finally {
    document.body.removeChild(ta);
    if (prevSelection) {
      const sel = document.getSelection();
      sel?.removeAllRanges();
      sel?.addRange(prevSelection);
    }
  }
  return ok;
}
