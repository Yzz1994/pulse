import { useRef, useState } from "react";
import * as Popover from "@radix-ui/react-popover";
import { ScrollArea } from "./scroll-area";

// ── SingleSelect ──────────────────────────────────────────────────────────────
// 与 MultiSelect 视觉一致的单选下拉，解决 Radix Select 在 Dialog 内滚动异常的问题。

export interface SingleSelectOption {
  value: string;
  label: React.ReactNode;
  triggerLabel?: string;
}

interface SingleSelectProps {
  value: string;
  onChange: (value: string) => void;
  options: SingleSelectOption[];
  placeholder?: string;
}

export function SingleSelect({
  value,
  onChange,
  options,
  placeholder = "选择...",
}: SingleSelectProps) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  const selected = options.find((o) => o.value === value);
  const triggerText = selected
    ? (selected.triggerLabel ?? (typeof selected.label === "string" ? selected.label : null))
    : null;

  const filtered = search.trim()
    ? options.filter((o) => {
        const text = o.triggerLabel ?? (typeof o.label === "string" ? o.label : "");
        return text.toLowerCase().includes(search.toLowerCase());
      })
    : options;

  const handleOpenChange = (v: boolean) => {
    setOpen(v);
    if (!v) setSearch("");
  };

  const handleSelect = (v: string) => {
    onChange(v);
    setOpen(false);
  };

  return (
    <Popover.Root open={open} onOpenChange={handleOpenChange}>
      <Popover.Trigger asChild>
        <button
          type="button"
          className="flex h-9 w-full items-center justify-between rounded-md border border-[hsl(var(--input))] bg-transparent px-3 py-2 text-sm shadow-sm focus:outline-none focus:ring-1 focus:ring-[hsl(var(--ring))]"
        >
          <span className={triggerText == null ? "text-[hsl(var(--muted-foreground))]" : ""}>
            {triggerText ?? placeholder}
          </span>
          <svg
            className="h-4 w-4 opacity-50 shrink-0"
            xmlns="http://www.w3.org/2000/svg"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth={2}
          >
            <path d="m6 9 6 6 6-6" />
          </svg>
        </button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          className="z-50 w-[var(--radix-popover-trigger-width)] rounded-md border border-[hsl(var(--border))] bg-[hsl(var(--popover))] p-1 text-[hsl(var(--popover-foreground))] shadow-md"
          sideOffset={4}
          align="start"
          onOpenAutoFocus={(e) => { e.preventDefault(); inputRef.current?.focus(); }}
        >
          <div className="px-1 pb-1">
            <input
              ref={inputRef}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="搜索..."
              className="h-8 w-full rounded-sm border border-[hsl(var(--input))] bg-transparent px-2 text-sm outline-none placeholder:text-[hsl(var(--muted-foreground))]"
            />
          </div>
          <ScrollArea
            style={{ maxHeight: 224 }}
            onWheel={(e) => e.stopPropagation()}
            onTouchMove={(e) => e.stopPropagation()}
          >
            {filtered.length === 0 && (
              <p className="px-2 py-3 text-center text-xs text-[hsl(var(--muted-foreground))]">无匹配结果</p>
            )}
            {filtered.map((opt) => (
              <button
                key={opt.value}
                type="button"
                className="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-sm hover:bg-[hsl(var(--accent))]"
                onClick={() => handleSelect(opt.value)}
              >
                <span className="flex h-4 w-4 shrink-0 items-center justify-center">
                  {value === opt.value && (
                    <svg
                      className="h-3 w-3"
                      viewBox="0 0 12 12"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth={2.5}
                    >
                      <path d="M2 6l3 3 5-5" />
                    </svg>
                  )}
                </span>
                <span className="min-w-0 text-left">{opt.label}</span>
              </button>
            ))}
          </ScrollArea>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}

export interface MultiSelectOption {
  value: string;
  /** 下拉列表中显示的内容（可以是 ReactNode） */
  label: React.ReactNode;
  /** 触发器上显示的纯文本（单选时展示，省略则回退到 label 的字符串值） */
  triggerLabel?: string;
}

interface MultiSelectProps {
  value: string[];
  onChange: (values: string[]) => void;
  options: MultiSelectOption[];
  /** 未选中时的占位文字（灰色） */
  placeholder?: string;
  /** 计数文字模板，{n} 会被替换成数字 */
  countLabel?: string;
}

