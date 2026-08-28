"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { createContext, useContext, useEffect, useState } from "react";
import { LogOut, Menu, X } from "lucide-react";
import { api, type Overview, type User } from "@/lib/api";
import { roleLabel } from "@/lib/format";

type Session = {
  user: User;
  overview: Overview | null;
  setUser: (user: User) => void;
  refreshOverview: () => Promise<void>;
};

const SessionContext = createContext<Session | null>(null);

export function useSession(): Session {
  const ctx = useContext(SessionContext);
  if (!ctx) throw new Error("useSession must be used within the app shell");
  return ctx;
}

function navActive(pathname: string, href: string, exact?: boolean) {
  return exact ? pathname === href : pathname.startsWith(href);
}

export default function AppShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const [user, setUser] = useState<User | null>(null);
  const [overview, setOverview] = useState<Overview | null>(null);
  const [loading, setLoading] = useState(true);
  const [navOpen, setNavOpen] = useState(false);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const [me, ov] = await Promise.all([api.me(), api.overview().catch(() => null)]);
        if (!cancelled) {
          setUser(me);
          setOverview(ov);
        }
      } catch {
        if (!cancelled) router.replace("/login");
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [router]);

  useEffect(() => {
    setNavOpen(false);
  }, [pathname]);

  useEffect(() => {
    if (!navOpen) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") setNavOpen(false);
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [navOpen]);

  async function logout() {
    await api.logout().catch(() => undefined);
    router.replace("/login");
  }

  async function refreshOverview() {
    try {
      setOverview(await api.overview());
    } catch {
      /* still signed in; badge can lag */
    }
  }

  if (loading || !user) {
    return (
      <div className="main">
        <p className="muted">Loading workspace…</p>
      </div>
    );
  }

  const workspace = [
    { href: "/app", label: "Overview", exact: true },
    { href: "/app/calendars", label: "Calendars" },
    { href: "/app/tasks", label: "Tasks" },
    { href: "/app/contacts", label: "Contacts" },
    { href: "/app/invitations", label: "Invitations", badge: overview?.pending_invitations ?? 0 },
  ];

  return (
    <SessionContext.Provider value={{ user, overview, setUser, refreshOverview }}>
      <div className={`app-shell${navOpen ? " nav-open" : ""}`}>
        <header className="mobile-bar">
          <button
            className="btn ghost sm icon-btn"
            type="button"
            aria-label={navOpen ? "Close menu" : "Open menu"}
            aria-expanded={navOpen}
            onClick={() => setNavOpen((v) => !v)}
          >
            {navOpen ? <X size={18} /> : <Menu size={18} />}
          </button>
          <Link className="brand" href="/app">
            dCalCon
          </Link>
          <Link className="mobile-user" href="/app/account">
            {user.username}
          </Link>
        </header>
        {navOpen ? (
          <button className="nav-scrim" type="button" aria-label="Close menu" onClick={() => setNavOpen(false)} />
        ) : null}
        <aside className="sidebar">
          <Link className="brand" href="/app">
            dCalCon
          </Link>
          <div className="nav-group">
            <div className="nav-label">Workspace</div>
            <nav>
              {workspace.map((it) => (
                <Link
                  key={it.href}
                  href={it.href}
                  className={navActive(pathname, it.href, it.exact) ? "active" : ""}
                >
                  <span>{it.label}</span>
                  {it.badge ? <span className="badge">{it.badge}</span> : null}
                </Link>
              ))}
            </nav>
          </div>
          {user.role === "admin" ? (
            <div className="nav-group">
              <div className="nav-label">Administration</div>
              <nav>
                <Link href="/app/admin/users" className={pathname === "/app/admin/users" || pathname === "/app/admin" ? "active" : ""}>
                  Users
                </Link>
                <Link href="/app/admin/outbox" className={pathname.startsWith("/app/admin/outbox") ? "active" : ""}>
                  Recovery mail
                </Link>
              </nav>
            </div>
          ) : null}
          <div className="sidebar-end">
            <nav>
              <Link href="/app/settings" className={pathname.startsWith("/app/settings") ? "active" : ""}>
                Settings
              </Link>
            </nav>
          </div>
          <div className="side-user">
            <Link
              href="/app/account"
              className={`account-card${pathname.startsWith("/app/account") ? " active" : ""}`}
            >
              <strong>{user.username}</strong>
              <span className="muted">{roleLabel(user.role)}</span>
            </Link>
            <button className="btn secondary sm" type="button" onClick={logout}>
              <LogOut size={14} aria-hidden />
              Sign out
            </button>
          </div>
        </aside>
        <div className="main">{children}</div>
      </div>
    </SessionContext.Provider>
  );
}
