export const TIMEZONES = [
  "UTC",
  "Europe/Athens",
  "Europe/Berlin",
  "Europe/London",
  "Europe/Paris",
  "America/New_York",
  "America/Chicago",
  "America/Los_Angeles",
  "America/Sao_Paulo",
  "Asia/Tokyo",
  "Asia/Singapore",
  "Australia/Sydney",
];

export function toICS(local: string, asUTC = false): string {
  if (!local) return "";
  const m = local.match(/^(\d{4})-(\d{2})-(\d{2})(?:T(\d{2}):(\d{2})(?::(\d{2}))?)?(Z)?$/i);
  if (m?.[4] && asUTC) {
    const d = new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]), Number(m[4]), Number(m[5]), Number(m[6] || 0));
    const p = (n: number) => String(n).padStart(2, "0");
    return `${d.getUTCFullYear()}${p(d.getUTCMonth() + 1)}${p(d.getUTCDate())}T${p(d.getUTCHours())}${p(d.getUTCMinutes())}${p(d.getUTCSeconds())}Z`;
  }
  return compactICS(local);
}

export function toICSDate(ymd: string): string {
  return (ymd || "").replace(/-/g, "").slice(0, 8);
}

function compactICS(v: string): string {
  v = (v || "").trim();
  if (!v) return "";
  const z = /Z$/i.test(v);
  v = v.replace(/Z$/i, "").replace(/[-:]/g, "").replace(/[Tt]/g, "");
  const dot = v.indexOf(".");
  if (dot >= 0) v = v.slice(0, dot);
  if (v.length === 12) v += "00";
  if (v.length === 8) return v;
  if (v.length >= 14) return `${v.slice(0, 8)}T${v.slice(8, 14)}${z ? "Z" : ""}`;
  return z ? `${v}Z` : v;
}

