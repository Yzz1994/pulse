// 复制文本到剪贴板。
// 在 secure context（HTTPS / localhost）下使用 navigator.clipboard，
// 否则降级到 document.execCommand('copy')，保证 HTTP 远程访问也能用。
// 失败时抛出异常，由调用方决定如何向用户反馈。
export async function copyText(text: string): Promise<void> {
  if (typeof navigator !== "undefined" && navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text);
      return;
    } catch {
      // 继续走降级
    }
  }
  if (typeof document === "undefined") {
    throw new Error("clipboard: document unavailable");
  }
  const ta = document.createElement("textarea");
  ta.value = text;
  ta.setAttribute("readonly", "");
  // iOS Safari 要求元素在视口内且 contentEditable 才能 select；用 fixed + 极小尺寸 + 透明
  // 兼容桌面与移动浏览器，避免触发 scroll 跳动。
  ta.style.cssText = "position:fixed;top:0;left:0;width:1px;height:1px;padding:0;border:0;outline:0;background:transparent;opacity:0;pointer-events:none;";
  ta.contentEditable = "true";
  document.body.appendChild(ta);
  const prevSelection = document.getSelection()?.rangeCount
    ? document.getSelection()!.getRangeAt(0)
    : null;
  ta.focus();
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
  if (!ok) {
    throw new Error("clipboard: copy command rejected");
  }
}
