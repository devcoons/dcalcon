"use client";

import { FormEvent, useEffect, useRef, useState } from "react";
import { Archive, Cake, CalendarDays, Download, KeyRound, Shield, Upload } from "lucide-react";
import { api, type ImportantDates, type Setup } from "@/lib/api";
import { MailPanel } from "@/lib/mail-panel";
import { CopyField, Fold, Notices, PageHeader, useNotice } from "@/lib/ui";
import { useSession } from "@/lib/shell";

export default function SettingsPage() {
  const { refreshOverview } = useSession();
  const notice = useNotice();
  const [setup, setSetup] = useState<Setup | null>(null);
  const [saving, setSaving] = useState(false);
  const [dates, setDates] = useState<ImportantDates>({
    enabled: false,
    include_birthdays: true,
    include_anniversaries: true,
    alarm_offsets: ["-P1D"],
  });
  const [alarmDay, setAlarmDay] = useState(true);
  const [alarmWeek, setAlarmWeek] = useState(false);

  useEffect(() => {
    Promise.all([api.setup(), api.importantDates()])
      .then(([s, d]) => {
        setSetup(s);
        setDates({
          ...d,
          alarm_offsets: d.alarm_offsets ?? ["-P1D"],
        });
        const offsets = d.alarm_offsets ?? [];
        setAlarmDay(offsets.includes("-P1D") || offsets.includes("P1D"));
        setAlarmWeek(offsets.includes("-P7D"));
      })
      .catch((e) => notice.failFrom(e, "Could not load settings."));
  }, []);

  async function saveDates(e: FormEvent) {
    e.preventDefault();
    const offsets: string[] = [];
    if (alarmDay) offsets.push("-P1D");
    if (alarmWeek) offsets.push("-P7D");
    const nextDates = { ...dates, alarm_offsets: offsets.length ? offsets : ["-P1D"] };
    setSaving(true);
    try {
      await api.saveImportantDates(nextDates);
      setDates(nextDates);
      await refreshOverview();
      notice.done("Important Dates saved. The worker rebuilds the calendar on its next pass.");
    } catch (ex) {
      notice.failFrom(ex, "Could not save Important Dates.");
    } finally {
      setSaving(false);
    }
  }

  return (
    <>
      <PageHeader title="Settings" lede="Client URLs, backups, Important Dates, and the mailbox used for external invitations." />
      <Notices notice={notice} />
      <div className="page-grid">
        <section className="home-panel">
          <div className="panel-head">
            <span className="stat-icon">
              <KeyRound size={16} aria-hidden />
            </span>
            <div>
              <h2>Calendar and contacts apps</h2>
              <p className="muted">
                In DAVx⁵ or GNOME, add an account for this host. Use HTTP Basic with an app password from Account.
              </p>
            </div>
          </div>
          {setup ? (
            <>
              <CopyField label="CalDAV discovery" value={setup.caldav_well_known} />
              <CopyField label="CardDAV discovery" value={setup.carddav_well_known} />
              <CopyField label="Username" value={setup.username} />
              {setup.scheduling_address ? (
                <CopyField label="Address on this server" value={setup.scheduling_address} />
              ) : null}
              <p className="muted">
                Auth: {setup.auth_method}. Other people invite you from DAVx⁵, Thunderbird, or Apple Calendar using the
                address above (enable the “People on this server” address book).
              </p>
              <Fold title="Principal and home URLs">
                <CopyField label="Principal" value={setup.principal_url} />
                <CopyField label="Calendar home" value={setup.calendar_home} />
                <CopyField label="Address book home" value={setup.addressbook_home} />
              </Fold>
            </>
          ) : (
            <p className="muted">Loading connection details…</p>
          )}
        </section>

        <div className="home-side">
          <form className="home-panel" onSubmit={saveDates}>
            <div className="panel-head">
              <span className="stat-icon">
                <Cake size={16} aria-hidden />
              </span>
              <div>
                <h2>Important Dates</h2>
                <p className="muted">A read-only calendar generated from contact birthdays and anniversaries.</p>
              </div>
            </div>
            <div className="chip-list">
              <label className={`chip ${dates.enabled ? "on" : ""}`}>
                <input
                  type="checkbox"
                  checked={dates.enabled}
                  onChange={(e) => setDates({ ...dates, enabled: e.target.checked })}
                />
                Enable calendar
              </label>
              <label className={`chip ${dates.include_birthdays ? "on" : ""}`}>
                <input
                  type="checkbox"
                  checked={dates.include_birthdays}
                  onChange={(e) => setDates({ ...dates, include_birthdays: e.target.checked })}
                />
                Birthdays
              </label>
              <label className={`chip ${dates.include_anniversaries ? "on" : ""}`}>
                <input
                  type="checkbox"
                  checked={dates.include_anniversaries}
                  onChange={(e) => setDates({ ...dates, include_anniversaries: e.target.checked })}
                />
                Anniversaries
              </label>
              <label className={`chip ${alarmDay ? "on" : ""}`}>
                <input type="checkbox" checked={alarmDay} onChange={(e) => setAlarmDay(e.target.checked)} />
                1 day before
              </label>
              <label className={`chip ${alarmWeek ? "on" : ""}`}>
                <input type="checkbox" checked={alarmWeek} onChange={(e) => setAlarmWeek(e.target.checked)} />
                1 week before
              </label>
            </div>
            <div className="form-actions">
              <button className="btn" type="submit" disabled={saving}>
                <CalendarDays size={15} aria-hidden />
                {saving ? "Saving…" : "Save"}
              </button>
            </div>
          </form>
          <MailPanel />
        </div>

        <BackupPanel onNotice={notice} />
      </div>
    </>
  );
}

