import * as React from "react";
import { format, parse, isValid } from "date-fns";
import { zhCN } from "date-fns/locale";
import { Calendar } from "./calendar";
import { Popover, PopoverContent, PopoverTrigger } from "./popover";
import { Button } from "./button";
import { cn } from "@/lib/utils";

interface DatePickerProps {
  /** YYYY-MM-DD 格式，空字符串表示未选择 */
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
  /** 禁用早于此日期的日期 */
  fromDate?: Date;
  className?: string;
}

function CalendarIcon({ className }: { className?: string }) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      className={cn("h-4 w-4", className)}
      fill="none"
      viewBox="0 0 24 24"
      stroke="currentColor"
      strokeWidth={1.5}
    >
      <path strokeLinecap="round" strokeLinejoin="round" d="M6.75 3v2.25M17.25 3v2.25M3 18.75V7.5a2.25 2.25 0 0 1 2.25-2.25h13.5A2.25 2.25 0 0 1 21 7.5v11.25m-18 0A2.25 2.25 0 0 0 5.25 21h13.5A2.25 2.25 0 0 0 21 18.75m-18 0v-7.5A2.25 2.25 0 0 1 5.25 9h13.5A2.25 2.25 0 0 1 21 11.25v7.5" />
    </svg>
  );
}

export function DatePicker({
  value,
  onChange,
  placeholder = "选择日期",
  disabled,
  fromDate,
  className,
}: DatePickerProps) {
  const [open, setOpen] = React.useState(false);

  const selected = React.useMemo<Date | undefined>(() => {
    if (!value) return undefined;
    const d = parse(value, "yyyy-MM-dd", new Date());
    return isValid(d) ? d : undefined;
  }, [value]);

  const handleSelect = (date: Date | undefined) => {
    onChange(date ? format(date, "yyyy-MM-dd") : "");
    setOpen(false);
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          disabled={disabled}
          className={cn(
            "w-full justify-start text-left font-normal",
            !selected && "text-[hsl(var(--muted-foreground))]",
            className
          )}
        >
          <CalendarIcon className="mr-2 shrink-0 opacity-60" />
          {selected ? format(selected, "yyyy年M月d日", { locale: zhCN }) : placeholder}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-auto p-0" align="start">
        <Calendar
          mode="single"
          selected={selected}
          onSelect={handleSelect}
          defaultMonth={selected}
          disabled={fromDate ? { before: fromDate } : undefined}
          locale={zhCN}
        />
      </PopoverContent>
    </Popover>
  );
}
