"use client";

import { FormEvent, useEffect, useState } from "react";
import { Share2 } from "lucide-react";
import { api, type Calendar, type CalendarShare, type DirectoryUser } from "@/lib/api";
import { Modal } from "@/lib/ui";

type Props = {
  calendar: Calendar;
  directory: DirectoryUser[];
  onClose: () => void;
  onChanged: () => void;
};

export function ShareDialog({ calendar, directory, onClose, onChanged }: Props) {
  const [shares, setShares] = useState<CalendarShare[]>([]);
  const [username, setUsername] = useState("");
  const [access, setAccess] = useState("read");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  async function load() {
    setShares(await api.calendarShares(calendar.id));
  }

  useEffect(() => {
    load().catch((e) => setErr(e instanceof Error ? e.message : "Could not load sharing."));
  }, [calendar.id]);

  async function add(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr("");
    try {
      setShares(await api.shareCalendar(calendar.id, username.trim(), access));
      setUsername("");
      onChanged();
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : "Could not share calendar.");
    } finally {
      setBusy(false);
    }
  }

  async function remove(userId: number) {
    setErr("");
    try {
      await api.unshareCalendar(calendar.id, userId);
      await load();
      onChanged();
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : "Could not remove access.");
    }
  }

  const available = directory.filter((u) => !shares.some((s) => s.username === u.username));

  return (
    <Modal
      title={`Share ${calendar.name}`}
      lede="Give another person on this server this calendar. View can see events; edit can add and change them."
      icon={<Share2 size={22} aria-hidden="true" />}
      size="md"
      onClose={onClose}
      footer={
        <div className="form-actions">
          <button className="btn secondary sm" type="button" onClick={onClose}>
            Done
          </button>
        </div>
      }
    >
      <div className="modal-form">
        {err ? <div className="banner err">{err}</div> : null}
        {available.length === 0 ? (
          <p className="muted">Everyone on this server already has access.</p>
        ) : (
          <form className="row" onSubmit={add}>
            <div className="field grow">
              <span>Person</span>
              <select value={username} onChange={(e) => setUsername(e.target.value)} required>
                <option value="">Select a user</option>
                {available.map((u) => (
                  <option key={u.id} value={u.username}>
                    {u.display_name ? `${u.display_name} (${u.username})` : u.username}
                  </option>
                ))}
              </select>
            </div>
            <div className="field">
              <span>Access</span>
              <select value={access} onChange={(e) => setAccess(e.target.value)}>
                <option value="read">Can view</option>
                <option value="write">Can edit</option>
              </select>
            </div>
            <div className="field">
              <span>Add</span>
              <button className="btn" type="submit" disabled={busy || !username}>
                Share
              </button>
            </div>
          </form>
        )}
        {shares.length === 0 ? (
          <div className="empty">Not shared with anyone yet.</div>
        ) : (
          <table className="data">
            <thead>
              <tr>
                <th>User</th>
                <th>Access</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {shares.map((s) => (
                <tr key={s.user_id}>
                  <td>
                    <strong>{s.display_name || s.username}</strong>
                    <div className="muted">{s.username}</div>
                  </td>
                  <td>{s.access === "write" ? "Can edit" : "Can view"}</td>
                  <td>
                    <button className="btn ghost sm" type="button" onClick={() => remove(s.user_id)}>
                      Remove
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </Modal>
  );
}