function BackupPanel({ onNotice }: { onNotice: ReturnType<typeof useNotice> }) {
  const { refreshOverview } = useSession();
  const dataFile = useRef<HTMLInputElement>(null);
  const fullFile = useRef<HTMLInputElement>(null);
  const [fullExportPass, setFullExportPass] = useState("");
  const [fullImportPass, setFullImportPass] = useState("");
  const [busy, setBusy] = useState("");

  async function downloadData() {
    setBusy("data-dl");
    try {
      await api.exportBackup("data");
      onNotice.done("Data backup downloaded.");
    } catch (ex) {
      onNotice.failFrom(ex, "Could not download data backup.");
    } finally {
      setBusy("");
    }
  }

  async function downloadFull() {
    if (!fullExportPass) {
      onNotice.fail("Enter your current password to download a full backup.");
      return;
    }
    setBusy("full-dl");
    try {
      await api.exportBackup("full", fullExportPass);
      onNotice.done("Full backup downloaded. Keep this zip as secret as a password file.");
    } catch (ex) {
      onNotice.failFrom(ex, "Could not download full backup.");
    } finally {
      setBusy("");
    }
  }

  async function restoreData(file: File) {
    if (!window.confirm("This replaces events, tasks, and contacts in matching calendars and address books. Continue?")) {
      return;
    }
    setBusy("data-up");
    try {
      const res = await api.restoreBackup(file);
      await refreshOverview();
      onNotice.done(`Restored ${res.objects} calendar items and ${res.contacts} contacts.`);
    } catch (ex) {
      onNotice.failFrom(ex, "Could not restore data backup.");
    } finally {
      setBusy("");
      if (dataFile.current) dataFile.current.value = "";
    }
  }

  async function restoreFull(file: File) {
    if (!fullImportPass) {
      onNotice.fail("Enter your current password to restore a full backup.");
      return;
    }
    if (
      !window.confirm(
        "This restores calendars, contacts, your password, authenticator, app passwords, and mail tokens. Continue?",
      )
    ) {
      return;
    }
    setBusy("full-up");
    try {
      const res = await api.restoreBackup(file, fullImportPass);
      await refreshOverview();
      onNotice.done(`Full restore finished (${res.objects} calendar items, ${res.contacts} contacts).`);
    } catch (ex) {
      onNotice.failFrom(ex, "Could not restore full backup.");
    } finally {
      setBusy("");
      if (fullFile.current) fullFile.current.value = "";
    }
  }

  return (
    <>
      <section className="home-panel">
        <div className="panel-head">
          <span className="stat-icon">
            <Archive size={16} aria-hidden />
          </span>
          <div>
            <h2>Data backup</h2>
            <p className="muted">
              Calendars, tasks, contacts, and attached files. Extra calendars you added after the backup are kept.
            </p>
          </div>
        </div>
        <div className="form-actions">
          <button className="btn secondary sm" type="button" onClick={downloadData} disabled={!!busy}>
            <Download size={15} aria-hidden />
            {busy === "data-dl" ? "Preparing…" : "Download zip"}
          </button>
          <button
            className="btn secondary sm"
            type="button"
            disabled={!!busy}
            onClick={() => dataFile.current?.click()}
          >
            <Upload size={15} aria-hidden />
            {busy === "data-up" ? "Restoring…" : "Restore zip"}
          </button>
          <input
            ref={dataFile}
            type="file"
            accept=".zip,application/zip"
            hidden
            onChange={(e) => {
              const f = e.target.files?.[0];
              if (f) void restoreData(f);
            }}
          />
        </div>
      </section>
      <section className="home-panel">
        <div className="panel-head">
          <span className="stat-icon">
            <Shield size={16} aria-hidden />
          </span>
          <div>
            <h2>Full backup</h2>
            <p className="muted">
              Includes sign-in secrets. Treat the zip like a password file. Mail tokens only work on a server with the
              same encryption key.
            </p>
          </div>
        </div>
        <div className="field">
          <span>Current password (download)</span>
          <input
            type="password"
            autoComplete="current-password"
            value={fullExportPass}
            onChange={(e) => setFullExportPass(e.target.value)}
          />
        </div>
        <div className="form-actions">
          <button className="btn secondary sm" type="button" onClick={downloadFull} disabled={!!busy}>
            <Download size={15} aria-hidden />
            {busy === "full-dl" ? "Preparing…" : "Download zip"}
          </button>
        </div>
        <div className="field">
          <span>Current password (restore)</span>
          <input
            type="password"
            autoComplete="current-password"
            value={fullImportPass}
            onChange={(e) => setFullImportPass(e.target.value)}
          />
        </div>
        <div className="form-actions">
          <button
            className="btn secondary sm"
            type="button"
            disabled={!!busy}
            onClick={() => fullFile.current?.click()}
          >
            <Upload size={15} aria-hidden />
            {busy === "full-up" ? "Restoring…" : "Restore zip"}
          </button>
          <input
            ref={fullFile}
            type="file"
            accept=".zip,application/zip"
            hidden
            onChange={(e) => {
              const f = e.target.files?.[0];
              if (f) void restoreFull(f);
            }}
          />
        </div>
      </section>
    </>
  );
}