export function fromICS(v: string): string {
  if (!v) return "";
  const compact = compactICS(v);
  const m = compact.match(/^(\d{4})(\d{2})(\d{2})(?:T(\d{2})(\d{2})(\d{2}))?(Z)?$/i);
  if (!m) return "";
  if (!m[4]) return `${m[1]}-${m[2]}-${m[3]}T00:00`;
  if (m[7]) {
    const utc = Date.UTC(Number(m[1]), Number(m[2]) - 1, Number(m[3]), Number(m[4]), Number(m[5]), Number(m[6] || 0));
    const d = new Date(utc);
    const p = (n: number) => String(n).padStart(2, "0");
    return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`;
  }
  return `${m[1]}-${m[2]}-${m[3]}T${m[4]}:${m[5]}`;
}

export function formatWhen(v: string, allDay?: boolean): string {
  if (!v) return "—";
  const m = v.match(/^(\d{4})(\d{2})(\d{2})(?:T(\d{2})(\d{2})(\d{2})?Z?)?/);
  if (!m) return v;
  const day = `${m[1]}-${m[2]}-${m[3]}`;
  if (allDay || !m[4]) return day;
  return `${day} ${m[4]}:${m[5]}`;
}

export function formatWhenRange(start: string, end?: string, allDay?: boolean): string {
  const from = formatWhen(start, allDay);
  if (!end) return from;
  if (allDay || !end.includes("T")) {
    const last = fromICSDate(end);
    const first = fromICSDate(start);
    if (!last || last === first) return from;
    const inclusive = addDaysYMD(last, -1);
    return inclusive !== first ? `${first} – ${inclusive}` : from;
  }
  const to = formatWhen(end, allDay);
  return to === from ? from : `${from} – ${to}`;
}

export function displayIdentity(v: string): string {
  return (v || "").replace(/^mailto:/i, "").trim();
}

export function partstatLabel(partstat?: string) {
  const s = (partstat || "").toUpperCase();
  if (s === "ACCEPTED") return "Accepted";
  if (s === "DECLINED") return "Declined";
  if (s === "TENTATIVE") return "Tentative";
  if (s === "NEEDS-ACTION") return "Pending";
  return "";
}

export function fromICSDate(v: string): string {
  const m = (v || "").match(/^(\d{4})(\d{2})(\d{2})/);
  return m ? `${m[1]}-${m[2]}-${m[3]}` : "";
}

export function dueYMD(v: string): string {
  const compact = fromICSDate(v);
  if (compact) return compact;
  const m = (v || "").match(/^(\d{4}-\d{2}-\d{2})/);
  return m ? m[1] : "";
}

export function calendarLabel(c: { name: string; shared?: boolean; owner_username?: string }) {
  return c.shared && c.owner_username ? `${c.owner_username} — ${c.name}` : c.name;
}

export function eventTimeLabel(v: string, allDay?: boolean): string {
  if (allDay || !v.includes("T")) return "All day";
  const m = v.match(/T(\d{2})(\d{2})/);
  return m ? `${m[1]}:${m[2]}` : "";
}

function longDate(d: Date) {
  return d.toLocaleDateString(undefined, { weekday: "long", month: "long", day: "numeric", year: "numeric" });
}

export function formatWhenPretty(start: string, end?: string, allDay?: boolean): { primary: string; secondary: string } {
  const startDay = fromICSDate(start);
  if (!startDay) return { primary: "—", secondary: "" };
  const startDate = parseYMD(startDay);
  if (allDay || !start.includes("T")) {
    const lastExcl = fromICSDate(end || "");
    if (lastExcl && lastExcl > startDay) {
      const lastIncl = parseYMD(addDaysYMD(lastExcl, -1));
      if (ymd(lastIncl) !== startDay) {
        return { primary: `${longDate(startDate)} – ${longDate(lastIncl)}`, secondary: "All day" };
      }
    }
    return { primary: longDate(startDate), secondary: "All day" };
  }
  const t0 = eventTimeLabel(start);
  const t1 = end && /T\d{2}/.test(end) ? eventTimeLabel(end) : "";
  return { primary: longDate(startDate), secondary: t1 && t1 !== t0 ? `${t0} – ${t1}` : t0 };
}

export function ymd(d: Date): string {
  const z = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${z(d.getMonth() + 1)}-${z(d.getDate())}`;
}

export function parseYMD(s: string): Date {
  const [y, m, d] = s.split("-").map(Number);
  return new Date(y, (m || 1) - 1, d || 1);
}

export function startOfWeek(d: Date): Date {
  const x = new Date(d.getFullYear(), d.getMonth(), d.getDate());
  const dow = (x.getDay() + 6) % 7;
  x.setDate(x.getDate() - dow);
  return x;
}

export function addDays(d: Date, n: number): Date {
  const x = new Date(d);
  x.setDate(x.getDate() + n);
  return x;
}

export function addDaysYMD(s: string, n: number): string {
  return ymd(addDays(parseYMD(s), n));
}

export function sameDay(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

export function monthGrid(cursor: Date): Date[] {
  const first = new Date(cursor.getFullYear(), cursor.getMonth(), 1);
  const start = startOfWeek(first);
  return Array.from({ length: 42 }, (_, i) => addDays(start, i));
}

export function eventSpan(ev: { dtstart: string; dtend?: string; all_day?: boolean }): { start: string; endExclusive: string } {
  const start = fromICSDate(ev.dtstart);
  if (!start) return { start: "", endExclusive: "" };
  const endRaw = ev.dtend || "";
  const end = fromICSDate(endRaw);
  if (!end || end < start) {
    return { start, endExclusive: addDaysYMD(start, 1) };
  }
  const allDay = Boolean(ev.all_day) || !/T\d{2}/.test(endRaw);
  if (allDay) {
    if (end === start) return { start, endExclusive: addDaysYMD(start, 1) };
    return { start, endExclusive: end };
  }
  const compact = endRaw.replace(/[-:]/g, "");
  if (/T000000/.test(compact) && end > start) {
    return { start, endExclusive: end };
  }
  return { start, endExclusive: addDaysYMD(end, 1) };
}

export function eventOnDay(ev: { dtstart: string; dtend?: string; all_day?: boolean }, day: Date): boolean {
  const key = ymd(day);
  const { start, endExclusive } = eventSpan(ev);
  return Boolean(start) && key >= start && key < endExclusive;
}

export function eventOverlaps(ev: { dtstart: string; dtend?: string; all_day?: boolean }, from: string, toExclusive: string): boolean {
  const { start, endExclusive } = eventSpan(ev);
  return Boolean(start) && start < toExclusive && endExclusive > from;
}

export function calendarWritable(c: { read_only?: boolean; kind?: string; shared?: boolean; access?: string }): boolean {
  if (c.read_only || c.kind === "inbox" || c.kind === "outbox" || c.kind === "important_dates") return false;
  if (c.shared && c.access !== "write") return false;
  return true;
}

export function calendarAcceptTarget(c: { read_only?: boolean; kind?: string; shared?: boolean; access?: string }): boolean {
  return calendarWritable(c) && c.kind === "personal";
}

export function calendarOwner(c: { shared?: boolean; access?: string }): boolean {
  return !c.shared && (c.access === "owner" || !c.access);
}

export function generatePassword(length = 16): string {
  const chars = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789!@$%";
  const buf = new Uint8Array(length);
  crypto.getRandomValues(buf);
  return Array.from(buf, (n) => chars[n % chars.length]).join("");
}

export function roleLabel(role: string): string {
  if (role === "admin") return "Administrator";
  return "Member";
}

export async function copyText(value: string): Promise<void> {
  await navigator.clipboard.writeText(value);
}

export function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) return "";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${Math.round((n / 1024) * 10) / 10} KB`;
  return `${Math.round((n / (1024 * 1024)) * 10) / 10} MB`;
}
