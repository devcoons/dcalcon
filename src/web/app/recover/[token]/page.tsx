"use client";

import { FormEvent, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { api } from "@/lib/api";

export default function RecoverTokenPage() {
  const params = useParams<{ token: string }>();
  const token = typeof params.token === "string" ? params.token : "";
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [err, setErr] = useState("");
  const [ok, setOk] = useState(false);
  const [busy, setBusy] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setErr("");
    if (password !== confirm) {
      setErr("Passwords do not match.");
      return;
    }
    setBusy(true);
    try {
      await api.resetWithToken(token, password);
      setOk(true);
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
        <h1>Choose a new password</h1>
        {ok ? (
          <>
            <div className="banner ok">
              Password updated. If an authenticator is enabled on the account, sign-in still requires a current code.
            </div>
            <p>
              <Link className="btn" href="/login">
                Sign in
              </Link>
            </p>
          </>
        ) : (
          <>
            <div className="field">
              <span>New password</span>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                minLength={8}
                required
                autoFocus
              />
            </div>
            <div className="field">
              <span>Confirm</span>
              <input
                type="password"
                value={confirm}
                onChange={(e) => setConfirm(e.target.value)}
                minLength={8}
                required
              />
            </div>
            {err ? <div className="banner err">{err}</div> : null}
            <button className="btn" type="submit" disabled={busy || !token}>
              {busy ? "Saving…" : "Update password"}
            </button>
          </>
        )}
        <p className="muted">
          <Link href="/login">Back to sign in</Link>
        </p>
      </form>
    </div>
  );
}
