export function parseRRule(raw?: string) {
  const out = { freq: "", interval: 1, until: "", count: "" };
  if (!raw) return out;
  for (const part of raw.split(";")) {
    const [k, v] = part.split("=");
    if (!k || !v) continue;
    if (k === "FREQ") out.freq = v;
    if (k === "INTERVAL") out.interval = Number(v) || 1;
    if (k === "UNTIL") out.until = v.length >= 8 ? `${v.slice(0, 4)}-${v.slice(4, 6)}-${v.slice(6, 8)}` : v;
    if (k === "COUNT") out.count = v;
  }
  return out;
}

export function buildRRule(freq: string, interval: number, until: string, count: string) {
  if (!freq) return "";
  const parts = [`FREQ=${freq}`];
  if (interval > 1) parts.push(`INTERVAL=${interval}`);
  if (until) parts.push(`UNTIL=${until.replace(/-/g, "")}`);
  if (count) parts.push(`COUNT=${count}`);
  return parts.join(";");
}

export function untilDateKey(v: string) {
  const d = (v || "").replace(/-/g, "");
  return d.length >= 8 ? d.slice(0, 8) : d;
}

export function rruleForSave(original: string | undefined, freq: string, interval: number, until: string, count: string) {
  const built = buildRRule(freq, interval, until, count);
  if (!original) return built;
  const parsed = parseRRule(original);
  const same =
    parsed.freq === freq &&
    parsed.interval === interval &&
    untilDateKey(parsed.until) === untilDateKey(until) &&
    parsed.count === count;
  if (same) return original;
  return built;
}
