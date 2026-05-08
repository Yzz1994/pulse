// 复制文本到剪贴板。
// 在 secure context（HTTPS / localhost）下使用 navigator.clipboard，
// 否则降级到 document.execCommand('copy')，保证 HTTP 远程访问也能用。
// container：调用方传入的宿主元素（例如当前 Dialog），用于绕过 Radix focus trap。
// 失败时抛出异常，由调用方决定如何向用户反馈。
export async function copyText(text: string, container?: HTMLElement | null): Promise<void> {
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
  // 必须保留 readonly 防止移动端弹出键盘，但不能 display:none / visibility:hidden，
  // 否则 execCommand('copy') 在多数浏览器下会失败。
  ta.setAttribute("readonly", "");
  ta.style.position = "fixed";
  ta.style.top = "0";
  ta.style.left = "0";
  ta.style.width = "1px";
  ta.style.height = "1px";
  ta.style.padding = "0";
  ta.style.border = "0";
  ta.style.outline = "0";
  ta.style.boxShadow = "none";
  ta.style.background = "transparent";
  ta.style.opacity = "0";
  // Radix Dialog 有 focus trap：body 层的元素无法被 focus，execCommand 会失败。
  // 调用方应传入当前 Dialog 元素作为 container，确保 textarea 在 focus scope 之内。
  const appendTo = container ?? document.body;
  appendTo.appendChild(ta);
  const prevActive = document.activeElement as HTMLElement | null;
  let ok = false;
  try {
    ta.focus({ preventScroll: true });
    ta.select();
    ta.setSelectionRange(0, ta.value.length);
    ok = document.execCommand("copy");
  } catch {
    ok = false;
  } finally {
    appendTo.removeChild(ta);
    if (prevActive && typeof prevActive.focus === "function") {
      try { prevActive.focus({ preventScroll: true } as FocusOptions); } catch { /* noop */ }
    }
  }
  if (!ok) {
    throw new Error("clipboard: copy command rejected");
  }
}
