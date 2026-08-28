"use client";

import { FormEvent, useState } from "react";
import Link from "next/link";
import { ApiError, api } from "@/lib/api";

export default function LoginPage() {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [totp, setTotp] = useState("");
  const [needTotp, setNeedTotp] = useState(false);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setErr("");
    setBusy(true);
    try {
      await api.login(username.trim(), password, totp.trim() || undefined);
      window.location.href = "/app";
    } catch (ex) {
      if (ex instanceof ApiError && /authenticator code required/i.test(ex.message)) {
        setNeedTotp(true);
        setErr("Enter the 6-digit code from your authenticator.");
      } else {
        setErr(ex instanceof Error ? ex.message : "Could not sign in.");
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="auth">
      <form onSubmit={onSubmit} className="auth-card">
        <div className="brand-mark">dCalCon</div>
        <h1>Sign in</h1>
        <p className="muted">
          Use your dashboard password. If you enabled an authenticator, the 6-digit code is required as a second factor.
          Calendar and contacts apps must use an app password once that is on.
        </p>
        <div className="field">
          <span>Username</span>
          <input
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            autoComplete="username"
            required
            autoFocus
          />
        </div>
        <div className="field">
          <span>Password</span>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
            required
          />
        </div>
        <div className="field">
          <span>Authenticator code{needTotp ? "" : " (if enabled)"}</span>
          <input
            className="totp-code"
            inputMode="numeric"
            autoComplete="one-time-code"
            pattern="[0-9]*"
            maxLength={6}
            value={totp}
            onChange={(e) => setTotp(e.target.value.replace(/\D/g, "").slice(0, 6))}
            required={needTotp}
          />
        </div>
        {err ? <div className="banner err">{err}</div> : null}
        <button className="btn" type="submit" disabled={busy}>
          {busy ? "Signing in…" : "Continue"}
        </button>
        <p className="muted">
          <Link href="/recover">Forgot password?</Link>
        </p>
      </form>
    </div>
  );
}
