"use client";

import { FormEvent, useState } from "react";
import Link from "next/link";
import { api } from "@/lib/api";

export default function RecoverPage() {
  const [mode, setMode] = useState<"email" | "totp">("email");
  const [email, setEmail] = useState("");
  const [username, setUsername] = useState("");
  const [code, setCode] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [err, setErr] = useState("");
  const [ok, setOk] = useState("");
  const [busy, setBusy] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setErr("");
    setBusy(true);
    try {
      if (mode === "email") {
        await api.recover(email.trim());
        setOk("If that address exists and mail is configured, a reset link is on its way. Otherwise ask an administrator for a recovery link.");
      } else {
        if (password !== confirm) {
          setErr("Passwords do not match.");
          return;
        }
        await api.resetWithTotp(username.trim(), code.trim(), password);
        setOk("Password updated. Sign-in still requires an authenticator code.");
      }
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : "Could not reset password.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="auth">
      <form onSubmit={onSubmit} className="auth-card">
        <div className="brand-mark">dCalCon</div>
        <h1>Reset password</h1>
        <div className="auth-tabs">
          <button type="button" className={mode === "email" ? "on" : ""} onClick={() => setMode("email")}>
            Email link
          </button>
          <button type="button" className={mode === "totp" ? "on" : ""} onClick={() => setMode("totp")}>
            Authenticator
          </button>
        </div>
        {ok ? <div className="banner ok">{ok}</div> : null}
        {!ok || mode === "totp" ? (
          <>
            {mode === "email" && !ok ? (
              <>
                <p className="muted">
                  Enter the email on your account. If mail is configured on the server, a reset link is sent. If it is
                  not, ask an administrator to copy a recovery link from Users.
                </p>
                <div className="field">
                  <span>Email</span>
                  <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="email" required autoFocus />
                </div>
              </>
            ) : null}
            {mode === "totp" && !ok ? (
              <>
                <p className="muted">If an authenticator is enabled, a current code from your phone can set a new password immediately.</p>
                <div className="field">
                  <span>Username</span>
                  <input value={username} onChange={(e) => setUsername(e.target.value)} required autoFocus />
                </div>
                <div className="field">
                  <span>Authenticator code</span>
                  <input
                    className="totp-code"
                    inputMode="numeric"
                    maxLength={6}
                    value={code}
                    onChange={(e) => setCode(e.target.value.replace(/\D/g, "").slice(0, 6))}
                    required
                  />
                </div>
                <div className="field">
                  <span>New password</span>
                  <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} minLength={8} required />
                </div>
                <div className="field">
                  <span>Confirm</span>
                  <input type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} minLength={8} required />
                </div>
              </>
            ) : null}
            {err ? <div className="banner err">{err}</div> : null}
            {(!ok || mode === "totp") && !(mode === "email" && ok) ? (
              <button className="btn" type="submit" disabled={busy}>
                {busy ? "Working…" : mode === "email" ? "Request reset" : "Set new password"}
              </button>
            ) : null}
          </>
        ) : null}
        {ok && mode === "totp" ? (
          <p>
            <Link className="btn" href="/login">
              Sign in
            </Link>
          </p>
        ) : null}
        <p className="muted">
          <Link href="/login">Back to sign in</Link>
        </p>
      </form>
    </div>
  );
}
