import { eventOnDay, fromICSDate, ymd } from "./format";

export const WEEK_HOUR_PX = 48;
export const WEEK_HOURS = 24;

export type TimedItem = {
  dtstart: string;
  dtend?: string;
  all_day?: boolean;
  kind?: string;
};

export function icsMinutes(v: string): number | null {
  const m = (v || "").match(/T(\d{2})(\d{2})/);
  if (!m) return null;
  return Number(m[1]) * 60 + Number(m[2]);
}

export function padHHMM(minutes: number): string {
  const n = Math.max(0, Math.min(24 * 60 - 15, Math.round(minutes / 15) * 15));
  const h = Math.floor(n / 60);
  const m = n % 60;
  return `${String(h).padStart(2, "0")}:${String(m).padStart(2, "0")}`;
}

export function addHourHHMM(hhmm: string): string {
  const [h, m] = hhmm.split(":").map(Number);
  const next = (h || 0) * 60 + (m || 0) + 60;
  if (next >= 24 * 60) return "23:45";
  return padHHMM(next);
}

export function timedRange(ev: TimedItem, day: Date): { startMin: number; endMin: number } | null {
  if (ev.kind === "task" || ev.all_day || !/T\d{2}/.test(ev.dtstart || "")) return null;
  if (!eventOnDay(ev, day)) return null;
  const dayKey = ymd(day);
  let start = icsMinutes(ev.dtstart) ?? 0;
  let end = ev.dtend && /T\d{2}/.test(ev.dtend) ? icsMinutes(ev.dtend) ?? start + 60 : start + 60;
  if (fromICSDate(ev.dtstart) < dayKey) start = 0;
  if (ev.dtend && fromICSDate(ev.dtend) > dayKey) end = WEEK_HOURS * 60;
  if (end <= start) end = Math.min(start + 30, WEEK_HOURS * 60);
  return { startMin: start, endMin: Math.min(end, WEEK_HOURS * 60) };
}

export type WeekBlock<T extends TimedItem> = {
  item: T;
  startMin: number;
  endMin: number;
  col: number;
  cols: number;
};

export function layoutDayTimed<T extends TimedItem>(items: T[], day: Date): WeekBlock<T>[] {
  const timed: WeekBlock<T>[] = [];
  for (const item of items) {
    const r = timedRange(item, day);
    if (!r) continue;
    timed.push({ item, startMin: r.startMin, endMin: r.endMin, col: 0, cols: 1 });
  }
  timed.sort((a, b) => a.startMin - b.startMin || a.endMin - b.endMin);
  const colEnd: number[] = [];
  for (const ev of timed) {
    let col = 0;
    while (col < colEnd.length && colEnd[col] > ev.startMin) col++;
    ev.col = col;
    if (col === colEnd.length) colEnd.push(ev.endMin);
    else colEnd[col] = ev.endMin;
  }
  const overlapCols = (ev: WeekBlock<T>) => {
    let n = 1;
    for (const other of timed) {
      if (other === ev) continue;
      if (other.startMin < ev.endMin && other.endMin > ev.startMin) {
        n = Math.max(n, other.col + 1, ev.col + 1);
      }
    }
    return n;
  };
  for (const ev of timed) {
    ev.cols = Math.max(ev.cols, overlapCols(ev));
  }
  for (const ev of timed) {
    let max = ev.cols;
    for (const other of timed) {
      if (other.startMin < ev.endMin && other.endMin > ev.startMin) {
        max = Math.max(max, other.cols, other.col + 1);
      }
    }
    ev.cols = max;
  }
  return timed;
}

export function minutesFromClientY(clientY: number, rectTop: number, height: number): number {
  if (height <= 0) return 9 * 60;
  const ratio = Math.max(0, Math.min(1, (clientY - rectTop) / height));
  return ratio * WEEK_HOURS * 60;
}
