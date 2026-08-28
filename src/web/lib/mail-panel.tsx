"use client";

import { FormEvent, useEffect, useState } from "react";
import { Mail } from "lucide-react";
import { api, type ConnectedAccount, type MailStatus } from "@/lib/api";
import { Banner, CopyField, Fold, errorMessage } from "@/lib/ui";

function providerLabel(p: string) {
  if (p === "google") return "Google";
  if (p === "microsoft") return "Microsoft";
  if (p === "smtp") return "SMTP";
  return p;
}

export function MailPanel() {
  const [status, setStatus] = useState<MailStatus | null>(null);
  const [accounts, setAccounts] = useState<ConnectedAccount[]>([]);
  const [err, setErr] = useState("");
  const [ok, setOk] = useState("");
  const [busy, setBusy] = useState("");
  const [smtp, setSmtp] = useState({
    email: "",
    host: "",
    port: "587",
    username: "",
    password: "",
  });

  const origin = typeof window !== "undefined" ? window.location.origin : "";
  const googleRedirect = origin + (status?.google_callback_path ?? "/api/v1/oauth/google/callback");
  const microsoftRedirect = origin + (status?.microsoft_callback_path ?? "/api/v1/oauth/microsoft/callback");

  async function load() {
    const [s, a] = await Promise.all([api.mailStatus(), api.accounts()]);
    setStatus(s);
    setAccounts(a);
  }

  useEffect(() => {
    const q = new URLSearchParams(window.location.search);
    if (q.get("mail") === "ok") setOk("Email account connected. You can send invitations from the event editor.");
    if (q.get("mail") === "err") setErr(q.get("detail") || "Could not connect the email account.");
    if (q.get("mail")) window.history.replaceState({}, "", "/app/settings");
    load().catch((e) => setErr(errorMessage(e, "Could not load mail settings.")));
  }, []);

  async function connectOAuth(provider: "google" | "microsoft") {
    setErr("");
    setOk("");
    setBusy(provider);
    try {
      const out = await api.connectAccount({ provider, origin: window.location.origin });
      if (!out.authorize_url) throw new Error("No authorize URL returned.");
      window.location.href = out.authorize_url;
    } catch (ex) {
      setErr(errorMessage(ex, "Could not start OAuth."));
      setBusy("");
    }
  }

  async function connectSMTP(e: FormEvent) {
    e.preventDefault();
    setErr("");
    setOk("");
    setBusy("smtp");
    try {
      await api.connectAccount({
        provider: "smtp",
        email: smtp.email,
        host: smtp.host,
        port: Number(smtp.port) || 587,
        username: smtp.username || smtp.email,
        password: smtp.password,
      });
      setSmtp({ ...smtp, password: "" });
      setOk("SMTP account saved.");
      await load();
    } catch (ex) {
      setErr(errorMessage(ex, "Could not save SMTP."));
    } finally {
      setBusy("");
    }
  }

  async function disconnect(id: number) {
    setErr("");
    try {
      await api.disconnectAccount(id);
      setOk("Account disconnected.");
      await load();
    } catch (ex) {
      setErr(errorMessage(ex, "Could not disconnect."));
    }
  }

  async function test(id: number) {
    setErr("");
    setBusy(`test-${id}`);
    try {
      await api.testAccount(id);
      setOk("Test message sent to the connected address.");
    } catch (ex) {
      setErr(errorMessage(ex, "Test send failed."));
    } finally {
      setBusy("");
    }
  }

  return (
    <section className="home-panel">
      <div className="panel-head">
        <span className="stat-icon">
          <Mail size={16} aria-hidden />
        </span>
        <div>
          <h2>Connected email</h2>
          <p className="muted">
            Send invitations to people outside this server. Linking a mailbox is send-only — calendars stay on dCalCon.
          </p>
        </div>
      </div>
      {err ? <Banner kind="err">{err}</Banner> : null}
      {ok ? <Banner kind="ok">{ok}</Banner> : null}
      {!status?.token_key ? (
        <Banner kind="info">Restart the core once so it can create an encryption key beside the database.</Banner>
      ) : null}

      {accounts.length === 0 ? <p className="muted">No mailbox linked yet.</p> : null}
      {accounts.map((a) => (
        <div className="item account-row" key={a.id}>
          <div>
            <strong>{providerLabel(a.provider)}</strong>
            <div className="muted">
              {a.email}
              {a.status !== "connected" ? ` · ${a.status}` : ""}
            </div>
            {a.last_error ? <p className="muted">{a.last_error}</p> : null}
          </div>
          <div className="btn-row">
            <button className="btn secondary sm" type="button" disabled={busy !== ""} onClick={() => test(a.id)}>
              {busy === `test-${a.id}` ? "Sending…" : "Send test"}
            </button>
            <button className="btn ghost sm" type="button" onClick={() => disconnect(a.id)}>
              Disconnect
            </button>
          </div>
        </div>
      ))}

      <div className="form-actions">
        <button
          className="btn"
          type="button"
          disabled={!status?.google_configured || busy !== ""}
          onClick={() => connectOAuth("google")}
        >
          {busy === "google" ? "Redirecting…" : "Connect Google"}
        </button>
        <button
          className="btn secondary"
          type="button"
          disabled={!status?.microsoft_configured || busy !== ""}
          onClick={() => connectOAuth("microsoft")}
        >
          {busy === "microsoft" ? "Redirecting…" : "Connect Microsoft"}
        </button>
      </div>
      {!status?.google_configured && !status?.microsoft_configured ? (
        <p className="muted">OAuth client IDs are not set on the server. You can still add SMTP below.</p>
      ) : null}
      {status?.server_smtp ? (
        <p className="muted">Server SMTP is configured and used if you have not linked a personal mailbox.</p>
      ) : null}

      <Fold title="Add SMTP mailbox">
        <form className="stack" onSubmit={connectSMTP}>
          <p className="muted">Port 587 uses STARTTLS; 465 uses implicit TLS.</p>
          <div className="row">
            <label className="field">
              <span>From / email</span>
              <input
                type="email"
                value={smtp.email}
                onChange={(e) => setSmtp({ ...smtp, email: e.target.value })}
                required
              />
            </label>
            <label className="field">
              <span>Host</span>
              <input value={smtp.host} onChange={(e) => setSmtp({ ...smtp, host: e.target.value })} required />
            </label>
          </div>
          <div className="row">
            <label className="field">
              <span>Port</span>
              <input value={smtp.port} onChange={(e) => setSmtp({ ...smtp, port: e.target.value })} />
            </label>
            <label className="field">
              <span>Username</span>
              <input value={smtp.username} onChange={(e) => setSmtp({ ...smtp, username: e.target.value })} />
            </label>
          </div>
          <label className="field">
            <span>Password</span>
            <input
              type="password"
              value={smtp.password}
              onChange={(e) => setSmtp({ ...smtp, password: e.target.value })}
              required
            />
          </label>
          <div className="form-actions">
            <button className="btn secondary" type="submit" disabled={busy !== ""}>
              {busy === "smtp" ? "Saving…" : "Save SMTP account"}
            </button>
          </div>
        </form>
      </Fold>

      <Fold title="OAuth redirect URIs">
        <p className="muted">Register these exact URIs on the Google or Microsoft app. Use the dashboard origin, not the Go listen port.</p>
        <CopyField label="Google" value={googleRedirect} />
        <CopyField label="Microsoft" value={microsoftRedirect} />
      </Fold>
    </section>
  );
}
