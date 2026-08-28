"use client";

import { FormEvent, useState } from "react";
import { ShieldCheck } from "lucide-react";
import { api, type User } from "@/lib/api";
import { QrCode } from "@/lib/qr-code";
import { CopyField } from "@/lib/ui";

export function TotpPanel({ user, onUser }: { user: User; onUser: (u: User) => void }) {
  const [phase, setPhase] = useState<"idle" | "setup" | "disable">("idle");
  const [secret, setSecret] = useState("");
  const [otpauth, setOtpauth] = useState("");
  const [code, setCode] = useState("");
  const [password, setPassword] = useState("");
  const [err, setErr] = useState("");
  const [ok, setOk] = useState("");
  const [busy, setBusy] = useState(false);

  async function startSetup() {
    setErr("");
    setBusy(true);
    try {
      const s = await api.totpSetup();
      setSecret(s.secret);
      setOtpauth(s.otpauth);
      setCode("");
      setPhase("setup");
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : "Could not start authenticator setup.");
    } finally {
      setBusy(false);
    }
  }

  async function confirm(e: FormEvent) {
    e.preventDefault();
    setErr("");
    setBusy(true);
    try {
      await api.totpEnable(code.trim());
      onUser({ ...user, totp_enabled: true });
      setPhase("idle");
      setSecret("");
      setOtpauth("");
      setCode("");
      setOk("Authenticator is on. Sign-in now requires your password plus this code. Create an app password for CalDAV.");
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : "That code did not match. Wait for the next one and try again.");
    } finally {
      setBusy(false);
    }
  }

  async function cancel() {
    await api.totpCancel().catch(() => undefined);
    setPhase("idle");
    setSecret("");
    setOtpauth("");
    setCode("");
  }

  async function disable(e: FormEvent) {
    e.preventDefault();
    setErr("");
    setBusy(true);
    try {
      await api.totpDisable({ password: password || undefined, code: code || undefined });
      onUser({ ...user, totp_enabled: false });
      setPhase("idle");
      setPassword("");
      setCode("");
      setOk("Authenticator turned off.");
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : "Could not turn off the authenticator.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="home-panel">
      <div className="panel-head">
        <span className="stat-icon">
          <ShieldCheck size={16} aria-hidden />
        </span>
        <div>
          <h2>Authenticator</h2>
          <p className="muted">
            After this is on, sign-in needs your password <em>and</em> a code from your phone. DAV clients must use an app
            password — the dashboard password is rejected.
          </p>
        </div>
        <span className={`pill ${user.totp_enabled ? "ok" : ""}`}>{user.totp_enabled ? "On" : "Off"}</span>
      </div>
      {err ? <div className="banner err">{err}</div> : null}
      {ok ? <div className="banner ok">{ok}</div> : null}

      {user.totp_enabled && phase === "idle" ? (
        <>
          <p className="muted">Codes from your phone are required together with your password at sign-in.</p>
          <div className="form-actions">
            <button className="btn secondary sm" type="button" onClick={() => setPhase("disable")}>
              Turn off
            </button>
          </div>
        </>
      ) : null}

      {!user.totp_enabled && phase === "idle" ? (
        <div className="form-actions">
          <button className="btn" type="button" onClick={startSetup} disabled={busy}>
            {busy ? "Preparing…" : "Set up authenticator"}
          </button>
        </div>
      ) : null}

      {phase === "setup" ? (
        <form onSubmit={confirm} className="stack">
          <p className="muted">Scan this QR code in your authenticator app, then enter the 6-digit code it shows. Setup is not finished until that code is accepted.</p>
          <div className="qr-box">
            {otpauth ? <QrCode value={otpauth} label="Authenticator QR code" /> : null}
            <div className="stack" style={{ flex: 1, minWidth: 220 }}>
              <CopyField label="Manual key" value={secret} />
              <CopyField label="otpauth URL" value={otpauth} />
            </div>
          </div>
          <div className="field">
            <span>Code from your phone</span>
            <input
              className="totp-code"
              inputMode="numeric"
              autoComplete="one-time-code"
              pattern="[0-9]{6}"
              maxLength={6}
              value={code}
              onChange={(e) => setCode(e.target.value.replace(/\D/g, "").slice(0, 6))}
              required
            />
          </div>
          <div className="form-actions">
            <button className="btn" type="submit" disabled={busy || code.length !== 6}>
              {busy ? "Checking…" : "Verify and enable"}
            </button>
            <button className="btn secondary sm" type="button" onClick={cancel}>
              Cancel
            </button>
          </div>
        </form>
      ) : null}

      {phase === "disable" ? (
        <form onSubmit={disable} className="stack">
          <p className="muted">Confirm with your password or a current authenticator code.</p>
          <div className="field">
            <span>Password</span>
            <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
          </div>
          <div className="field">
            <span>Or authenticator code</span>
            <input
              className="totp-code"
              inputMode="numeric"
              maxLength={6}
              value={code}
              onChange={(e) => setCode(e.target.value.replace(/\D/g, "").slice(0, 6))}
            />
          </div>
          <div className="form-actions">
            <button className="btn danger" type="submit" disabled={busy}>
              Turn off authenticator
            </button>
            <button className="btn ghost sm" type="button" onClick={() => setPhase("idle")}>
              Cancel
            </button>
          </div>
        </form>
      ) : null}
    </section>
  );
}
