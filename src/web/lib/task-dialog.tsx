"use client";

import { FormEvent, useState } from "react";
import { ListTodo, Trash2 } from "lucide-react";
import { api, type Calendar, type TaskItem } from "@/lib/api";
import { AttachmentEditor, saveAttachments } from "@/lib/attachments";
import { calendarLabel, calendarWritable, dueYMD } from "@/lib/format";
import { Modal } from "@/lib/ui";

type Props = {
  calendars: Calendar[];
  writable: Calendar[];
  item: TaskItem | null;
  calendarId: number;
  due?: string;
  onClose: () => void;
  onSaved: (msg: string) => void | Promise<void>;
  onDeleted: () => void | Promise<void>;
};

export function TaskDialog({ calendars, writable, item, calendarId, due: duePreset, onClose, onSaved, onDeleted }: Props) {
  const editing = Boolean(item);
  const cal = calendars.find((c) => c.id === (item?.calendar_id ?? calendarId));
  const canEdit = cal ? calendarWritable(cal) : writable.length > 0;
  const [calId, setCalId] = useState(item?.calendar_id ?? calendarId);
  const [summary, setSummary] = useState(item?.summary ?? "");
  const [due, setDue] = useState(dueYMD(item?.due ?? "") || duePreset || "");
  const [description, setDescription] = useState(item?.description ?? "");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [pendingFiles, setPendingFiles] = useState<File[]>([]);
  const [removedFiles, setRemovedFiles] = useState<string[]>([]);

  async function save(e: FormEvent) {
    e.preventDefault();
    if (!canEdit) return;
    const title = summary.trim();
    if (!title) {
      setErr("Title is required.");
      return;
    }
    setBusy(true);
    setErr("");
    try {
      if (item) {
        await api.updateTask(item.calendar_id, item.href, {
          summary: title,
          description,
          due,
          status: item.status,
        });
        await saveAttachments("task", item.calendar_id, item.href, pendingFiles, removedFiles);
        await onSaved("Task updated.");
      } else {
        const created = await api.createTask(calId, { summary: title, description, due, status: "NEEDS-ACTION" });
        if (created.href) await saveAttachments("task", calId, created.href, pendingFiles, removedFiles);
        await onSaved("Task added.");
      }
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : "Could not save task.");
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    if (!item || !canEdit) return;
    if (!window.confirm(`Delete “${item.summary}”?`)) return;
    setBusy(true);
    setErr("");
    try {
      await api.deleteTask(item.calendar_id, item.href);
      await onDeleted();
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : "Could not delete task.");
      setBusy(false);
    }
  }

  return (
    <Modal
      title={editing ? "Task" : "New task"}
      lede={canEdit ? (editing ? "Edit the title, due date, and notes." : "Saved on the calendar you pick.") : "This calendar is read-only."}
      icon={<ListTodo size={22} aria-hidden="true" />}
      size="md"
      onClose={onClose}
      footer={
        <div className="form-actions">
          {canEdit ? (
            <button className="btn" form="task-form" type="submit" disabled={busy}>
              {busy ? "Saving…" : editing ? "Save task" : "Add task"}
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
      <form id="task-form" className="modal-form" onSubmit={save}>
        {err ? <div className="banner err">{err}</div> : null}
        <div className="field">
          <span>Title</span>
          <input value={summary} onChange={(e) => setSummary(e.target.value)} required autoFocus disabled={!canEdit} />
        </div>
        <div className="row">
          <div className="field grow">
            <span>Calendar</span>
            {editing ? (
              <input readOnly value={cal ? calendarLabel(cal) : item?.calendar_name || ""} />
            ) : (
              <select value={calId} onChange={(e) => setCalId(Number(e.target.value))} disabled={!canEdit}>
                {writable.map((c) => (
                  <option key={c.id} value={c.id}>
                    {calendarLabel(c)}
                  </option>
                ))}
              </select>
            )}
          </div>
          <div className="field">
            <span>Due</span>
            <input type="date" value={due} onChange={(e) => setDue(e.target.value)} disabled={!canEdit} />
          </div>
        </div>
        <div className="field">
          <span>Notes</span>
          <textarea value={description} onChange={(e) => setDescription(e.target.value)} rows={3} disabled={!canEdit} />
        </div>
        <AttachmentEditor
          saved={(item?.attachments ?? []).filter((a) => !removedFiles.includes(a.id))}
          pending={pendingFiles}
          canEdit={canEdit}
          onPending={setPendingFiles}
          onRemoveSaved={(id) => setRemovedFiles((cur) => (cur.includes(id) ? cur : [...cur, id]))}
          onRemovePending={(i) => setPendingFiles((cur) => cur.filter((_, idx) => idx !== i))}
          onDownload={(a) => api.downloadAttachment(item?.calendar_id ?? calId, a.id, a.filename)}
        />
      </form>
    </Modal>
  );
}
