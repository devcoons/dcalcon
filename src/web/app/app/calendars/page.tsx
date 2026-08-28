"use client";

import { FormEvent, useEffect, useMemo, useRef, useState, type CSSProperties, type MouseEvent } from "react";
import { CalendarDays, ChevronLeft, ChevronRight, Columns2, Download, Link2, List, ListTodo, Pencil, Plus, Share2, Unlink, Upload } from "lucide-react";
import { api, type Calendar, type DirectoryUser, type EventItem, type TaskItem } from "@/lib/api";
import { EventDialog } from "@/lib/event-dialog";
import {
  addDays,
  addDaysYMD,
  calendarLabel,
  calendarOwner,
  calendarWritable,
  dueYMD,
  eventOverlaps,
  eventSpan,
  eventTimeLabel,
  fromICSDate,
  monthGrid,
  parseYMD,
  sameDay,
  startOfWeek,
  toICSDate,
  ymd,
} from "@/lib/format";
import { ItemContextMenu, ItemViewDialog, itemCanWrite, toEventPreset, type CalendarItem } from "@/lib/item-view";
import { useSession } from "@/lib/shell";
import { ShareDialog } from "@/lib/share-dialog";
import { TaskDialog } from "@/lib/task-dialog";
import { Notices, useNotice } from "@/lib/ui";
import { layoutDayTimed, minutesFromClientY, padHHMM, WEEK_HOUR_PX, WEEK_HOURS } from "@/lib/week-grid";

type View = "month" | "week" | "list";
type CalEvent = CalendarItem;

const VIEWS: { id: View; label: string; icon: typeof CalendarDays }[] = [
  { id: "month", label: "Month", icon: CalendarDays },
  { id: "week", label: "Week", icon: Columns2 },
  { id: "list", label: "List", icon: List },
];

function chipStyle(color: string): CSSProperties {
  return { ["--chip" as string]: color || "#E72625" };
}

function eventChipText(ev: CalEvent, day: Date) {
  const starts = fromICSDate(ev.dtstart) === ymd(day);
  if (ev.kind === "task") return ev.summary || ev.uid;
  if (starts && !ev.all_day && ev.dtstart.includes("T")) {
    return `${eventTimeLabel(ev.dtstart)}  ${ev.summary}`;
  }
  return ev.summary || ev.uid;
}

function itemWhen(ev: CalEvent) {
  if (ev.kind === "task") return "Due";
  return eventTimeLabel(ev.dtstart, ev.all_day);
}

function itemKey(ev: CalEvent) {
  return `${ev.kind ?? "event"}-${ev.calendarId}-${ev.href}`;
}

function chipClass(ev: CalEvent, extra = "") {
  return `cal-chip${ev.kind === "task" ? " task" : ""}${extra ? ` ${extra}` : ""}`;
}

