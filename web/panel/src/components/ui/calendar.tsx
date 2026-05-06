import * as React from "react";
import { DayPicker } from "react-day-picker";
import { cn } from "@/lib/utils";

export type CalendarProps = React.ComponentProps<typeof DayPicker>;

function ChevronLeft() {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="m15 18-6-6 6-6" />
    </svg>
  );
}

function ChevronRight() {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="m9 18 6-6-6-6" />
    </svg>
  );
}

export function Calendar({ className, classNames, showOutsideDays = true, ...props }: CalendarProps) {
  return (
    <DayPicker
      showOutsideDays={showOutsideDays}
      className={cn("p-3", className)}
      components={{
        Chevron: ({ orientation }) =>
          orientation === "left" ? <ChevronLeft /> : <ChevronRight />,
      }}
      classNames={{
        root: "select-none",
        months: "flex flex-col",
        month: "flex flex-col gap-3",
        month_caption: "flex justify-center items-center relative h-8",
        caption_label: "text-sm font-medium",
        nav: "absolute inset-x-0 top-0 flex items-center justify-between",
        button_previous: cn(
          "inline-flex h-7 w-7 items-center justify-center rounded-md",
          "border border-[hsl(var(--border))] bg-transparent",
          "text-[hsl(var(--muted-foreground))] hover:bg-[hsl(var(--accent))] hover:text-[hsl(var(--accent-foreground))]",
          "disabled:pointer-events-none disabled:opacity-50"
        ),
        button_next: cn(
          "inline-flex h-7 w-7 items-center justify-center rounded-md",
          "border border-[hsl(var(--border))] bg-transparent",
          "text-[hsl(var(--muted-foreground))] hover:bg-[hsl(var(--accent))] hover:text-[hsl(var(--accent-foreground))]",
          "disabled:pointer-events-none disabled:opacity-50"
        ),
        month_grid: "w-full border-collapse",
        weekdays: "flex",
        weekday: "w-9 text-center text-[0.75rem] font-normal text-[hsl(var(--muted-foreground))] pb-1",
        weeks: "",
        week: "flex w-full mt-1",
        day: "relative w-9 h-9 p-0 text-center text-sm",
        day_button: cn(
          "inline-flex h-9 w-9 items-center justify-center rounded-md p-0 text-sm font-normal w-full",
          "hover:bg-[hsl(var(--accent))] hover:text-[hsl(var(--accent-foreground))]",
          "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[hsl(var(--ring))]",
          "transition-colors"
        ),
        selected: "[&>button]:!bg-[hsl(var(--primary))] [&>button]:!text-[hsl(var(--primary-foreground))] [&>button]:hover:!bg-[hsl(var(--primary))]",
        today: "[&>button]:bg-[hsl(var(--accent))] [&>button]:text-[hsl(var(--accent-foreground))] [&>button]:font-semibold",
        outside: "opacity-40",
        disabled: "opacity-30 pointer-events-none",
        hidden: "invisible",
        ...classNames,
      }}
      {...props}
    />
  );
}

Calendar.displayName = "Calendar";
