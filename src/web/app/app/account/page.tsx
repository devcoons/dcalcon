"use client";

import { FormEvent, useEffect, useState } from "react";
import { Download, KeyRound, ShieldOff, UserRound } from "lucide-react";
import Link from "next/link";
import { api } from "@/lib/api";
import { TIMEZONES } from "@/lib/format";
import { useSession } from "@/lib/shell";
import { Notices, PageHeader, useNotice } from "@/lib/ui";
import { TotpPanel } from "@/lib/totp-panel";
import { AppPasswordsPanel } from "@/lib/app-passwords";

export default function AccountPage() {
  const { user, setUser } = useSession();
  const notice = useNotice();
  const [display, setDisplay] = useState(user.display_name);
  const [email, setEmail] = useState(user.email);
  const [tz, setTz] = useState(user.timezone || "UTC");
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [next2, setNext2] = useState("");
  const [savingProfile, setSavingProfile] = useState(false);
  const [savingPass, setSavingPass] = useState(false);

  useEffect(() => {
    setDisplay(user.display_name);
    setEmail(user.email);
    setTz(user.timezone || "UTC");
  }, [user]);

  async function saveProfile(e: FormEvent) {
    e.preventDefault();
    setSavingProfile(true);
    try {
      const u = await api.patchMe({ display_name: display, email, timezone: tz });
      setUser(u);
      notice.done("Profile saved.");
    } catch (ex) {
      notice.fail(ex instanceof Error ? ex.message : "Could not save profile.");
    } finally {
      setSavingProfile(false);
    }
  }

  async function savePassword(e: FormEvent) {
    e.preventDefault();
    if (next !== next2) {
      notice.fail("New passwords do not match.");
      return;
    }
    setSavingPass(true);
    try {
      await api.changePassword(current, next);
      setCurrent("");
      setNext("");
      setNext2("");
      notice.done("Password updated. DAV clients that used this password must be updated.");
    } catch (ex) {
      notice.fail(ex instanceof Error ? ex.message : "Could not change password.");
    } finally {
      setSavingPass(false);
    }
  }

  return (
    <>
      <PageHeader title="Account" lede="Your profile, dashboard password, authenticator, and app passwords for calendar apps." />
      <Notices notice={notice} />
      <div className="page-grid">
        <form className="home-panel" onSubmit={saveProfile}>
          <div className="panel-head">
            <span className="stat-icon">
              <UserRound size={16} aria-hidden />
            </span>
            <div>
              <h2>Profile</h2>
              <p className="muted">Signed in as {user.username}.</p>
            </div>
          </div>
          <div className="field">
            <span>Display name</span>
            <input value={display} onChange={(e) => setDisplay(e.target.value)} required />
          </div>
          <div className="field">
            <span>Email</span>
            <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
          </div>
          <div className="field">
            <span>Timezone</span>
            <select value={tz} onChange={(e) => setTz(e.target.value)}>
              {TIMEZONES.map((z) => (
                <option key={z}>{z}</option>
              ))}
            </select>
          </div>
          <div className="form-actions">
            <button className="btn" type="submit" disabled={savingProfile}>
              {savingProfile ? "Saving…" : "Save profile"}
            </button>
          </div>
        </form>
        <form className="home-panel" onSubmit={savePassword}>
          <div className="panel-head">
            <span className="stat-icon">
              <KeyRound size={16} aria-hidden />
            </span>
            <div>
              <h2>Dashboard password</h2>
              <p className="muted">Used to sign in here. DAV clients should use an app password.</p>
            </div>
          </div>
          <div className="field">
            <span>Current password</span>
            <input type="password" value={current} onChange={(e) => setCurrent(e.target.value)} required />
          </div>
          <div className="field">
            <span>New password (min 8)</span>
            <input type="password" value={next} onChange={(e) => setNext(e.target.value)} minLength={8} required />
          </div>
          <div className="field">
            <span>Confirm new password</span>
            <input type="password" value={next2} onChange={(e) => setNext2(e.target.value)} minLength={8} required />
          </div>
          <div className="form-actions">
            <button className="btn" type="submit" disabled={savingPass}>
              {savingPass ? "Updating…" : "Update password"}
            </button>
          </div>
        </form>
        <TotpPanel user={user} onUser={setUser} />
        <AppPasswordsPanel />
        <SessionsPanel />
        <TakeoutPanel />
      </div>
    </>
  );
}

function SessionsPanel() {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [ok, setOk] = useState("");

  async function revoke() {
    if (!window.confirm("Sign out every other browser and device?")) return;
    setErr("");
    setBusy(true);
    try {
      await api.revokeSessions();
      setOk("Other sessions were signed out.");
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : "Could not revoke sessions.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="home-panel">
      <div className="panel-head">
        <span className="stat-icon">
          <ShieldOff size={16} aria-hidden />
        </span>
        <div>
          <h2>Sessions</h2>
          <p className="muted">Sign out every other device. This one stays signed in.</p>
        </div>
      </div>
      {err ? <div className="banner err">{err}</div> : null}
      {ok ? <div className="banner ok">{ok}</div> : null}
      <div className="form-actions">
        <button className="btn secondary sm" type="button" onClick={revoke} disabled={busy}>
          {busy ? "Revoking…" : "Revoke other sessions"}
        </button>
      </div>
    </section>
  );
}

function TakeoutPanel() {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  async function download() {
    setErr("");
    setBusy(true);
    try {
      await api.exportTakeout();
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : "Could not export.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="home-panel">
      <div className="panel-head">
        <span className="stat-icon">
          <Download size={16} aria-hidden />
        </span>
        <div>
          <h2>Download your data</h2>
          <p className="muted">
            A restorable zip of your calendars, tasks, contacts, and files. Restore it — or download a full account
            backup — from <Link href="/app/settings">Settings</Link>.
          </p>
        </div>
      </div>
      {err ? <div className="banner err">{err}</div> : null}
      <div className="form-actions">
        <button className="btn secondary sm" type="button" onClick={download} disabled={busy}>
          {busy ? "Preparing…" : "Download zip"}
        </button>
      </div>
    </section>
  );
}
