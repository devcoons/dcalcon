"use client";

import { useEffect, useMemo, useRef, useState, type CSSProperties } from "react";
import Link from "next/link";
import { CalendarDays, Check, Circle, ListTodo, Pencil, Plus, Search, Trash2 } from "lucide-react";
import { api, type Calendar, type TaskItem } from "@/lib/api";
import { calendarLabel, calendarWritable, dueYMD, parseYMD, ymd } from "@/lib/format";
import { useSession } from "@/lib/shell";
import { TaskDialog } from "@/lib/task-dialog";
import { IconBtn, Notices, PageHeader, useNotice } from "@/lib/ui";

type StatusFilter = "open" | "done" | "all";

function chipStyle(color: string): CSSProperties {
  return { ["--chip" as string]: color || "#E72625" };
}

function formatDueDay(day: string) {
  const d = parseYMD(day);
  const opts: Intl.DateTimeFormatOptions = { month: "short", day: "numeric" };
  if (d.getFullYear() !== new Date().getFullYear()) opts.year = "numeric";
  return d.toLocaleDateString(undefined, opts);
}

export default function TasksPage() {
  const { refreshOverview } = useSession();
  const notice = useNotice();
  const [cals, setCals] = useState<Calendar[]>([]);
  const [items, setItems] = useState<TaskItem[]>([]);
  const [q, setQ] = useState("");
  const [status, setStatus] = useState<StatusFilter>("open");
  const [calFilter, setCalFilter] = useState(0);
  const [dialog, setDialog] = useState<null | { item: TaskItem | null; calendarId: number }>(null);
  const loadGen = useRef(0);
  const today = ymd(new Date());

  const writable = useMemo(() => cals.filter(calendarWritable), [cals]);
  const calById = useMemo(() => new Map(cals.map((c) => [c.id, c])), [cals]);

  async function load() {
    const gen = ++loadGen.current;
    const [list, tasks] = await Promise.all([api.calendars(), api.tasks()]);
    if (gen !== loadGen.current) return;
    const usable = (list ?? []).filter((c) => c.kind !== "inbox" && c.kind !== "outbox");
    setCals(usable);
    setItems(tasks ?? []);
  }

  useEffect(() => {
    load().catch((e) => notice.fail(e instanceof Error ? e.message : "Could not load tasks."));
  }, []);

  const filtered = useMemo(() => {
    const s = q.trim().toLowerCase();
    return items
      .filter((it) => {
        if (calFilter && it.calendar_id !== calFilter) return false;
        const done = it.status === "COMPLETED";
        if (status === "open" && done) return false;
        if (status === "done" && !done) return false;
        if (!s) return true;
        const blob = [it.summary, it.description, it.calendar_name].join(" ").toLowerCase();
        return blob.includes(s);
      })
      .sort((a, b) => {
        const aDone = a.status === "COMPLETED" ? 2 : 0;
        const bDone = b.status === "COMPLETED" ? 2 : 0;
        const aOver = !aDone && dueYMD(a.due) && dueYMD(a.due) < today ? 0 : 1;
        const bOver = !bDone && dueYMD(b.due) && dueYMD(b.due) < today ? 0 : 1;
        const ar = aDone + aOver;
        const br = bDone + bOver;
        if (ar !== br) return ar - br;
        const ad = dueYMD(a.due);
        const bd = dueYMD(b.due);
        if (ad !== bd) {
          if (!ad) return 1;
          if (!bd) return -1;
          return ad.localeCompare(bd);
        }
        return (a.summary || "").localeCompare(b.summary || "");
      });
  }, [items, q, status, calFilter, today]);

  function openNew() {
    if (!writable.length) {
      notice.fail("Create a calendar you can edit first.");
      return;
    }
    const calendarId = writable.find((c) => c.id === calFilter)?.id ?? writable[0].id;
    setDialog({ item: null, calendarId });
  }

  function canWrite(it: TaskItem) {
    const c = calById.get(it.calendar_id);
    return c ? calendarWritable(c) : false;
  }

  async function toggle(it: TaskItem) {
    if (!canWrite(it)) return;
    const next = it.status === "COMPLETED" ? "NEEDS-ACTION" : "COMPLETED";
    try {
      await api.updateTask(it.calendar_id, it.href, {
        summary: it.summary,
        description: it.description,
        due: it.due,
        status: next,
      });
      await load();
      await refreshOverview();
    } catch (ex) {
      notice.fail(ex instanceof Error ? ex.message : "Could not update task.");
    }
  }

  async function remove(it: TaskItem) {
    if (!canWrite(it)) return;
    if (!window.confirm(`Delete “${it.summary}”?`)) return;
    try {
      await api.deleteTask(it.calendar_id, it.href);
      if (dialog?.item?.href === it.href && dialog.item.calendar_id === it.calendar_id) setDialog(null);
      await load();
      await refreshOverview();
      notice.done("Task deleted.");
    } catch (ex) {
      notice.fail(ex instanceof Error ? ex.message : "Could not delete task.");
    }
  }

  const openCount = items.filter((t) => t.status !== "COMPLETED").length;
  const doneCount = items.length - openCount;

  let emptyCopy = "No tasks match.";
  if (items.length === 0) emptyCopy = "";
  else if (q.trim()) emptyCopy = "No tasks match that search.";
  else if (calFilter && status === "open") emptyCopy = "No open tasks on this calendar.";
  else if (calFilter && status === "done") emptyCopy = "No completed tasks on this calendar.";
  else if (calFilter) emptyCopy = "No tasks on this calendar.";
  else if (status === "open") emptyCopy = "No open tasks.";
  else if (status === "done") emptyCopy = "No completed tasks.";

  return (
    <>
      <PageHeader title="Tasks" lede="To-dos on your calendars. They sync to the same phone and desktop apps as events.">
        <div className="btn-row">
          <div className="search-field">
            <Search size={16} aria-hidden />
            <input placeholder="Search" value={q} onChange={(e) => setQ(e.target.value)} aria-label="Search tasks" />
          </div>
          <div className="seg" role="group" aria-label="Task status">
            {(
              [
                ["open", "Open", openCount],
                ["done", "Done", doneCount],
                ["all", "All", items.length],
              ] as const
            ).map(([id, label, n]) => (
              <button key={id} type="button" className={status === id ? "on" : ""} onClick={() => setStatus(id)}>
                {label}
                {items.length ? <span className="muted">{n}</span> : null}
              </button>
            ))}
          </div>
          <button
            className="btn"
            type="button"
            disabled={!writable.length}
            title={!writable.length ? "Create a calendar you can edit first." : undefined}
            onClick={openNew}
          >
            <Plus size={16} aria-hidden />
            Add task
          </button>
        </div>
      </PageHeader>
      {cals.length > 1 ? (
        <div className="chip-list" style={{ marginBottom: "1rem" }}>
          <button type="button" className={`chip${calFilter === 0 ? " on" : ""}`} onClick={() => setCalFilter(0)}>
            All calendars
          </button>
          {cals.map((c) => (
            <button
              key={c.id}
              type="button"
              className={`chip${calFilter === c.id ? " on" : ""}`}
              onClick={() => setCalFilter(c.id)}
            >
              <span className="cal-swatch" style={chipStyle(c.color)} />
              {calendarLabel(c)}
            </button>
          ))}
        </div>
      ) : null}
      <Notices notice={notice} />
      {items.length === 0 ? (
        <div className="empty-hero">
          <span className="stat-icon">
            <ListTodo size={18} aria-hidden />
          </span>
          <h2>No tasks yet</h2>
          <p className="muted">Add one here, or from a calendar app on your phone or computer.</p>
          {writable.length ? (
            <button className="btn sm" type="button" onClick={openNew}>
              <Plus size={15} aria-hidden />
              Add task
            </button>
          ) : (
            <Link className="btn sm" href="/app/calendars">
              <CalendarDays size={15} aria-hidden />
              Open calendar
            </Link>
          )}
        </div>
      ) : filtered.length === 0 ? (
        <div className="empty">{emptyCopy}</div>
      ) : (
        <div className="table-wrap">
          <table className="data">
            <thead>
              <tr>
                <th className="task-mark" />
                <th>Task</th>
                <th>Due</th>
                <th className="hide-narrow">Calendar</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {filtered.map((it) => {
                const done = it.status === "COMPLETED";
                const day = dueYMD(it.due);
                const overdue = Boolean(!done && day && day < today);
                const dueToday = Boolean(!done && day && day === today);
                const cal = calById.get(it.calendar_id);
                const write = canWrite(it);
                return (
                  <tr
                    key={`${it.calendar_id}-${it.href}`}
                    className={`row-link${done ? " task-done" : ""}`}
                    onClick={() => setDialog({ item: it, calendarId: it.calendar_id })}
                  >
                    <td className="task-mark">
                      {write ? (
                        <IconBtn label={done ? "Reopen" : "Mark complete"} onClick={() => toggle(it)}>
                          {done ? <Check size={16} /> : <Circle size={16} />}
                        </IconBtn>
                      ) : null}
                    </td>
                    <td>
                      <div className="task-title">{it.summary || it.uid}</div>
                      {it.description ? <div className="muted">{it.description}</div> : null}
                    </td>
                    <td>
                      {day ? (
                        <span className={overdue ? "pill bad" : dueToday ? "pill warn" : "muted"}>
                          {overdue ? `Overdue · ${formatDueDay(day)}` : dueToday ? "Today" : formatDueDay(day)}
                        </span>
                      ) : (
                        <span className="muted">—</span>
                      )}
                    </td>
                    <td className="hide-narrow">
                      <span className="task-cal">
                        <span className="cal-swatch" style={chipStyle(cal?.color || "")} />
                        {cal ? calendarLabel(cal) : it.calendar_name}
                      </span>
                    </td>
                    <td>
                      <div className="btn-row">
                        <IconBtn label={write ? "Edit" : "View"} onClick={() => setDialog({ item: it, calendarId: it.calendar_id })}>
                          <Pencil size={16} />
                        </IconBtn>
                        {write ? (
                          <IconBtn label="Delete" danger onClick={() => remove(it)}>
                            <Trash2 size={16} />
                          </IconBtn>
                        ) : null}
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
      {dialog ? (
        <TaskDialog
          key={dialog.item ? `${dialog.item.calendar_id}-${dialog.item.href}` : `new-${dialog.calendarId}`}
          calendars={cals}
          writable={writable}
          item={dialog.item}
          calendarId={dialog.calendarId}
          onClose={() => setDialog(null)}
          onSaved={async (msg) => {
            notice.done(msg);
            setDialog(null);
            await load();
            await refreshOverview();
          }}
          onDeleted={async () => {
            notice.done("Task deleted.");
            setDialog(null);
            await load();
            await refreshOverview();
          }}
        />
      ) : null}
    </>
  );
}
