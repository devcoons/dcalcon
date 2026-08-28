export default function HomePage() {
  return (
    <div className="landing">
      <div className="hero">
        <div>
          <p className="pill">CalDAV · CardDAV · SQLite</p>
          <h1>Calendars and contacts that desktop and phone clients already speak.</h1>
          <p className="lede">
            dCalCon is a self-hosted CalDAV and CardDAV server with a workspace for people
            and administrators. Edit events and contacts here, or sync GNOME Calendar,
            GNOME Contacts, and DAVx⁵ against the same store.
          </p>
          <div className="actions">
            <a className="btn" href="/login">
              Open workspace
            </a>
            <a className="btn secondary" href="/app">
              Dashboard
            </a>
          </div>
        </div>
        <div className="panel">
          <h2>Client setup</h2>
          <p className="muted">Add one account on this host. Discovery uses RFC 6764 well-known URLs.</p>
          <p>
            CalDAV <code>/.well-known/caldav</code>
            <br />
            CardDAV <code>/.well-known/carddav</code>
          </p>
          <p className="muted">
            HTTP Basic for DAV clients. Session cookie for this dashboard. Users are created by an administrator.
          </p>
        </div>
      </div>
      <div className="grid-3">
        <div className="panel">
          <h2>Calendars</h2>
          <p>Personal calendars, events, and a server-built Important Dates calendar from contact birthdays.</p>
        </div>
        <div className="panel">
          <h2>Contacts</h2>
          <p>Address books with vCard 3/4. Birthdays and anniversaries feed yearly reminders when enabled.</p>
        </div>
        <div className="panel">
          <h2>Sharing</h2>
          <p>Share a calendar with another local user, invite them to an event, or email a guest from a connected mailbox.</p>
        </div>
      </div>
    </div>
  );
}
