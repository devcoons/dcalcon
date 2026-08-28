"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import { CalendarDays, Check, Inbox, MapPin, X } from "lucide-react";
import { api, type Calendar, type Invitation } from "@/lib/api";
import { calendarAcceptTarget, displayIdentity, formatWhenRange } from "@/lib/format";
import { useSession } from "@/lib/shell";
import { Notices, PageHeader, useNotice } from "@/lib/ui";

function statusClass(status: string) {
  if (status === "accepted") return "ok";
  if (status === "pending") return "warn";
  return "bad";
}

function statusLabel(status: string) {
  return status ? status.charAt(0).toUpperCase() + status.slice(1) : status;
}

export default function InvitationsPage() {
  const { refreshOverview } = useSession();
  const notice = useNotice();
  const [items, setItems] = useState<Invitation[]>([]);
  const [cals, setCals] = useState<Calendar[]>([]);
  const [targetCal, setTargetCal] = useState(0);
  const [busy, setBusy] = useState<number | null>(null);
  const loadGen = useRef(0);

  async function load() {
    const gen = ++loadGen.current;
    const [inv, list] = await Promise.all([api.invitations(), api.calendars()]);
    if (gen !== loadGen.current) return;
    setItems(inv ?? []);
    const writable = (list ?? []).filter(calendarAcceptTarget);
    setCals(writable);
    setTargetCal((id) => id || writable[0]?.id || 0);
  }

  useEffect(() => {
    load().catch((e) => notice.fail(e instanceof Error ? e.message : "Could not load invitations."));
  }, []);

  const pending = useMemo(() => items.filter((it) => it.status === "pending"), [items]);
  const earlier = useMemo(() => items.filter((it) => it.status !== "pending"), [items]);

  async function act(id: number, kind: "accept" | "decline") {
    setBusy(id);
    try {
      if (kind === "accept") await api.accept(id, targetCal || undefined);
      else await api.decline(id);
      await load();
      await refreshOverview();
      notice.done(kind === "accept" ? "Event added to your personal calendar." : "Invitation declined.");
    } catch (ex) {
      notice.fail(ex instanceof Error ? ex.message : "Could not update invitation.");
    } finally {
      setBusy(null);
    }
  }

  return (
    <>
      <PageHeader
        title="Invitations"
        lede="Events another local user invited you to. Choose which calendar to copy them into when you accept."
      />
      <Notices notice={notice} />

      {items.length === 0 ? (
        <div className="empty-hero">
          <span className="stat-icon">
            <Inbox size={18} aria-hidden />
          </span>
          <h2>Nothing waiting</h2>
          <p className="muted">When someone on this server invites you, it shows up here.</p>
          <Link className="btn sm" href="/app/calendars">
            <CalendarDays size={15} aria-hidden />
            Open calendar
          </Link>
        </div>
      ) : (
        <div className="invite-page">
          {pending.length === 0 ? (
            <p className="muted">No pending invitations.</p>
          ) : (
            <div className="invite-list">
              {cals.length > 1 ? (
                <div className="field">
                  <span>Add accepted events to</span>
                  <select value={targetCal} onChange={(e) => setTargetCal(Number(e.target.value))}>
                    {cals.map((c) => (
                      <option key={c.id} value={c.id}>
                        {c.name}
                      </option>
                    ))}
                  </select>
                </div>
              ) : null}
              {pending.map((it) => (
                <InviteCard key={it.id} it={it} busy={busy === it.id} onAct={act} />
              ))}
            </div>
          )}
          {earlier.length > 0 ? (
            <div className="invite-history">
              <div className="section-label">Earlier</div>
              <div className="invite-list">
                {earlier.map((it) => (
                  <InviteCard key={it.id} it={it} />
                ))}
              </div>
            </div>
          ) : null}
        </div>
      )}
    </>
  );
}

function InviteCard({
  it,
  busy,
  onAct,
}: {
  it: Invitation;
  busy?: boolean;
  onAct?: (id: number, kind: "accept" | "decline") => void;
}) {
  const when = formatWhenRange(it.dtstart || "", it.dtend, false);
  return (
    <article className={`invite-card${it.status === "pending" ? " pending" : ""}`}>
      <div className="invite-card-body">
        <div className="invite-card-top">
          <span className={`pill ${statusClass(it.status)}`}>{statusLabel(it.status)}</span>
        </div>
        <h2>{it.summary || it.uid}</h2>
        <p className="muted">
          {when}
          {it.location ? (
            <>
              {" · "}
              <MapPin size={12} aria-hidden /> {it.location}
            </>
          ) : null}
        </p>
        <p className="muted">From {displayIdentity(it.organizer) || "someone on this server"}</p>
        {it.description ? <p className="invite-note">{it.description}</p> : null}
      </div>
      {it.status === "pending" && onAct ? (
        <div className="btn-row">
          <button className="btn sm" type="button" disabled={busy} onClick={() => onAct(it.id, "accept")}>
            <Check size={14} aria-hidden />
            {busy ? "Working…" : "Accept"}
          </button>
          <button className="btn secondary sm" type="button" disabled={busy} onClick={() => onAct(it.id, "decline")}>
            <X size={14} aria-hidden />
            Decline
          </button>
        </div>
      ) : null}
    </article>
  );
}
