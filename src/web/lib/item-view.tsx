"use client";

import { useEffect, useLayoutEffect, useRef, useState, type CSSProperties, type ReactNode } from "react";
import { Bell, CalendarDays, Check, Clock, FileText, ListTodo, MapPin, Paperclip, Pencil, Repeat, Trash2, Users } from "lucide-react";
import { api, type Calendar, type EventItem, type TaskItem } from "@/lib/api";
import { calendarWritable, displayIdentity, formatBytes, formatWhenPretty, parseYMD, partstatLabel } from "@/lib/format";
import { Modal } from "@/lib/ui";

export type CalendarItem = EventItem & {
  calendarId: number;
  color: string;
  calendarName: string;
  kind?: "event" | "task";
  task?: TaskItem;
};

export function itemCanWrite(item: CalendarItem, calendars: Calendar[]) {
  const c = calendars.find((x) => x.id === item.calendarId);
  return c ? calendarWritable(c) : false;
}

export function toEventPreset(item: CalendarItem): EventItem {
  return {
    href: item.href,
    uid: item.uid,
    summary: item.summary,
    description: item.description,
    location: item.location,
    dtstart: item.dtstart,
    dtend: item.dtend,
    all_day: item.all_day,
    rrule: item.rrule,
    alarm_minutes: item.alarm_minutes,
    attendees: item.attendees,
    attachments: item.attachments,
  };
}

function chipStyle(color: string): CSSProperties {
  return { ["--chip" as string]: color || "#E72625" };
}

function rruleLabel(raw?: string) {
  if (!raw) return "";
  const parts: Record<string, string> = {};
  for (const bit of raw.split(";")) {
    const [k, v] = bit.split("=");
    if (k && v) parts[k] = v;
  }
  const freq = (parts.FREQ || "").toUpperCase();
  const n = Number(parts.INTERVAL || 1) || 1;
  const units: Record<string, [string, string, string]> = {
    DAILY: ["Daily", "day", "days"],
    WEEKLY: ["Weekly", "week", "weeks"],
    MONTHLY: ["Monthly", "month", "months"],
    YEARLY: ["Yearly", "year", "years"],
  };
  const unit = units[freq];
  if (!unit) return "";
  let label = n === 1 ? unit[0] : `Every ${n} ${unit[2]}`;
  if (parts.COUNT) label += ` · ${parts.COUNT} times`;
  if (parts.UNTIL && parts.UNTIL.length >= 8) {
    const d = parseYMD(`${parts.UNTIL.slice(0, 4)}-${parts.UNTIL.slice(4, 6)}-${parts.UNTIL.slice(6, 8)}`);
    label += ` · until ${d.toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" })}`;
  }
  return label;
}

function alarmLabel(mins?: number) {
  if (!mins) return "";
  if (mins === 5) return "5 minutes before";
  if (mins === 10) return "10 minutes before";
  if (mins === 15) return "15 minutes before";
  if (mins === 30) return "30 minutes before";
  if (mins === 60) return "1 hour before";
  if (mins === 1440) return "1 day before";
  if (mins % 1440 === 0) return `${mins / 1440} days before`;
  if (mins % 60 === 0) return `${mins / 60} hours before`;
  return `${mins} minutes before`;
}

function Row({ icon, children }: { icon: ReactNode; children: ReactNode }) {
  return (
    <div className="item-view-row">
      <span className="item-view-icon" aria-hidden>
        {icon}
      </span>
      <div>{children}</div>
    </div>
  );
}