export function MultiSelect({
  value,
  onChange,
  options,
  placeholder = "选择...",
  countLabel = "已选 {n} 项",
}: MultiSelectProps) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  const filtered = search.trim()
    ? options.filter((o) => {
        const text = o.triggerLabel ?? (typeof o.label === "string" ? o.label : "");
        return text.toLowerCase().includes(search.toLowerCase());
      })
    : options;

  const handleOpenChange = (v: boolean) => {
    setOpen(v);
    if (!v) setSearch("");
  };

  const toggle = (v: string) => {
    onChange(value.includes(v) ? value.filter((x) => x !== v) : [...value, v]);
  };

  // 触发器显示文字
  const triggerText = (() => {
    if (value.length === 0) return null;
    if (value.length === 1) {
      const opt = options.find((o) => o.value === value[0]);
      if (opt) return opt.triggerLabel ?? (typeof opt.label === "string" ? opt.label : null);
    }
    return countLabel.replace("{n}", String(value.length));
  })();

  return (
    <Popover.Root open={open} onOpenChange={handleOpenChange}>
      <Popover.Trigger asChild>
        <button
          type="button"
          className="flex h-9 w-full items-center justify-between rounded-md border border-[hsl(var(--input))] bg-transparent px-3 py-2 text-sm shadow-sm focus:outline-none focus:ring-1 focus:ring-[hsl(var(--ring))]"
        >
          <span className={triggerText == null ? "text-[hsl(var(--muted-foreground))]" : ""}>
            {triggerText ?? placeholder}
          </span>
          <svg
            className="h-4 w-4 opacity-50 shrink-0"
            xmlns="http://www.w3.org/2000/svg"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth={2}
          >
            <path d="m6 9 6 6 6-6" />
          </svg>
        </button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          className="z-50 w-[var(--radix-popover-trigger-width)] rounded-md border border-[hsl(var(--border))] bg-[hsl(var(--popover))] p-1 text-[hsl(var(--popover-foreground))] shadow-md"
          sideOffset={4}
          align="start"
          onOpenAutoFocus={(e) => { e.preventDefault(); inputRef.current?.focus(); }}
        >
          <div className="px-1 pb-1">
            <input
              ref={inputRef}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="搜索..."
              className="h-8 w-full rounded-sm border border-[hsl(var(--input))] bg-transparent px-2 text-sm outline-none placeholder:text-[hsl(var(--muted-foreground))]"
            />
          </div>
          {/* onWheel/onTouchMove stopPropagation 防止 Dialog 的 react-remove-scroll 拦截滚动 */}
          <ScrollArea
            style={{ maxHeight: 224 }}
            onWheel={(e) => e.stopPropagation()}
            onTouchMove={(e) => e.stopPropagation()}
          >
            {/* 全选 / 取消全选（作用于当前过滤结果） */}
            {filtered.length > 0 && (
              <button
                type="button"
                className="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-sm text-[hsl(var(--muted-foreground))] hover:bg-[hsl(var(--accent))]"
                onClick={() => {
                  const filteredValues = filtered.map((o) => o.value);
                  const allSelected = filteredValues.every((v) => value.includes(v));
                  if (allSelected) {
                    onChange(value.filter((v) => !filteredValues.includes(v)));
                  } else {
                    onChange([...new Set([...value, ...filteredValues])]);
                  }
                }}
              >
                <span className="flex h-4 w-4 shrink-0 items-center justify-center">
                  {filtered.every((o) => value.includes(o.value)) && (
                    <svg className="h-3 w-3" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth={2.5}>
                      <path d="M2 6l3 3 5-5" />
                    </svg>
                  )}
                </span>
                {filtered.every((o) => value.includes(o.value)) ? "取消全选" : "全选"}
              </button>
            )}
            {filtered.length === 0 && (
              <p className="px-2 py-3 text-center text-xs text-[hsl(var(--muted-foreground))]">无匹配结果</p>
            )}
            {filtered.map((opt) => (
              <button
                key={opt.value}
                type="button"
                className="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-sm hover:bg-[hsl(var(--accent))]"
                onClick={() => toggle(opt.value)}
              >
                {/* 勾选指示器 */}
                <span className="flex h-4 w-4 shrink-0 items-center justify-center">
                  {value.includes(opt.value) && (
                    <svg
                      className="h-3 w-3"
                      viewBox="0 0 12 12"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth={2.5}
                    >
                      <path d="M2 6l3 3 5-5" />
                    </svg>
                  )}
                </span>
                <span className="min-w-0 text-left">{opt.label}</span>
              </button>
            ))}
          </ScrollArea>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}
