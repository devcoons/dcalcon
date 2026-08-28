"use client";

import Link from "next/link";
import { useEffect, useMemo, useState, type CSSProperties, type ReactNode } from "react";
import {
  BookUser,
  CalendarDays,
  Check,
  Inbox,
  KeyRound,
  Mail,
  Plus,
  ShieldCheck,
  X,
} from "lucide-react";
import { api, type OverviewEvent, type OverviewInvite, type Setup } from "@/lib/api";
import {
  addDays,
  calendarAcceptTarget,
  displayIdentity,
  eventTimeLabel,
  fromICSDate,
  parseYMD,
  roleLabel,
  sameDay,
} from "@/lib/format";
import { useSession } from "@/lib/shell";
import { CopyField, Notices, useNotice } from "@/lib/ui";

function chipStyle(color: string): CSSProperties {
  return { ["--chip" as string]: color || "#E72625" };
}

function greeting(now: Date) {
  const h = now.getHours();
  if (h < 12) return "Good morning";
  if (h < 18) return "Good afternoon";
  return "Good evening";
}

function dayLabel(ics: string, today: Date) {
  const key = fromICSDate(ics);
  if (!key) return "Upcoming";
  const d = parseYMD(key);
  if (sameDay(d, today)) return "Today";
  if (sameDay(d, addDays(today, 1))) return "Tomorrow";
  return d.toLocaleDateString(undefined, { weekday: "long", month: "short", day: "numeric" });
}

