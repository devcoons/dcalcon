"use client";

import { FormEvent, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { api, type AuditEntry, type CreatedUser, type User } from "@/lib/api";
import { generatePassword, TIMEZONES } from "@/lib/format";
import { CopyField, Notices, PageHeader, useNotice } from "@/lib/ui";
import { useSession } from "@/lib/shell";

export default function AdminUsersPage() {
  const { user: me } = useSession();
  const notice = useNotice();
  const [users, setUsers] = useState<User[]>([]);
  const [open, setOpen] = useState(false);
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [display, setDisplay] = useState("");
  const [role, setRole] = useState("user");
  const [tz, setTz] = useState("UTC");
  const [password, setPassword] = useState("");
  const [created, setCreated] = useState<CreatedUser | null>(null);
  const [shownPass, setShownPass] = useState("");
  const [resetFor, setResetFor] = useState<User | null>(null);
  const [resetPass, setResetPass] = useState("");
  const [recoveryURL, setRecoveryURL] = useState("");
  const [audit, setAudit] = useState<AuditEntry[]>([]);
  const loadGen = useRef(0);

  async function load() {
    const gen = ++loadGen.current;
    const [list, log] = await Promise.all([api.users(), api.audit().catch(() => [])]);
    if (gen !== loadGen.current) return;
    setUsers(list ?? []);
    setAudit(log ?? []);
  }

  useEffect(() => {
    load().catch((e) => notice.fail(e instanceof Error ? e.message : "Admin only."));
  }, []);

  function fillPassword() {
    const p = generatePassword();
    setPassword(p);
  }

  async function create(e: FormEvent) {
    e.preventDefault();
    setCreated(null);
    try {
      const result = await api.createUser({
        username: username.trim(),
        email: email.trim(),
        password,
        display_name: display.trim() || username.trim(),
        role,
        timezone: tz,
      });
      setCreated(result);
      setShownPass(password);
      setUsername("");
      setEmail("");
      setDisplay("");
      setPassword("");
      setRole("user");
      setOpen(false);
      await load();
      notice.done(`User ${result.user.username} created. Share the password now — it is not stored in clear text.`);
    } catch (ex) {
      notice.fail(ex instanceof Error ? ex.message : "Could not create user.");
    }
  }

  async function setStatus(u: User, status: string) {
    try {
      await api.patchUser(u.id, { status });
      await load();
      notice.done(`${u.username} is now ${status}.`);
    } catch (ex) {
      notice.fail(ex instanceof Error ? ex.message : "Could not update user.");
    }
  }

  async function setRoleOf(u: User, next: string) {
    try {
      await api.patchUser(u.id, { role: next });
      await load();
      notice.done(`${u.username} is now ${next}.`);
    } catch (ex) {
      notice.fail(ex instanceof Error ? ex.message : "Could not update role.");
    }
  }

  async function sendLink(u: User) {
    try {
      const res = await api.sendRecovery(u.id);
      setRecoveryURL(res.recovery_url);
      notice.done(
        res.emailed
          ? `Reset email sent to ${u.email}. The link is also below if you need to copy it.`
          : `SMTP is not configured. Copy this reset link for ${u.username}.`,
      );
    } catch (ex) {
      notice.fail(ex instanceof Error ? ex.message : "Could not create recovery link.");
    }
  }

  async function doReset(e: FormEvent) {
    e.preventDefault();
    if (!resetFor) return;
    try {
      await api.resetPassword(resetFor.id, resetPass);
      notice.done(`Password reset for ${resetFor.username}. They must sign in again.`);
      setResetFor(null);
      setResetPass("");
    } catch (ex) {
      notice.fail(ex instanceof Error ? ex.message : "Could not reset password.");
    }
  }

  return (
    <>
      <PageHeader
        title="Users"
        lede={
          <>
            Each person gets a Personal calendar, a Contacts address book, and CalDAV/CardDAV credentials. There is no
            public self-registration. Without SMTP, copy a recovery link here;{" "}
            <Link href="/app/admin/outbox">Recovery mail</Link> lists send attempts.
          </>
        }
      >
        <button
          className="btn"
          type="button"
          onClick={() => {
            setOpen((v) => !v);
            if (!password) fillPassword();
          }}
        >
          {open ? "Close" : "Create user"}
        </button>
      </PageHeader>
      <Notices notice={notice} />

      {recoveryURL ? (
        <div className="created-box">
          <h2>Recovery link</h2>
          <p>Valid for two hours. Share it only with that person.</p>
          <CopyField label="Reset URL" value={recoveryURL} />
        </div>
      ) : null}

      {created ? (
        <div className="created-box">
          <h2>User is ready</h2>
          <p>
            Give <strong>{created.user.display_name}</strong> these details. The password is shown once.
          </p>
          <CopyField label="Username" value={created.user.username} />
          <CopyField label="Temporary password" value={shownPass} />
          <CopyField label="CalDAV" value={created.setup.caldav_well_known} />
          <CopyField label="CardDAV" value={created.setup.carddav_well_known} />
        </div>
      ) : null}

      {open ? (
        <form className="panel" onSubmit={create}>
          <h2>New user</h2>
          <div className="row">
            <div className="field">
              <span>Username</span>
              <input value={username} onChange={(e) => setUsername(e.target.value)} required minLength={2} />
            </div>
            <div className="field">
              <span>Email</span>
              <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
            </div>
            <div className="field">
              <span>Display name</span>
              <input value={display} onChange={(e) => setDisplay(e.target.value)} placeholder="Optional" />
            </div>
          </div>
          <div className="row">
            <div className="field">
              <span>Role</span>
              <select value={role} onChange={(e) => setRole(e.target.value)}>
                <option value="user">User</option>
                <option value="admin">Administrator</option>
              </select>
            </div>
            <div className="field">
              <span>Timezone</span>
              <select value={tz} onChange={(e) => setTz(e.target.value)}>
                {TIMEZONES.map((z) => (
                  <option key={z}>{z}</option>
                ))}
              </select>
            </div>
            <div className="field">
              <span>Password</span>
              <div className="copy-row">
                <input value={password} onChange={(e) => setPassword(e.target.value)} minLength={8} required />
                <button className="btn secondary sm" type="button" onClick={fillPassword}>
                  Generate
                </button>
              </div>
            </div>
          </div>
          <div className="form-actions">
            <button className="btn" type="submit">
              Create user
            </button>
          </div>
        </form>
      ) : null}

      {resetFor ? (
        <form className="panel" onSubmit={doReset}>
          <h2>Reset password for {resetFor.username}</h2>
          <div className="copy-row">
            <input value={resetPass} onChange={(e) => setResetPass(e.target.value)} minLength={8} required />
            <button className="btn secondary sm" type="button" onClick={() => setResetPass(generatePassword())}>
              Generate
            </button>
            <button className="btn" type="submit">
              Save password
            </button>
            <button className="btn ghost sm" type="button" onClick={() => setResetFor(null)}>
              Cancel
            </button>
          </div>
        </form>
      ) : null}

      <div className="table-wrap">
      <table className="data">
        <thead>
          <tr>
            <th>User</th>
            <th>Email</th>
            <th>Role</th>
            <th>Status</th>
            <th>Authenticator</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {users.map((u) => (
            <tr key={u.id}>
              <td>
                <strong>{u.display_name || u.username}</strong>
                <div className="muted">{u.username}</div>
              </td>
              <td className="muted">{u.email}</td>
              <td>
                <select
                  value={u.role}
                  disabled={me.id === u.id}
                  onChange={(e) => setRoleOf(u, e.target.value)}
                  className="auto"
                >
                  <option value="user">user</option>
                  <option value="admin">admin</option>
                </select>
              </td>
              <td>
                <span className={`pill ${u.status === "active" ? "ok" : "bad"}`}>{u.status}</span>
              </td>
              <td>{u.totp_enabled ? <span className="pill ok">on</span> : <span className="muted">off</span>}</td>
              <td>
                <div className="btn-row">
                  {me.id !== u.id ? (
                    u.status === "active" ? (
                      <button className="btn secondary sm" type="button" onClick={() => setStatus(u, "disabled")}>
                        Disable
                      </button>
                    ) : (
                      <button className="btn secondary sm" type="button" onClick={() => setStatus(u, "active")}>
                        Enable
                      </button>
                    )
                  ) : null}
                  <details className="menu">
                    <summary className="btn secondary sm">More</summary>
                    <div className="menu-panel">
                      <button
                        className="btn ghost sm"
                        type="button"
                        onClick={() => {
                          setResetFor(u);
                          setResetPass(generatePassword());
                        }}
                      >
                        Reset password
                      </button>
                      <button className="btn ghost sm" type="button" onClick={() => sendLink(u)}>
                        Recovery link
                      </button>
                      {u.totp_enabled ? (
                        <button
                          className="btn ghost sm"
                          type="button"
                          onClick={async () => {
                            try {
                              await api.disableUserTotp(u.id);
                              await load();
                              notice.done(`Authenticator turned off for ${u.username}.`);
                            } catch (ex) {
                              notice.fail(ex instanceof Error ? ex.message : "Could not disable authenticator.");
                            }
                          }}
                        >
                          Turn off TOTP
                        </button>
                      ) : null}
                    </div>
                  </details>
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      </div>
      {audit.length > 0 ? (
        <section className="home-panel" style={{ marginTop: 24 }}>
          <h2>Audit</h2>
          <p className="muted">Recent admin and account actions on this node.</p>
          <table>
            <thead>
              <tr>
                <th>When</th>
                <th>Who</th>
                <th>Action</th>
                <th>Detail</th>
              </tr>
            </thead>
            <tbody>
              {audit.slice(0, 40).map((e) => (
                <tr key={e.id}>
                  <td>{e.at.replace("T", " ").slice(0, 19)}</td>
                  <td>{e.actor || "—"}</td>
                  <td>{e.action}</td>
                  <td className="muted">{e.detail}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      ) : null}
    </>
  );
}