export function ItemViewDialog({
  item,
  canWrite,
  onClose,
  onEdit,
  onDelete,
  onComplete,
}: {
  item: CalendarItem;
  canWrite: boolean;
  onClose: () => void;
  onEdit: () => void;
  onDelete: () => void | Promise<void>;
  onComplete?: () => void | Promise<void>;
}) {
  const task = item.kind === "task";
  const when = task
    ? { primary: formatWhenPretty(item.dtstart, "", true).primary, secondary: "Due" }
    : formatWhenPretty(item.dtstart, item.dtend, item.all_day);
  const repeat = rruleLabel(item.rrule);
  const reminder = alarmLabel(item.alarm_minutes);
  const guests = item.attendees ?? [];
  const [busy, setBusy] = useState(false);

  async function run(fn?: () => void | Promise<void>) {
    if (!fn || busy) return;
    setBusy(true);
    try {
      await fn();
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal
      title={task ? "Task" : "Event"}
      icon={task ? <ListTodo size={22} aria-hidden="true" /> : <CalendarDays size={22} aria-hidden="true" />}
      size="sm"
      className="item-view"
      style={chipStyle(item.color)}
      titleId="item-view-heading"
      onClose={onClose}
      footer={
        <div className="form-actions">
          {canWrite ? (
            <>
              <button className="btn sm" type="button" onClick={onEdit} disabled={busy}>
                <Pencil size={14} aria-hidden />
                Edit
              </button>
              {task && onComplete ? (
                <button className="btn secondary sm" type="button" onClick={() => run(onComplete)} disabled={busy}>
                  <Check size={14} aria-hidden />
                  Mark done
                </button>
              ) : null}
              <button className="btn danger sm" type="button" onClick={() => run(onDelete)} disabled={busy}>
                <Trash2 size={14} aria-hidden />
                Delete
              </button>
            </>
          ) : (
            <p className="muted">This calendar is read-only.</p>
          )}
          <button className="btn secondary sm" type="button" onClick={onClose}>
            Close
          </button>
        </div>
      }
    >
      <div className="item-view-hero">
        <span className="item-view-mark" aria-hidden />
        <h2 id="item-view-title">{item.summary || item.uid}</h2>
      </div>
      <div className="item-view-rows">
        <Row icon={task ? <ListTodo size={16} /> : <Clock size={16} />}>
          <strong>{when.primary}</strong>
          {when.secondary ? <span className="muted">{when.secondary}</span> : null}
        </Row>
        <Row icon={<CalendarDays size={16} />}>
          <strong>{item.calendarName}</strong>
          {!canWrite ? <span className="muted">View only</span> : null}
        </Row>
        {item.location ? (
          <Row icon={<MapPin size={16} />}>
            <strong>{item.location}</strong>
          </Row>
        ) : null}
        {repeat ? (
          <Row icon={<Repeat size={16} />}>
            <strong>{repeat}</strong>
          </Row>
        ) : null}
        {reminder ? (
          <Row icon={<Bell size={16} />}>
            <strong>{reminder}</strong>
          </Row>
        ) : null}
        {guests.length ? (
          <Row icon={<Users size={16} />}>
            <div className="item-view-guests">
              {guests.map((a) => {
                const name = a.cn || displayIdentity(a.value);
                const st = partstatLabel(a.partstat);
                return (
                  <span key={a.value} className="chip">
                    {name}
                    {st ? <span className="muted"> · {st}</span> : null}
                  </span>
                );
              })}
            </div>
          </Row>
        ) : null}
        {item.description ? <p className="item-view-notes">{item.description}</p> : null}
        {(item.attachments ?? item.task?.attachments ?? []).length ? (
          <Row icon={<Paperclip size={16} />}>
            <div className="attach-list">
              {(item.attachments ?? item.task?.attachments ?? []).map((a) => (
                <button
                  key={a.id}
                  type="button"
                  className="attach-link"
                  onClick={() => api.downloadAttachment(item.calendarId, a.id, a.filename)}
                >
                  <FileText size={14} aria-hidden />
                  {a.filename}
                  <span className="muted">{formatBytes(a.size)}</span>
                </button>
              ))}
            </div>
          </Row>
        ) : null}
      </div>
    </Modal>
  );
}

export function ItemContextMenu({
  x,
  y,
  item,
  canWrite,
  onClose,
  onView,
  onEdit,
  onDelete,
  onComplete,
}: {
  x: number;
  y: number;
  item: CalendarItem;
  canWrite: boolean;
  onClose: () => void;
  onView: () => void;
  onEdit: () => void;
  onDelete: () => void;
  onComplete?: () => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState({ left: x, top: y });
  const task = item.kind === "task";

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const r = el.getBoundingClientRect();
    const pad = 8;
    setPos({
      left: Math.max(pad, Math.min(x, window.innerWidth - r.width - pad)),
      top: Math.max(pad, Math.min(y, window.innerHeight - r.height - pad)),
    });
  }, [x, y]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    const close = () => onClose();
    window.addEventListener("keydown", onKey);
    window.addEventListener("mousedown", close);
    window.addEventListener("scroll", close, true);
    window.addEventListener("resize", close);
    return () => {
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("mousedown", close);
      window.removeEventListener("scroll", close, true);
      window.removeEventListener("resize", close);
    };
  }, [onClose]);

  return (
    <div
      ref={ref}
      className="ctx-menu"
      style={{ left: pos.left, top: pos.top }}
      role="menu"
      aria-label={task ? "Task actions" : "Event actions"}
      onMouseDown={(e) => e.stopPropagation()}
    >
      <button type="button" role="menuitem" onClick={onView}>
        {task ? <ListTodo size={15} aria-hidden /> : <CalendarDays size={15} aria-hidden />}
        View
      </button>
      {canWrite ? (
        <>
          <button type="button" role="menuitem" onClick={onEdit}>
            <Pencil size={15} aria-hidden />
            Edit
          </button>
          {task && onComplete ? (
            <button type="button" role="menuitem" onClick={onComplete}>
              <Check size={15} aria-hidden />
              Mark done
            </button>
          ) : null}
          <hr />
          <button type="button" role="menuitem" className="danger" onClick={onDelete}>
            <Trash2 size={15} aria-hidden />
            Delete
          </button>
        </>
      ) : null}
    </div>
  );
}