export default function OverviewPage() {
  const { user, overview, refreshOverview } = useSession();
  const notice = useNotice();
  const [setup, setSetup] = useState<Setup | null>(null);
  const today = useMemo(() => new Date(), []);

  useEffect(() => {
    api
      .setup()
      .then(setSetup)
      .catch((e) => notice.fail(e instanceof Error ? e.message : "Could not load overview."));
  }, []);

  const upcoming = overview?.upcoming ?? [];
  const agenda = useMemo(() => {
    const map = new Map<string, OverviewEvent[]>();
    for (const ev of upcoming) {
      const key = fromICSDate(ev.dtstart) || "other";
      const arr = map.get(key) ?? [];
      arr.push(ev);
      map.set(key, arr);
    }
    return [...map.entries()];
  }, [upcoming]);

  const pending = overview?.pending ?? [];
  const calendars = (overview?.calendar_list ?? []).filter((c) => c.kind !== "inbox" && c.kind !== "outbox");
  const acceptCals = useMemo(
    () => (overview?.calendar_list ?? []).filter(calendarAcceptTarget),
    [overview?.calendar_list],
  );
  const [targetCal, setTargetCal] = useState(0);
  const name = user.display_name || user.username;

  useEffect(() => {
    setTargetCal((id) => id || acceptCals[0]?.id || 0);
  }, [acceptCals]);

  async function act(it: OverviewInvite, kind: "accept" | "decline") {
    try {
      if (kind === "accept") await api.accept(it.id, targetCal || undefined);
      else await api.decline(it.id);
      await refreshOverview();
      notice.done(kind === "accept" ? "Event added to your calendar." : "Invitation declined.");
    } catch (ex) {
      notice.fail(ex instanceof Error ? ex.message : "Could not update invitation.");
    }
  }

  return (
    <>
      <Notices notice={notice} />

      <div className="home-hero">
        <div>
          <p className="home-kicker">{today.toLocaleDateString(undefined, { weekday: "long", month: "long", day: "numeric" })}</p>
          <h1>
            {greeting(today)}, {name}
          </h1>
          <p className="lede">
            {roleLabel(user.role)} · {user.timezone || "UTC"}
            {overview?.events_soon ? ` · ${overview.events_soon} event${overview.events_soon === 1 ? "" : "s"} in the next week` : ""}
          </p>
        </div>
        <div className="home-hero-actions">
          <Link className="btn sm" href="/app/calendars">
            <Plus size={15} aria-hidden />
            New event
          </Link>
          <Link className="btn secondary sm" href="/app/contacts">
            <BookUser size={15} aria-hidden />
            Add contact
          </Link>
        </div>
      </div>

      <div className="home-stats">
        <Stat href="/app/calendars" icon={<CalendarDays size={16} />} label="Calendars" value={overview?.calendars ?? "—"}>
          {overview?.shared_calendars
            ? `${overview.shared_calendars} shared with you`
            : "Personal and shared"}
        </Stat>
        <Stat href="/app/contacts" icon={<BookUser size={16} />} label="Contacts" value={overview?.contacts ?? "—"}>
          People and imported cards
        </Stat>
        <Stat href="/app/invitations" icon={<Inbox size={16} />} label="Invites" value={overview?.pending_invitations ?? "—"}>
          {overview?.pending_invitations ? "Waiting for a response" : "Nothing pending"}
        </Stat>
        <Stat href="/app/settings" icon={<Mail size={16} />} label="Email" value={overview?.mail_connected ? "On" : "Off"}>
          {overview?.mail_address || "Connect a mailbox to invite guests"}
        </Stat>
      </div>

      <div className="home-grid">
        <section className="home-panel">
          <div className="home-panel-head">
            <h2>Upcoming</h2>
            <Link href="/app/calendars">Open calendar</Link>
          </div>
          {agenda.length === 0 ? (
            <div className="home-empty">
              <p>No events or tasks in the next three weeks.</p>
              <div className="btn-row">
                <Link className="btn sm" href="/app/calendars">
                  Add an event
                </Link>
                <Link className="btn secondary sm" href="/app/tasks">
                  Add a task
                </Link>
              </div>
            </div>
          ) : (
            <div className="cal-agenda home-agenda">
              {agenda.map(([key, items]) => {
                const day = key === "other" ? today : parseYMD(key);
                return (
                  <section key={key} className={`cal-agenda-day${sameDay(day, today) ? " today" : ""}`}>
                    <header>
                      <span className={`cal-num${sameDay(day, today) ? " today" : ""}`}>{day.getDate()}</span>
                      <div>
                        <strong>{dayLabel(items[0]?.dtstart || "", today)}</strong>
                        <span className="muted">{day.toLocaleDateString(undefined, { month: "long" })}</span>
                      </div>
                    </header>
                    <div className="cal-agenda-items">
                      {items.map((ev) => {
                        const task = ev.kind === "task";
                        return (
                          <Link
                            key={`${ev.kind || "event"}-${ev.calendar_id}-${ev.href}`}
                            href={task ? "/app/tasks" : "/app/calendars"}
                            className={`cal-agenda-item${task ? " task" : ""}`}
                          >
                            <span className="cal-agenda-time">{task ? "Due" : eventTimeLabel(ev.dtstart, ev.all_day)}</span>
                            <span className="cal-swatch" style={chipStyle(ev.color)} />
                            <span className="cal-agenda-copy">
                              <strong>{ev.summary || (task ? "Task" : "Event")}</strong>
                              <span className="muted">
                                {ev.calendar_name}
                                {ev.location ? ` · ${ev.location}` : ""}
                              </span>
                            </span>
                          </Link>
                        );
                      })}
                    </div>
                  </section>
                );
              })}
            </div>
          )}
        </section>

        <div className="home-side">
          {pending.length > 0 ? (
            <section className="home-panel">
              <div className="home-panel-head">
                <h2>Waiting on you</h2>
                <Link href="/app/invitations">All invites</Link>
              </div>
              <div className="home-invites">
                {acceptCals.length > 1 ? (
                  <div className="field">
                    <span>Add accepted events to</span>
                    <select value={targetCal} onChange={(e) => setTargetCal(Number(e.target.value))}>
                      {acceptCals.map((c) => (
                        <option key={c.id} value={c.id}>
                          {c.name}
                        </option>
                      ))}
                    </select>
                  </div>
                ) : null}
                {pending.map((it) => (
                  <div key={it.id} className="home-invite">
                    <div>
                      <strong>{it.summary || "Invitation"}</strong>
                      <span className="muted">
                        {displayIdentity(it.organizer)}
                        {it.dtstart ? ` · ${dayLabel(it.dtstart, today)}` : ""}
                      </span>
                    </div>
                    <div className="btn-row">
                      <button className="btn sm" type="button" disabled={!targetCal} onClick={() => act(it, "accept")}>
                        <Check size={14} aria-hidden />
                        Accept
                      </button>
                      <button className="btn ghost sm" type="button" onClick={() => act(it, "decline")}>
                        <X size={14} aria-hidden />
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            </section>
          ) : null}

          <section className="home-panel">
            <div className="home-panel-head">
              <h2>Calendars</h2>
              <Link href="/app/calendars">Manage</Link>
            </div>
            {calendars.length === 0 ? (
              <p className="muted">No calendars yet.</p>
            ) : (
              <div className="home-cals">
                {calendars.map((c) => (
                  <Link key={c.id} className="home-cal" href="/app/calendars">
                    <span className="cal-swatch" style={chipStyle(c.color)} />
                    <span>
                      {c.name}
                      {c.shared ? <span className="muted"> · {c.owner_username}</span> : null}
                    </span>
                    {c.kind === "important_dates" ? <span className="pill">System</span> : null}
                  </Link>
                ))}
              </div>
            )}
          </section>

          <section className="home-panel">
            <div className="home-panel-head">
              <h2>Workspace</h2>
            </div>
            <div className="home-status">
              <Link href="/app/account">
                <span>
                  <ShieldCheck size={15} aria-hidden />
                  Authenticator
                </span>
                <span className={`pill ${overview?.totp_enabled || user.totp_enabled ? "ok" : ""}`}>
                  {overview?.totp_enabled || user.totp_enabled ? "On" : "Off"}
                </span>
              </Link>
              <Link href="/app/settings">
                <span>
                  <CalendarDays size={15} aria-hidden />
                  Important Dates
                </span>
                <span className={`pill ${overview?.important_dates_enabled ? "ok" : ""}`}>
                  {overview?.important_dates_enabled ? "On" : "Off"}
                </span>
              </Link>
              <Link href="/app/settings">
                <span>
                  <KeyRound size={15} aria-hidden />
                  Sync username
                </span>
                <span className="muted">{setup?.username || user.username}</span>
              </Link>
            </div>
          </section>

          <section className="home-panel">
            <div className="home-panel-head">
              <h2>Connect a client</h2>
              <Link href="/app/settings">All URLs</Link>
            </div>
            <p className="muted">Point a phone or desktop calendar app at this host. Use an app password from Account.</p>
            {setup ? (
              <>
                <CopyField label="CalDAV" value={setup.caldav_well_known} />
                <CopyField label="CardDAV" value={setup.carddav_well_known} />
                {setup.scheduling_address ? <CopyField label="Invite address" value={setup.scheduling_address} /> : null}
              </>
            ) : (
              <p className="muted">Loading connection details…</p>
            )}
          </section>
        </div>
      </div>
    </>
  );
}

function Stat({
  href,
  icon,
  label,
  value,
  children,
}: {
  href: string;
  icon: ReactNode;
  label: string;
  value: ReactNode;
  children?: ReactNode;
}) {
  return (
    <Link className="card stat" href={href}>
      <span className="stat-icon">{icon}</span>
      <span className="muted">{label}</span>
      <strong>{value}</strong>
      {children ? <span className="stat-hint">{children}</span> : null}
    </Link>
  );
}
