"use client";

import { FormEvent, useEffect, useState } from "react";
import { KeyRound } from "lucide-react";
import { api, type AppPassword } from "@/lib/api";
import { CopyField } from "@/lib/ui";

export function AppPasswordsPanel() {
  const [list, setList] = useState<AppPassword[]>([]);
  const [name, setName] = useState("");
  const [secret, setSecret] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  async function reload() {
    setList(await api.appPasswords());
  }

  useEffect(() => {
    reload().catch((e) => setErr(e instanceof Error ? e.message : "Could not load app passwords."));
  }, []);

  async function create(e: FormEvent) {
    e.preventDefault();
    setErr("");
    setBusy(true);
    try {
      const created = await api.createAppPassword(name.trim() || "DAV client");
      setSecret(created.password);
      setName("");
      await reload();
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : "Could not create an app password.");
    } finally {
      setBusy(false);
    }
  }

  async function revoke(id: number) {
    setErr("");
    try {
      await api.deleteAppPassword(id);
      await reload();
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : "Could not revoke.");
    }
  }

  return (
    <section className="home-panel">
      <div className="panel-head">
        <span className="stat-icon">
          <KeyRound size={16} aria-hidden />
        </span>
        <div>
          <h2>DAV app passwords</h2>
          <p className="muted">
            CalDAV and CardDAV clients should use a dedicated app password. The dashboard password still works for existing DAV clients.
          </p>
        </div>
      </div>
      {err ? <p className="banner err">{err}</p> : null}
      {secret ? (
        <div className="banner ok">
          <p>Copy this password now. It will not be shown again.</p>
          <CopyField label="App password" value={secret} />
        </div>
      ) : null}
      <form onSubmit={create} className="stack">
        <div className="field">
          <span>Label</span>
          <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Phone, Thunderbird…" />
        </div>
        <div className="form-actions">
          <button className="btn" type="submit" disabled={busy}>
            {busy ? "Creating…" : "Create app password"}
          </button>
        </div>
      </form>
      {list.length ? (
        <div className="list">
          {list.map((p) => (
            <div className="item account-row" key={p.id}>
              <div>
                <strong>{p.name}</strong>
                <div className="muted mono">{p.prefix}…</div>
              </div>
              <button className="btn danger sm" type="button" onClick={() => revoke(p.id)}>
                Revoke
              </button>
            </div>
          ))}
        </div>
      ) : (
        <p className="muted">No app passwords yet.</p>
      )}
    </section>
  );
}
