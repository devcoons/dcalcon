"use client";

import { FormEvent, useEffect, useState } from "react";
import Link from "next/link";
import { CalendarDays, MapPin, Trash2, X } from "lucide-react";
import { api, type BusyPeriod, type Calendar, type DirectoryUser, type EventItem, type EventWrite } from "@/lib/api";
import { AttachmentEditor, saveAttachments } from "@/lib/attachments";
import { calendarLabel, calendarWritable, displayIdentity, fromICS, fromICSDate, partstatLabel, toICS, toICSDate, addDaysYMD } from "@/lib/format";
import { parseRRule, rruleForSave } from "@/lib/rrule";
import { addHourHHMM } from "@/lib/week-grid";
import { Modal } from "@/lib/ui";

type Props = {
  calendars: Calendar[];
  directory: DirectoryUser[];
  timezone: string;
  preset?: { calendarId: number; day?: string; time?: string; event?: EventItem };
  onClose: () => void;
  onSaved: (result?: { mailError?: string; deleted?: boolean }) => void;
};

export function EventDialog({ calendars, directory, timezone, preset, onClose, onSaved }: Props) {
  const writable = calendars.filter(calendarWritable);
  const editing = preset?.event;
  const initialCal = preset?.calendarId ?? writable[0]?.id ?? calendars[0]?.id ?? 0;
  const [calId, setCalId] = useState(initialCal);
  const [summary, setSummary] = useState(editing?.summary ?? "");
  const [location, setLocation] = useState(editing?.location ?? "");
  const [description, setDescription] = useState(editing?.description ?? "");
  const [allDay, setAllDay] = useState(Boolean(editing?.all_day));
  const [start, setStart] = useState(() => {
    if (editing?.all_day) return fromICSDate(editing.dtstart);
    if (editing) return fromICS(editing.dtstart);
    if (preset?.day) {
      const t = preset.time || "09:00";
      return `${preset.day}T${t}`;
    }
    return "";
  });
  const [end, setEnd] = useState(() => {
    if (editing?.all_day) {
      const endDay = fromICSDate(editing.dtend);
      return endDay ? addDaysYMD(endDay, -1) : fromICSDate(editing.dtstart);
    }
    if (editing) return fromICS(editing.dtend);
    if (preset?.day) {
      const t = preset.time || "09:00";
      return `${preset.day}T${addHourHHMM(t)}`;
    }
    return "";
  });
  const [invite, setInvite] = useState<string[]>([]);
  const [emails, setEmails] = useState<string[]>([]);
  const [emailDraft, setEmailDraft] = useState("");
  const [guestQ, setGuestQ] = useState("");
  const [mailHint, setMailHint] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const parsed = parseRRule(editing?.rrule);
  const [freq, setFreq] = useState(parsed.freq);
  const [interval, setInterval] = useState(parsed.interval);
  const [until, setUntil] = useState(parsed.until);
  const [count, setCount] = useState(parsed.count);
  const [alarm, setAlarm] = useState(editing?.alarm_minutes ?? 0);
  const [busyMap, setBusyMap] = useState<Record<string, BusyPeriod[]>>({});
  const [pendingFiles, setPendingFiles] = useState<File[]>([]);
  const [removedFiles, setRemovedFiles] = useState<string[]>([]);

  useEffect(() => {
    Promise.all([api.mailStatus(), api.accounts()])
      .then(([m, a]) => {
        if (a.length === 0 && !m.server_smtp) {
          setMailHint("Connect a mailbox in Settings to email people outside this server.");
        }
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    if (invite.length === 0 || !start) {
      setBusyMap({});
      return;
    }
    const s = allDay ? toICSDate(start) : toICS(start);
    const e = allDay ? toICSDate(end || start) : toICS(end || start);
    let cancelled = false;
    api
      .freebusy(invite, s, e)
      .then((m) => {
        if (!cancelled) setBusyMap(m);
      })
      .catch(() => {
        if (!cancelled) setBusyMap({});
      });
    return () => {
      cancelled = true;
    };
  }, [invite, start, end, allDay]);

  function toggleInvite(name: string) {
    setInvite((cur) => (cur.includes(name) ? cur.filter((x) => x !== name) : [...cur, name]));
  }

  function addEmail(raw = emailDraft) {
    const value = raw.trim().replace(/,$/, "");
    if (!value) return;
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value)) {
      setErr("Enter a valid email address.");
      return;
    }
    setEmails((cur) => (cur.some((x) => x.toLowerCase() === value.toLowerCase()) ? cur : [...cur, value]));
    setEmailDraft("");
  }

  async function save(e: FormEvent) {
    e.preventDefault();
    const cal = calendars.find((c) => c.id === calId);
    if (!cal || !calendarWritable(cal)) {
      setErr("Choose a calendar you can edit.");
      return;
    }
    let inviteEmails = emails;
    if (emailDraft.trim()) {
      const value = emailDraft.trim().replace(/,$/, "");
      if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value)) {
        setErr("Enter a valid email address.");
        return;
      }
      if (!inviteEmails.some((e) => e.toLowerCase() === value.toLowerCase())) {
        inviteEmails = [...inviteEmails, value];
      }
      setEmails(inviteEmails);
      setEmailDraft("");
    }
    setBusy(true);
    setErr("");
    try {
      const body: EventWrite = {
        summary: summary.trim(),
        description,
        location,
        all_day: allDay,
        dtstart: allDay ? toICSDate(start) : toICS(start, /Z$/i.test(editing?.dtstart || "") || !editing),
        dtend: allDay ? toICSDate(end) : end ? toICS(end, /Z$/i.test(editing?.dtend || editing?.dtstart || "") || !editing) : "",
        rrule: rruleForSave(editing?.rrule, freq, interval, until, count),
        invite,
        invite_emails: inviteEmails,
      };
      if (!editing || alarm !== (editing.alarm_minutes ?? 0)) {
        body.alarm_minutes = alarm;
      }
      const out = editing ? await api.updateEvent(cal.id, editing.href, body) : await api.createEvent(cal.id, body);
      const href = editing?.href || ("href" in out ? out.href : "");
      if (href) await saveAttachments("event", cal.id, href, pendingFiles, removedFiles);
      const mailErr = "invite" in out ? out.invite?.mail_error : undefined;
      onSaved(mailErr ? { mailError: mailErr } : undefined);
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : "Could not save event.");
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    if (!editing) return;
    if (!window.confirm(`Delete “${editing.summary || "this event"}”?`)) return;
    setBusy(true);
    try {
      await api.deleteEvent(calId, editing.href);
      onSaved({ deleted: true });
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : "Could not delete event.");
      setBusy(false);
    }
  }

  const canEdit = calendarWritable(calendars.find((c) => c.id === calId) ?? writable[0] ?? calendars[0] ?? { read_only: true });
  const existingAttendees = editing?.attendees ?? [];

  return (
    <Modal
      title={editing ? "Event" : "New event"}
      lede={timezone ? `Times use this device’s clock · ${timezone}` : "Times use this device’s clock."}
      icon={<CalendarDays size={22} aria-hidden="true" />}
      size="md"
      onClose={onClose}
      footer={
        <div className="form-actions">
          {canEdit ? (
            <button className="btn" form="event-form" type="submit" disabled={busy}>
              {busy ? "Saving…" : editing ? "Save event" : "Create event"}
            </button>
          ) : (
            <p className="muted">This calendar is read-only.</p>
          )}
          {editing && canEdit ? (
            <button className="btn danger sm" type="button" onClick={remove} disabled={busy}>
              <Trash2 size={14} aria-hidden />
              Delete
            </button>
          ) : null}
          <button className="btn secondary sm" type="button" onClick={onClose}>
            Close
          </button>
        </div>
      }
    >
          <form id="event-form" className="modal-form" onSubmit={save}>
            {err ? <div className="banner err">{err}</div> : null}
            <div className="field">
              <span>Title</span>
              <input value={summary} onChange={(e) => setSummary(e.target.value)} required autoFocus disabled={!canEdit} />
            </div>
            <div className="row">
              <div className="field grow">
                <span>Calendar</span>
                <select value={calId} onChange={(e) => setCalId(Number(e.target.value))} disabled={Boolean(editing) || !canEdit}>
                  {(editing ? calendars : writable).map((c) => (
                    <option key={c.id} value={c.id}>
                      {calendarLabel(c)}
                    </option>
                  ))}
                </select>
              </div>
            </div>
            <label className="check">
              <input
                type="checkbox"
                checked={allDay}
                disabled={!canEdit}
                onChange={(e) => {
                  const on = e.target.checked;
                  setAllDay(on);
                  if (on) {
                    setStart(start.slice(0, 10));
                    setEnd((end || start).slice(0, 10));
                  } else {
                    setStart(start.length <= 10 ? `${start}T09:00` : start);
                    setEnd((end || start).length <= 10 ? `${end || start}T10:00` : end);
                  }
                }}
              />
              All day
            </label>
            <div className="row">
              <div className="field">
                <span>Starts</span>
                <input
                  type={allDay ? "date" : "datetime-local"}
                  value={allDay ? start.slice(0, 10) : start}
                  onChange={(e) => setStart(e.target.value)}
                  required
                  disabled={!canEdit}
                />
              </div>
              <div className="field">
                <span>Ends</span>
                <input
                  type={allDay ? "date" : "datetime-local"}
                  value={allDay ? end.slice(0, 10) : end}
                  onChange={(e) => setEnd(e.target.value)}
                  disabled={!canEdit}
                />
              </div>
            </div>
            <div className="field">
              <span>Location</span>
              <div className="input-icon">
                <span className="input-icon-mark">
                  <MapPin size={16} aria-hidden />
                </span>
                <input value={location} onChange={(e) => setLocation(e.target.value)} disabled={!canEdit} />
              </div>
            </div>
            <div className="field">
              <span>Notes</span>
              <textarea value={description} onChange={(e) => setDescription(e.target.value)} disabled={!canEdit} />
            </div>
            <AttachmentEditor
              saved={(editing?.attachments ?? []).filter((a) => !removedFiles.includes(a.id))}
              pending={pendingFiles}
              canEdit={canEdit}
              onPending={setPendingFiles}
              onRemoveSaved={(id) => setRemovedFiles((cur) => (cur.includes(id) ? cur : [...cur, id]))}
              onRemovePending={(i) => setPendingFiles((cur) => cur.filter((_, idx) => idx !== i))}
              onDownload={(a) => api.downloadAttachment(calId, a.id, a.filename)}
            />
            <div className="row">
              <div className="field">
                <span>Repeat</span>
                <select value={freq} onChange={(e) => setFreq(e.target.value)} disabled={!canEdit}>
                  <option value="">Does not repeat</option>
                  <option value="DAILY">Daily</option>
                  <option value="WEEKLY">Weekly</option>
                  <option value="MONTHLY">Monthly</option>
                  <option value="YEARLY">Yearly</option>
                </select>
              </div>
              {freq ? (
                <div className="field">
                  <span>Every</span>
                  <input
                    type="number"
                    min={1}
                    value={interval}
                    onChange={(e) => setInterval(Math.max(1, Number(e.target.value) || 1))}
                    disabled={!canEdit}
                  />
                </div>
              ) : null}
              <div className="field">
                <span>Reminder</span>
                <select value={alarm} onChange={(e) => setAlarm(Number(e.target.value))} disabled={!canEdit}>
                  <option value={0}>None</option>
                  <option value={5}>5 minutes before</option>
                  <option value={10}>10 minutes before</option>
                  <option value={15}>15 minutes before</option>
                  <option value={30}>30 minutes before</option>
                  <option value={60}>1 hour before</option>
                  <option value={1440}>1 day before</option>
                </select>
              </div>
            </div>
            {freq ? (
              <div className="row">
                <div className="field">
                  <span>Until</span>
                  <input type="date" value={until} onChange={(e) => setUntil(e.target.value)} disabled={!canEdit} />
                </div>
                <div className="field">
                  <span>Times</span>
                  <input
                    type="number"
                    min={1}
                    value={count}
                    placeholder="Unlimited"
                    onChange={(e) => setCount(e.target.value)}
                    disabled={!canEdit}
                  />
                </div>
              </div>
            ) : null}
            {canEdit ? (
              <div className="guest-box">
                <span>Guests</span>
                {existingAttendees.length ? (
                  <div className="chip-list">
                    {existingAttendees.map((a) => {
                      const st = partstatLabel(a.partstat);
                      return (
                        <span key={a.value} className="chip on">
                          {a.cn || displayIdentity(a.value)}
                          {st ? <span className="muted"> · {st}</span> : null}
                        </span>
                      );
                    })}
                  </div>
                ) : null}
                {directory.length > 8 ? (
                  <input
                    value={guestQ}
                    onChange={(e) => setGuestQ(e.target.value)}
                    placeholder="Search people on this server"
                    aria-label="Search people"
                  />
                ) : null}
                {directory.length > 0 ? (
                  <div className="chip-list">
                    {directory
                      .filter((u) => {
                        const q = guestQ.trim().toLowerCase();
                        if (directory.length > 8 && !q) return invite.includes(u.username);
                        if (!q) return true;
                        return (
                          u.username.toLowerCase().includes(q) ||
                          (u.display_name || "").toLowerCase().includes(q) ||
                          (u.local_email || "").toLowerCase().includes(q)
                        );
                      })
                      .map((u) => (
                      <label key={u.id} className={`chip${invite.includes(u.username) ? " on" : ""}`}>
                        <input
                          type="checkbox"
                          checked={invite.includes(u.username)}
                          onChange={() => toggleInvite(u.username)}
                        />
                        {u.display_name || u.username}
                      </label>
                    ))}
                  </div>
                ) : null}
                <div className="chip-list">
                  {emails.map((e) => (
                    <button
                      key={e}
                      type="button"
                      className="chip on dismiss"
                      onClick={() => setEmails((cur) => cur.filter((x) => x !== e))}
                    >
                      {e}
                      <X size={12} aria-hidden />
                    </button>
                  ))}
                </div>
                <input
                  type="email"
                  value={emailDraft}
                  placeholder="Add an email and press Enter"
                  onChange={(e) => setEmailDraft(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === ",") {
                      e.preventDefault();
                      addEmail();
                    }
                  }}
                  onBlur={() => addEmail()}
                />
                {mailHint ? (
                  <p className="muted">
                    {mailHint} <Link href="/app/settings">Open Settings</Link>
                  </p>
                ) : (
                  <p className="muted">People on this server get an invitation here. Others get an email if a mailbox is connected.</p>
                )}
                {busyHint(invite, busyMap) ? <p className="muted">{busyHint(invite, busyMap)}</p> : null}
              </div>
            ) : existingAttendees.length ? (
              <div className="guest-box">
                <span>Guests</span>
                <div className="chip-list">
                  {existingAttendees.map((a) => {
                    const st = partstatLabel(a.partstat);
                    return (
                      <span key={a.value} className="chip">
                        {a.cn || displayIdentity(a.value)}
                        {st ? <span className="muted"> · {st}</span> : null}
                      </span>
                    );
                  })}
                </div>
              </div>
            ) : null}
          </form>
    </Modal>
  );
}

function busyHint(users: string[], map: Record<string, BusyPeriod[]>) {
  const busy = users.filter((u) => (map[u] || []).length > 0);
  if (!busy.length) return "";
  return `${busy.join(", ")} look busy in this window.`;
}
