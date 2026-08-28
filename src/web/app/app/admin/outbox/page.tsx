"use client";

import { useEffect, useRef, useState } from "react";
import { api, type RecoveryMessage } from "@/lib/api";
import { Notices, PageHeader, useNotice } from "@/lib/ui";

function deliveredLabel(d: string) {
  if (d === "sent") return "Emailed";
  if (d === "error") return "SMTP error";
  return "Not emailed";
}

export default function RecoveryOutboxPage() {
  const notice = useNotice();
  const [rows, setRows] = useState<RecoveryMessage[]>([]);
  const loadGen = useRef(0);

  async function load() {
    const gen = ++loadGen.current;
    const list = await api.recoveryOutbox();
    if (gen !== loadGen.current) return;
    setRows(list);
  }

  useEffect(() => {
    load().catch((e) => notice.fail(e instanceof Error ? e.message : "Admin only."));
  }, []);

  return (
    <>
      <PageHeader
        title="Recovery mail"
        lede="Attempts to send password-reset email. Reset URLs are never stored here. If SMTP is off, create a link from Users and copy it."
      />
      <Notices notice={notice} />
      {rows.length === 0 ? (
        <div className="empty">No recovery attempts yet.</div>
      ) : (
        <div className="panel table-wrap">
          <table className="data">
            <thead>
              <tr>
                <th>When</th>
                <th>User</th>
                <th>Email</th>
                <th>Result</th>
                <th>Detail</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.id}>
                  <td>{r.created_at.replace("T", " ").replace("Z", " UTC")}</td>
                  <td>{r.username || r.user_id}</td>
                  <td>{r.email}</td>
                  <td>{deliveredLabel(r.delivered)}</td>
                  <td className="muted">{r.last_error || "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}