export default function CalendarsPage() {
  const { user, refreshOverview } = useSession();
  const notice = useNotice();
  const [cals, setCals] = useState<Calendar[]>([]);
  const [directory, setDirectory] = useState<DirectoryUser[]>([]);
  const [events, setEvents] = useState<CalEvent[]>([]);
  const [visible, setVisible] = useState<Record<number, boolean>>({});
  const [view, setView] = useState<View>("month");
  const [cursor, setCursor] = useState(() => new Date());
  const [name, setName] = useState("");
  const [color, setColor] = useState("#E72625");
  const [desc, setDesc] = useState("");
  const [creating, setCreating] = useState(false);
  const [eventDlg, setEventDlg] = useState<{ calendarId: number; day?: string; time?: string; event?: EventItem } | null>(null);
  const [taskDlg, setTaskDlg] = useState<null | { item: TaskItem | null; calendarId: number; due?: string }>(null);
  const [viewItem, setViewItem] = useState<CalEvent | null>(null);
  const [menu, setMenu] = useState<{ x: number; y: number; item: CalEvent } | null>(null);
  const [shareCal, setShareCal] = useState<Calendar | null>(null);
  const [editCal, setEditCal] = useState<Calendar | null>(null);
  const [editName, setEditName] = useState("");
  const [editColor, setEditColor] = useState("#E72625");
  const [editDesc, setEditDesc] = useState("");
  const [webcalOn, setWebcalOn] = useState(false);
  const [webcalURL, setWebcalURL] = useState("");
  const weekScroll = useRef<HTMLDivElement>(null);
  const icsFile = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (view !== "week") return;
    weekScroll.current?.scrollTo({ top: 7 * WEEK_HOUR_PX });
  }, [view, cursor]);
  const loadGen = useRef(0);

  const shown = useMemo(() => cals.filter((c) => c.kind !== "inbox" && c.kind !== "outbox"), [cals]);
  const writable = shown.filter(calendarWritable);
  const personalOwned = shown.filter((c) => calendarOwner(c) && c.kind === "personal");

  async function load(signal?: AbortSignal) {
    const gen = ++loadGen.current;
    const init = signal ? { signal } : undefined;
    const [list, people] = await Promise.all([api.calendars(init), api.directory(init)]);
    if (gen !== loadGen.current) return;
    const usable = (list ?? []).filter((c) => c.kind !== "inbox" && c.kind !== "outbox");
    setCals(usable);
    setDirectory(people ?? []);
    setVisible((prev) => {
      const next = { ...prev };
      for (const c of usable) if (next[c.id] === undefined) next[c.id] = true;
      return next;
    });
    const [packed, taskList] = await Promise.all([
      Promise.all(
        usable.map(async (c) => {
          const items = (await api.events(c.id, init)) ?? [];
          return items.map((ev) => ({
            ...ev,
            calendarId: c.id,
            color: c.color,
            calendarName: calendarLabel(c),
            kind: "event" as const,
          }));
        }),
      ),
      api.tasks(init),
    ]);
    if (gen !== loadGen.current) return;
    const tasks = taskList ?? [];
    const byId = new Map(usable.map((c) => [c.id, c]));
    const dueTasks: CalEvent[] = [];
    for (const t of tasks) {
      if (t.status === "COMPLETED") continue;
      const day = dueYMD(t.due);
      if (!day) continue;
      const c = byId.get(t.calendar_id);
      if (!c) continue;
      dueTasks.push({
        href: t.href,
        uid: t.uid,
        summary: t.summary,
        description: t.description,
        dtstart: toICSDate(day),
        dtend: "",
        all_day: true,
        calendarId: t.calendar_id,
        color: c.color,
        calendarName: calendarLabel(c),
        kind: "task",
        task: t,
        attachments: t.attachments,
      });
    }
    setEvents([...packed.flat(), ...dueTasks]);
  }

  useEffect(() => {
    const ac = new AbortController();
    load(ac.signal).catch((e) => {
      if (e instanceof DOMException && e.name === "AbortError") return;
      if (e instanceof Error && e.name === "AbortError") return;
      notice.failFrom(e, "Could not load calendars.");
    });
    return () => ac.abort();
  }, []);

  const visibleEvents = useMemo(() => events.filter((ev) => visible[ev.calendarId] !== false), [events, visible]);
  const eventsByDay = useMemo(() => {
    const m = new Map<string, CalEvent[]>();
    for (const ev of visibleEvents) {
      const { start, endExclusive } = eventSpan(ev);
      if (!start) continue;
      let d = start;
      for (let i = 0; i < 400 && d < endExclusive; i++) {
        const arr = m.get(d);
        if (arr) arr.push(ev);
        else m.set(d, [ev]);
        d = addDaysYMD(d, 1);
      }
    }
    return m;
  }, [visibleEvents]);

  function eventsOn(day: Date) {
    return eventsByDay.get(ymd(day)) ?? [];
  }

  function openView(ev: CalEvent) {
    setMenu(null);
    setViewItem(ev);
  }

  function openEdit(ev: CalEvent) {
    setMenu(null);
    setViewItem(null);
    if (ev.kind === "task" && ev.task) {
      setTaskDlg({ item: ev.task, calendarId: ev.calendarId });
      return;
    }
    setEventDlg({ calendarId: ev.calendarId, event: toEventPreset(ev) });
  }

  function itemPointer(ev: CalEvent) {
    return {
      onClick: (e: MouseEvent<HTMLButtonElement>) => {
        e.stopPropagation();
        openView(ev);
      },
      onContextMenu: (e: MouseEvent<HTMLButtonElement>) => {
        e.preventDefault();
        e.stopPropagation();
        setViewItem(null);
        setMenu({ x: e.clientX, y: e.clientY, item: ev });
      },
    };
  }

  async function deleteItem(ev: CalEvent) {
    if (!itemCanWrite(ev, shown)) return;
    setMenu(null);
    const kind = ev.kind === "task" ? "task" : "event";
    if (!window.confirm(`Delete “${ev.summary || kind}”?`)) return;
    try {
      if (ev.kind === "task") await api.deleteTask(ev.calendarId, ev.href);
      else await api.deleteEvent(ev.calendarId, ev.href);
      setViewItem(null);
      await load();
      await refreshOverview();
      notice.done(ev.kind === "task" ? "Task deleted." : "Event deleted.");
    } catch (ex) {
      notice.fail(ex instanceof Error ? ex.message : "Could not delete.");
    }
  }

  async function completeTask(ev: CalEvent) {
    if (!ev.task || !itemCanWrite(ev, shown)) return;
    setMenu(null);
    try {
      await api.updateTask(ev.task.calendar_id, ev.task.href, {
        summary: ev.task.summary,
        description: ev.task.description,
        due: ev.task.due,
        status: "COMPLETED",
      });
      setViewItem(null);
      await load();
      await refreshOverview();
      notice.done("Task marked done.");
    } catch (ex) {
      notice.fail(ex instanceof Error ? ex.message : "Could not update task.");
    }
  }

  function openNew(day?: Date, calendarId?: number, time?: string) {
    const cal = writable.find((c) => c.id === calendarId) ?? writable[0];
    if (!cal) {
      notice.fail("Create a calendar you can edit first.");
      return;
    }
    setEventDlg({ calendarId: cal.id, day: day ? ymd(day) : ymd(new Date()), time });
  }

  function openNewTask(day?: Date) {
    const cal = writable[0];
    if (!cal) {
      notice.fail("Create a calendar you can edit first.");
      return;
    }
    setTaskDlg({ item: null, calendarId: cal.id, due: ymd(day ?? new Date()) });
  }

  function startEdit(c: Calendar) {
    setCreating(false);
    setEditCal(c);
    setEditName(c.name);
    setEditColor(c.color || "#E72625");
    setEditDesc(c.description || "");
    setWebcalOn(false);
    setWebcalURL("");
    api
      .webcal(c.id)
      .then((w) => {
        setWebcalOn(!!w.enabled);
        setWebcalURL(w.url || "");
      })
      .catch(() => {
        setWebcalOn(false);
        setWebcalURL("");
      });
  }

  async function addCal(e: FormEvent) {
    e.preventDefault();
    try {
      await api.createCalendar({ name, color, description: desc.trim() });
      setName("");
      setDesc("");
      setCreating(false);
      await load();
      await refreshOverview();
      notice.done("Calendar created.");
    } catch (ex) {
      notice.fail(ex instanceof Error ? ex.message : "Could not create calendar.");
    }
  }

  const title =
    view === "week"
      ? `Week of ${startOfWeek(cursor).toLocaleDateString(undefined, { month: "short", day: "numeric" })}`
      : cursor.toLocaleDateString(undefined, { month: "long", year: "numeric" });

  function shift(dir: number) {
    const next = new Date(cursor);
    if (view === "week") next.setDate(next.getDate() + dir * 7);
    else next.setMonth(next.getMonth() + dir);
    setCursor(next);
  }

  const weekDays = Array.from({ length: 7 }, (_, i) => addDays(startOfWeek(cursor), i));
  const today = new Date();
  const listStart = new Date(cursor.getFullYear(), cursor.getMonth(), 1);
  const listEnd = new Date(cursor.getFullYear(), cursor.getMonth() + 1, 1);
  const monthStart = ymd(listStart);
  const monthEnd = ymd(listEnd);
  const listItems = visibleEvents
    .filter((ev) => eventOverlaps(ev, monthStart, monthEnd))
    .sort((a, b) => a.dtstart.localeCompare(b.dtstart));

  const agenda = useMemo(() => {
    const map = new Map<string, CalEvent[]>();
    for (const ev of listItems) {
      const start = fromICSDate(ev.dtstart);
      const key = !start || start < monthStart ? monthStart : start;
      const arr = map.get(key) ?? [];
      arr.push(ev);
      map.set(key, arr);
    }
    return [...map.entries()].sort(([a], [b]) => a.localeCompare(b));
  }, [listItems, monthStart]);

  async function saveCal(e: FormEvent) {
    e.preventDefault();
    if (!editCal) return;
    try {
      await api.patchCalendar(editCal.id, { name: editName.trim(), color: editColor, description: editDesc });
      setEditCal(null);
      await load();
      notice.done("Calendar updated.");
    } catch (ex) {
      notice.fail(ex instanceof Error ? ex.message : "Could not update calendar.");
    }
  }

  async function removeCal(c: Calendar) {
    if (!window.confirm(`Delete “${c.name}” and its events and tasks?`)) return;
    try {
      await api.deleteCalendar(c.id);
      setEditCal(null);
      await load();
      await refreshOverview();
      notice.done("Calendar deleted.");
    } catch (ex) {
      notice.fail(ex instanceof Error ? ex.message : "Could not delete calendar.");
    }
  }

  async function exportCal(c: Calendar) {
    try {
      await api.exportCalendar(c.id);
    } catch (ex) {
      notice.fail(ex instanceof Error ? ex.message : "Could not export.");
    }
  }

  async function importCal(c: Calendar, files: FileList | null) {
    if (!files?.length) return;
    try {
      let created = 0,
        updated = 0,
        skipped = 0;
      for (const f of Array.from(files)) {
        const part = await api.importCalendar(c.id, await f.text());
        created += part.created;
        updated += part.updated;
        skipped += part.skipped;
      }
      await load();
      notice.done(`Imported: ${created} new, ${updated} updated${skipped ? `, ${skipped} skipped` : ""}.`);
    } catch (ex) {
      notice.fail(ex instanceof Error ? ex.message : "Could not import.");
    }
  }

  async function enableWebcal(c: Calendar) {
    try {
      const w = await api.rotateWebcal(c.id);
      setWebcalOn(true);
      setWebcalURL(w.url || "");
      notice.done("Secret webcal URL created. Anyone with the link can read this calendar.");
    } catch (ex) {
      notice.fail(ex instanceof Error ? ex.message : "Could not create webcal URL.");
    }
  }

  async function disableWebcal(c: Calendar) {
    try {
      await api.deleteWebcal(c.id);
      setWebcalOn(false);
      setWebcalURL("");
      notice.done("Webcal URL turned off. The old link no longer works.");
    } catch (ex) {
      notice.fail(ex instanceof Error ? ex.message : "Could not turn off webcal.");
    }
  }

  function calMeta(c: Calendar) {
    if (c.kind === "important_dates") return "System";
    if (c.shared) return c.access === "write" ? "Can edit" : "View only";
    return "";
  }

  return (
    <>
      <Notices notice={notice} />

      <div className="cal-app">
        <aside className="cal-side">
          <div className="cal-legend">
            <div className="section-label">Calendars</div>
            {shown.length === 0 ? (
              <div className="empty">No calendars yet.</div>
            ) : (
              <div className="cal-filters">
                {shown.map((c) => {
                  const on = visible[c.id] !== false;
                  const meta = calMeta(c);
                  return (
                    <div
                      key={c.id}
                      className={`cal-filter${on ? "" : " off"}${editCal?.id === c.id ? " editing" : ""}`}
                    >
                      <button
                        type="button"
                        className="cal-filter-toggle"
                        aria-pressed={on}
                        onClick={() => setVisible((v) => ({ ...v, [c.id]: v[c.id] === false }))}
                      >
                        <span className="cal-swatch" style={chipStyle(c.color)} />
                        <span className="cal-filter-copy">
                          <span className="cal-filter-name">
                            {c.name}
                            {c.shared ? <span className="muted"> · {c.owner_username}</span> : null}
                          </span>
                          {meta ? <span className="cal-filter-meta">{meta}</span> : null}
                        </span>
                      </button>
                      {calendarOwner(c) && c.kind === "personal" ? (
                        <div className="cal-filter-actions">
                          <button
                            className="btn ghost sm icon-btn"
                            type="button"
                            aria-label={`Edit ${c.name}`}
                            onClick={() => startEdit(c)}
                          >
                            <Pencil size={14} />
                          </button>
                          <button
                            className="btn ghost sm icon-btn"
                            type="button"
                            aria-label={`Share ${c.name}`}
                            onClick={() => setShareCal(c)}
                          >
                            <Share2 size={14} />
                          </button>
                        </div>
                      ) : null}
                    </div>
                  );
                })}
              </div>
            )}

            {editCal ? (
              <form className="cal-edit" onSubmit={saveCal}>
                <div className="row">
                  <div className="field grow">
                    <span>Name</span>
                    <input value={editName} onChange={(e) => setEditName(e.target.value)} required />
                  </div>
                  <div className="field">
                    <span>Color</span>
                    <input type="color" value={editColor} onChange={(e) => setEditColor(e.target.value)} />
                  </div>
                </div>
                <div className="field">
                  <span>Description</span>
                  <input value={editDesc} onChange={(e) => setEditDesc(e.target.value)} />
                </div>
                <div className="form-actions">
                  <button className="btn sm" type="submit">
                    Save
                  </button>
                  <button className="btn ghost sm" type="button" onClick={() => setEditCal(null)}>
                    Cancel
                  </button>
                  {personalOwned.length > 1 ? (
                    <button className="btn danger sm" type="button" onClick={() => removeCal(editCal)}>
                      Delete
                    </button>
                  ) : null}
                </div>
                <div className="form-actions">
                  <button className="btn ghost sm" type="button" onClick={() => exportCal(editCal)}>
                    <Download size={14} aria-hidden />
                    Export ICS
                  </button>
                  <button className="btn ghost sm" type="button" onClick={() => icsFile.current?.click()}>
                    <Upload size={14} aria-hidden />
                    Import ICS
                  </button>
                  <input
                    ref={icsFile}
                    type="file"
                    accept=".ics,text/calendar"
                    hidden
                    onChange={(e) => {
                      importCal(editCal, e.target.files);
                      e.target.value = "";
                    }}
                  />
                  <button className="btn ghost sm" type="button" onClick={() => enableWebcal(editCal)}>
                    <Link2 size={14} aria-hidden />
                    {webcalOn ? "Rotate webcal" : "Webcal URL"}
                  </button>
                  {webcalOn ? (
                    <button className="btn ghost sm" type="button" onClick={() => disableWebcal(editCal)}>
                      <Unlink size={14} aria-hidden />
                      Turn off webcal
                    </button>
                  ) : null}
                </div>
                {webcalURL ? (
                  <p className="muted" style={{ wordBreak: "break-all" }}>
                    {webcalURL}
                  </p>
                ) : webcalOn ? (
                  <p className="muted">Feed is on. Rotate to copy a new URL.</p>
                ) : null}
              </form>
            ) : null}

            {creating ? (
              <form className="cal-edit" onSubmit={addCal}>
                <div className="row">
                  <div className="field grow">
                    <span>Name</span>
                    <input value={name} onChange={(e) => setName(e.target.value)} required autoFocus />
                  </div>
                  <div className="field">
                    <span>Color</span>
                    <input type="color" value={color} onChange={(e) => setColor(e.target.value)} />
                  </div>
                </div>
                <div className="field">
                  <span>Description</span>
                  <input value={desc} onChange={(e) => setDesc(e.target.value)} />
                </div>
                <div className="form-actions">
                  <button className="btn sm" type="submit">
                    Create
                  </button>
                  <button className="btn ghost sm" type="button" onClick={() => setCreating(false)}>
                    Cancel
                  </button>
                </div>
              </form>
            ) : (
              <button
                className="cal-create"
                type="button"
                onClick={() => {
                  setEditCal(null);
                  setCreating(true);
                }}
              >
                <Plus size={14} aria-hidden />
                New calendar
              </button>
            )}
          </div>
        </aside>

        <div className="cal-main">
          <div className="cal-stage">
            <div className="cal-toolbar">
              <div className="cal-nav">
                <button className="btn ghost sm icon-btn" type="button" onClick={() => shift(-1)} aria-label="Previous">
                  <ChevronLeft size={18} />
                </button>
                <button className="btn ghost sm" type="button" onClick={() => setCursor(new Date())}>
                  Today
                </button>
                <button className="btn ghost sm icon-btn" type="button" onClick={() => shift(1)} aria-label="Next">
                  <ChevronRight size={18} />
                </button>
              </div>
              <h1 className="cal-title">{title}</h1>
              <div className="cal-toolbar-end">
                <div className="seg" role="tablist">
                  {VIEWS.map((v) => {
                    const Icon = v.icon;
                    return (
                      <button
                        key={v.id}
                        className={view === v.id ? "on" : ""}
                        type="button"
                        role="tab"
                        aria-selected={view === v.id}
                        onClick={() => setView(v.id)}
                      >
                        <Icon size={14} aria-hidden />
                        <span className="seg-label">{v.label}</span>
                      </button>
                    );
                  })}
                </div>
                <button
                  className="btn sm"
                  type="button"
                  onClick={() => openNew(today)}
                  disabled={writable.length === 0}
                  title={writable.length === 0 ? "Create a calendar you can edit first." : undefined}
                >
                  <Plus size={15} aria-hidden />
                  New event
                </button>
                <button
                  className="btn secondary sm"
                  type="button"
                  onClick={() => openNewTask(today)}
                  disabled={writable.length === 0}
                  title={writable.length === 0 ? "Create a calendar you can edit first." : undefined}
                >
                  <ListTodo size={15} aria-hidden />
                  New task
                </button>
              </div>
            </div>

            {view === "month" ? (
              <div className="cal-month">
                {["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"].map((d) => (
                  <div key={d} className="cal-dow">
                    {d}
                  </div>
                ))}
                {monthGrid(cursor).map((day) => {
                  const items = eventsOn(day);
                  const extra = items.length - 3;
                  const inMonth = day.getMonth() === cursor.getMonth();
                  return (
                    <div
                      key={ymd(day)}
                      role="button"
                      tabIndex={0}
                      className={`cal-cell${inMonth ? "" : " out"}${sameDay(day, today) ? " today" : ""}`}
                      onClick={() => openNew(day)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" || e.key === " ") {
                          e.preventDefault();
                          openNew(day);
                        }
                      }}
                    >
                      <span className="cal-num">{day.getDate()}</span>
                      {items.slice(0, 3).map((ev) => (
                        <button
                          key={itemKey(ev)}
                          type="button"
                          className={chipClass(ev)}
                          style={chipStyle(ev.color)}
                          {...itemPointer(ev)}
                        >
                          {eventChipText(ev, day)}
                        </button>
                      ))}
                      {extra > 0 ? (
                        <button
                          type="button"
                          className="cal-more"
                          onClick={(e) => {
                            e.stopPropagation();
                            setCursor(day);
                            setView("week");
                          }}
                        >
                          {extra} more
                        </button>
                      ) : null}
                    </div>
                  );
                })}
              </div>
            ) : null}

            {view === "week" ? (
              <div className="cal-week-grid">
                <div className="cal-week-head-row">
                  <div className="cal-week-gutter" />
                  {weekDays.map((day) => (
                    <button
                      key={ymd(day)}
                      type="button"
                      className={`cal-week-head${sameDay(day, today) ? " today" : ""}`}
                      onClick={() => openNew(day)}
                    >
                      <span className="cal-week-dow">{day.toLocaleDateString(undefined, { weekday: "short" })}</span>
                      <strong className={sameDay(day, today) ? "cal-num today" : "cal-num"}>{day.getDate()}</strong>
                    </button>
                  ))}
                </div>
                <div className="cal-week-allday-row">
                  <div className="cal-week-gutter">
                    <span>All day</span>
                  </div>
                  {weekDays.map((day) => {
                    const allday = eventsOn(day).filter((ev) => ev.kind === "task" || ev.all_day || !/T\d{2}/.test(ev.dtstart));
                    return (
                      <div key={ymd(day)} className={`cal-week-allday${sameDay(day, today) ? " today" : ""}`}>
                        {allday.map((ev) => (
                          <button
                            key={itemKey(ev)}
                            type="button"
                            className={chipClass(ev)}
                            style={chipStyle(ev.color)}
                            {...itemPointer(ev)}
                          >
                            {ev.summary}
                          </button>
                        ))}
                      </div>
                    );
                  })}
                </div>
                <div className="cal-week-timed-scroll" ref={weekScroll}>
                  <div className="cal-week-timed" style={{ height: WEEK_HOURS * WEEK_HOUR_PX }}>
                    <div className="cal-week-hours">
                      {Array.from({ length: WEEK_HOURS }, (_, h) => (
                        <div key={h} className="cal-week-hour" style={{ height: WEEK_HOUR_PX }}>
                          {h === 0 ? "" : `${String(h).padStart(2, "0")}:00`}
                        </div>
                      ))}
                    </div>
                    {weekDays.map((day) => {
                      const items = eventsOn(day);
                      const blocks = layoutDayTimed(items, day);
                      const isToday = sameDay(day, today);
                      const nowMin = today.getHours() * 60 + today.getMinutes();
                      return (
                        <div
                          key={ymd(day)}
                          className={`cal-week-daycol${isToday ? " today" : ""}`}
                          onClick={(e) => {
                            const rect = e.currentTarget.getBoundingClientRect();
                            openNew(day, undefined, padHHMM(minutesFromClientY(e.clientY, rect.top, rect.height)));
                          }}
                        >
                          {Array.from({ length: WEEK_HOURS }, (_, h) => (
                            <div key={h} className="cal-week-slot" style={{ height: WEEK_HOUR_PX }} />
                          ))}
                          {isToday ? <div className="cal-week-now" style={{ top: (nowMin / (WEEK_HOURS * 60)) * 100 + "%" }} /> : null}
                          {blocks.map((b) => {
                            const top = (b.startMin / (WEEK_HOURS * 60)) * 100;
                            const height = Math.max(((b.endMin - b.startMin) / (WEEK_HOURS * 60)) * 100, 2.2);
                            const width = 100 / b.cols;
                            const left = b.col * width;
                            return (
                              <button
                                key={itemKey(b.item)}
                                type="button"
                                className={chipClass(b.item, "timed")}
                                style={{
                                  ...chipStyle(b.item.color),
                                  top: `${top}%`,
                                  height: `${height}%`,
                                  left: `calc(${left}% + 2px)`,
                                  width: `calc(${width}% - 4px)`,
                                }}
                                {...itemPointer(b.item)}
                              >
                                <strong>{b.item.summary}</strong>
                                <span>{eventTimeLabel(b.item.dtstart)}</span>
                              </button>
                            );
                          })}
                        </div>
                      );
                    })}
                  </div>
                </div>
              </div>
            ) : null}

            {view === "list" ? (
              agenda.length === 0 ? (
                <div className="cal-agenda-empty">No events or tasks this month.</div>
              ) : (
                <div className="cal-agenda">
                  {agenda.map(([key, items]) => {
                    const day = parseYMD(key);
                    const isToday = sameDay(day, today);
                    return (
                      <section key={key} className={`cal-agenda-day${isToday ? " today" : ""}`}>
                        <header>
                          <span className={`cal-num${isToday ? " today" : ""}`}>{day.getDate()}</span>
                          <div>
                            <strong>{day.toLocaleDateString(undefined, { weekday: "long" })}</strong>
                            <span className="muted">{day.toLocaleDateString(undefined, { month: "long" })}</span>
                          </div>
                        </header>
                        <div className="cal-agenda-items">
                          {items.map((ev) => (
                            <button
                              key={itemKey(ev)}
                              type="button"
                              className={`cal-agenda-item${ev.kind === "task" ? " task" : ""}`}
                              {...itemPointer(ev)}
                            >
                              <span className="cal-agenda-time">{itemWhen(ev)}</span>
                              <span className="cal-swatch" style={chipStyle(ev.color)} />
                              <span className="cal-agenda-copy">
                                <strong>{ev.summary || ev.uid}</strong>
                                <span className="muted">
                                  {ev.calendarName}
                                  {ev.location ? ` · ${ev.location}` : ""}
                                </span>
                              </span>
                            </button>
                          ))}
                        </div>
                      </section>
                    );
                  })}
                </div>
              )
            ) : null}
          </div>
        </div>
      </div>

      {viewItem ? (
        <ItemViewDialog
          item={viewItem}
          canWrite={itemCanWrite(viewItem, shown)}
          onClose={() => setViewItem(null)}
          onEdit={() => openEdit(viewItem)}
          onDelete={() => deleteItem(viewItem)}
          onComplete={viewItem.kind === "task" ? () => completeTask(viewItem) : undefined}
        />
      ) : null}
      {menu ? (
        <ItemContextMenu
          x={menu.x}
          y={menu.y}
          item={menu.item}
          canWrite={itemCanWrite(menu.item, shown)}
          onClose={() => setMenu(null)}
          onView={() => openView(menu.item)}
          onEdit={() => openEdit(menu.item)}
          onDelete={() => deleteItem(menu.item)}
          onComplete={menu.item.kind === "task" ? () => completeTask(menu.item) : undefined}
        />
      ) : null}
      {eventDlg ? (
        <EventDialog
          key={
            eventDlg.event
              ? `${eventDlg.calendarId}-${eventDlg.event.href}`
              : `new-${eventDlg.calendarId}-${eventDlg.day || ""}-${eventDlg.time || ""}`
          }
          calendars={shown}
          directory={directory}
          timezone={user.timezone || "UTC"}
          preset={eventDlg}
          onClose={() => setEventDlg(null)}
          onSaved={(result) => {
            setEventDlg(null);
            load().catch(() => undefined);
            refreshOverview();
            if (result?.deleted) notice.done("Event deleted.");
            else if (result?.mailError) notice.fail(`Event saved. Invitations were not emailed: ${result.mailError}`);
            else notice.done("Event saved.");
          }}
        />
      ) : null}
      {taskDlg ? (
        <TaskDialog
          key={taskDlg.item ? `${taskDlg.item.calendar_id}-${taskDlg.item.href}` : `new-${taskDlg.calendarId}-${taskDlg.due || ""}`}
          calendars={shown}
          writable={writable}
          item={taskDlg.item}
          calendarId={taskDlg.calendarId}
          due={taskDlg.due}
          onClose={() => setTaskDlg(null)}
          onSaved={async (msg) => {
            notice.done(msg);
            setTaskDlg(null);
            await load();
            await refreshOverview();
          }}
          onDeleted={async () => {
            notice.done("Task deleted.");
            setTaskDlg(null);
            await load();
            await refreshOverview();
          }}
        />
      ) : null}
      {shareCal ? (
        <ShareDialog
          calendar={shareCal}
          directory={directory}
          onClose={() => setShareCal(null)}
          onChanged={() => load().catch(() => undefined)}
        />
      ) : null}
    </>
  );
}
