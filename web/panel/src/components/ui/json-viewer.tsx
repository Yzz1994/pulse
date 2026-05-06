import { useState, useEffect } from "react";

// ── 颜色 token ────────────────────────────────────────────────────

const cls = {
  key:     "text-[#7dd3fc]",          // 键名：sky-300
  str:     "text-[#86efac]",          // 字符串值：green-300
  num:     "text-[#fdba74]",          // 数字：orange-300
  bool:    "text-[#c084fc]",          // boolean：purple-400
  nil:     "text-[#f87171]",          // null：red-400
  bracket: "text-[hsl(var(--foreground))]",
  meta:    "text-[hsl(var(--muted-foreground))]",
};

// ── 单个 JSON 值渲染 ──────────────────────────────────────────────

interface NodeProps {
  value: unknown;
  path: string;
  collapsed: Set<string>;
  toggle: (path: string) => void;
  depth: number;
}

function JsonNode({ value, path, collapsed, toggle, depth }: NodeProps) {
  const indent = "  ".repeat(depth);
  const isCollapsed = collapsed.has(path);

  // 对象
  if (value !== null && typeof value === "object" && !Array.isArray(value)) {
    const obj = value as Record<string, unknown>;
    const keys = Object.keys(obj);
    const open = "{", close = "}";

    if (keys.length === 0) {
      return <span className={cls.bracket}>{"{}"}</span>;
    }

    return (
      <>
        <button
          type="button"
          onClick={() => toggle(path)}
          className="hover:opacity-70 transition-opacity"
        >
          <span className={cls.bracket}>{open}</span>
        </button>
        {isCollapsed ? (
          <>
            <span className={`${cls.meta} cursor-pointer hover:opacity-70`} onClick={() => toggle(path)}>
              {" "}{keys.length} 项{" "}
            </span>
            <span className={cls.bracket}>{close}</span>
          </>
        ) : (
          <>
            {"\n"}
            {keys.map((k, i) => (
              <span key={k}>
                {indent}{"  "}
                <span className={cls.key}>"{k}"</span>
                <span className={cls.bracket}>: </span>
                <JsonNode
                  value={obj[k]}
                  path={`${path}.${k}`}
                  collapsed={collapsed}
                  toggle={toggle}
                  depth={depth + 1}
                />
                {i < keys.length - 1 && <span className={cls.bracket}>,</span>}
                {"\n"}
              </span>
            ))}
            {indent}<span className={cls.bracket}>{close}</span>
          </>
        )}
      </>
    );
  }

  // 数组
  if (Array.isArray(value)) {
    const arr = value;
    const open = "[", close = "]";

    if (arr.length === 0) {
      return <span className={cls.bracket}>{"[]"}</span>;
    }

    return (
      <>
        <button
          type="button"
          onClick={() => toggle(path)}
          className="hover:opacity-70 transition-opacity"
        >
          <span className={cls.bracket}>{open}</span>
        </button>
        {isCollapsed ? (
          <>
            <span className={`${cls.meta} cursor-pointer hover:opacity-70`} onClick={() => toggle(path)}>
              {" "}{arr.length} 项{" "}
            </span>
            <span className={cls.bracket}>{close}</span>
          </>
        ) : (
          <>
            {"\n"}
            {arr.map((item, i) => (
              <span key={i}>
                {indent}{"  "}
                <JsonNode
                  value={item}
                  path={`${path}[${i}]`}
                  collapsed={collapsed}
                  toggle={toggle}
                  depth={depth + 1}
                />
                {i < arr.length - 1 && <span className={cls.bracket}>,</span>}
                {"\n"}
              </span>
            ))}
            {indent}<span className={cls.bracket}>{close}</span>
          </>
        )}
      </>
    );
  }

  // 基础类型
  if (typeof value === "string") {
    return <span className={cls.str}>"{value}"</span>;
  }
  if (typeof value === "number") {
    return <span className={cls.num}>{value}</span>;
  }
  if (typeof value === "boolean") {
    return <span className={cls.bool}>{String(value)}</span>;
  }
  if (value === null) {
    return <span className={cls.nil}>null</span>;
  }
  return <span>{String(value)}</span>;
}

// ── 默认收起深度较深的节点（depth >= threshold） ──────────────────

function collectDeepPaths(value: unknown, path: string, threshold: number, depth: number, out: Set<string>) {
  if (depth >= threshold) {
    if (value !== null && typeof value === "object") {
      out.add(path);
      return; // 收起后不继续递归
    }
  }
  if (Array.isArray(value)) {
    value.forEach((item, i) => collectDeepPaths(item, `${path}[${i}]`, threshold, depth + 1, out));
  } else if (value !== null && typeof value === "object") {
    for (const k of Object.keys(value as object)) {
      collectDeepPaths((value as Record<string, unknown>)[k], `${path}.${k}`, threshold, depth + 1, out);
    }
  }
}

// ── 公开组件 ──────────────────────────────────────────────────────

interface JsonViewerProps {
  /** JSON 字符串或已解析的对象 */
  src: string | unknown;
  /** 初始自动收起深度（默认 3，即第 3 层以下收起） */
  collapseDepth?: number;
  className?: string;
}

export function JsonViewer({ src, collapseDepth = 3, className }: JsonViewerProps) {
  const parsed = (() => {
    if (typeof src === "string") {
      try { return JSON.parse(src); } catch { return null; }
    }
    return src;
  })();

  const [collapsed, setCollapsed] = useState<Set<string>>(() => {
    const s = new Set<string>();
    collectDeepPaths(parsed, "root", collapseDepth, 0, s);
    return s;
  });

  // src 切换时重置折叠状态
  useEffect(() => {
    const s = new Set<string>();
    collectDeepPaths(parsed, "root", collapseDepth, 0, s);
    setCollapsed(s);
  }, [src, collapseDepth]); // eslint-disable-line react-hooks/exhaustive-deps

  const toggle = (path: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  };

  if (parsed === null && typeof src === "string") {
    // JSON 解析失败，原样显示
    return (
      <pre className={`font-mono text-xs leading-relaxed ${className ?? ""}`}>
        {src}
      </pre>
    );
  }

  return (
    <pre className={`font-mono text-xs leading-relaxed whitespace-pre ${className ?? ""}`}>
      <JsonNode value={parsed} path="root" collapsed={collapsed} toggle={toggle} depth={0} />
      {"\n"}
    </pre>
  );
}
