"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { Download, Pencil, Plus, Search, Trash2, Upload } from "lucide-react";
import { api, type AddressBook, type Contact } from "@/lib/api";
import ContactDialog from "@/lib/contact-dialog";
import { IconBtn, Notices, PageHeader, useNotice } from "@/lib/ui";
import { useSession } from "@/lib/shell";

export default function ContactsPage() {
  const { refreshOverview } = useSession();
  const notice = useNotice();
  const [books, setBooks] = useState<AddressBook[]>([]);
  const [book, setBook] = useState<AddressBook | null>(null);
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [q, setQ] = useState("");
  const [dialog, setDialog] = useState<null | { href: string | null }>(null);
  const [busy, setBusy] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);
  const loadGen = useRef(0);
  const [localDomain, setLocalDomain] = useState("dcalcon.private");

  async function load(preferred?: AddressBook | null) {
    const gen = ++loadGen.current;
    const list = await api.addressbooks();
    if (gen !== loadGen.current) return;
    setBooks(list);
    const next =
      (preferred && list.find((b) => b.id === preferred.id)) ||
      (book && list.find((b) => b.id === book.id)) ||
      list.find((b) => b.slug === "contacts") ||
      list[0] ||
      null;
    setBook(next);
    if (!next) {
      setContacts([]);
      return;
    }
    const items = await api.contacts(next.id);
    if (gen !== loadGen.current) return;
    setContacts(items);
  }

  useEffect(() => {
    api
      .setup()
      .then((s) => {
        if (s.scheduling_domain) setLocalDomain(s.scheduling_domain);
      })
      .catch(() => undefined);
    load().catch((e) => notice.fail(e instanceof Error ? e.message : "Could not load contacts."));
  }, []);

  const readOnly = Boolean(book?.read_only || book?.slug === "people");

  const filtered = useMemo(() => {
    const s = q.trim().toLowerCase();
    if (!s) return contacts;
    return contacts.filter((c) => {
      const blob = [c.fn, c.email, c.tel, c.org, c.nickname, ...(c.emails ?? []).map((e) => e.value)]
        .join(" ")
        .toLowerCase();
      return blob.includes(s);
    });
  }, [contacts, q]);

  async function remove(href: string) {
    if (!book || readOnly) return;
    if (!window.confirm("Delete this contact?")) return;
    try {
      await api.deleteContact(book.id, href);
      if (dialog?.href === href) setDialog(null);
      await load();
      await refreshOverview();
      notice.done("Contact deleted.");
    } catch (ex) {
      notice.fail(ex instanceof Error ? ex.message : "Could not delete contact.");
    }
  }

  async function downloadVCard(href?: string) {
    if (!book) return;
    try {
      if (href) await api.exportContact(book.id, href);
      else await api.exportContacts(book.id);
    } catch (ex) {
      notice.fail(ex instanceof Error ? ex.message : "Could not export.");
    }
  }

  async function importFiles(files: FileList | null) {
    if (!book || readOnly || !files?.length) return;
    setBusy(true);
    try {
      const result = { created: 0, updated: 0, skipped: 0, errors: [] as string[] };
      for (const f of Array.from(files)) {
        const zip = /\.zip$/i.test(f.name) || f.type === "application/zip" || f.type === "application/x-zip-compressed";
        const part = zip
          ? await api.importContacts(book.id, await f.arrayBuffer(), "application/zip")
          : await api.importContacts(book.id, await f.text(), "text/vcard");
        result.created += part.created;
        result.updated += part.updated;
        result.skipped += part.skipped;
        if (part.errors) result.errors.push(...part.errors);
      }
      await load();
      await refreshOverview();
      const bits: string[] = [];
      if (result.created) bits.push(`${result.created} created`);
      if (result.updated) bits.push(`${result.updated} updated`);
      if (result.skipped) bits.push(`${result.skipped} skipped`);
      const extra = result.errors?.length ? " " + result.errors.slice(0, 3).join(" ") : "";
      const msg = (bits.join(", ") || "No contacts imported") + "." + extra;
      if (result.created + result.updated === 0) notice.fail(msg.trim());
      else notice.done(msg);
    } catch (ex) {
      notice.fail(ex instanceof Error ? ex.message : "Could not import contacts.");
    } finally {
      setBusy(false);
      if (fileRef.current) fileRef.current.value = "";
    }
  }

  return (
    <>
      <PageHeader
        title="Contacts"
        lede={
          readOnly
            ? `People on this server. Invite them from any calendar app as username@${localDomain}.`
            : "Add contacts, or import and export vCards (or a .zip of them)."
        }
      >
        <div className="btn-row">
          <div className="search-field">
            <Search size={16} aria-hidden />
            <input placeholder="Search" value={q} onChange={(e) => setQ(e.target.value)} aria-label="Search contacts" />
          </div>
          <input
            ref={fileRef}
            type="file"
            accept=".vcf,.vcard,.zip,text/vcard,text/x-vcard,application/zip"
            multiple
            hidden
            onChange={(e) => importFiles(e.target.files)}
          />
          {!readOnly ? (
            <button className="btn secondary" type="button" disabled={!book || busy} onClick={() => fileRef.current?.click()}>
              <Upload size={16} aria-hidden />
              {busy ? "Importing…" : "Import"}
            </button>
          ) : null}
          <button className="btn secondary" type="button" disabled={!book || contacts.length === 0} onClick={() => downloadVCard()}>
            <Download size={16} aria-hidden />
            Export
          </button>
          {!readOnly ? (
            <button className="btn" type="button" disabled={!book} onClick={() => setDialog({ href: null })}>
              <Plus size={16} aria-hidden />
              Add contact
            </button>
          ) : null}
        </div>
      </PageHeader>
      {books.length > 1 ? (
        <div className="chip-list" style={{ marginBottom: "1rem" }}>
          {books.map((b) => (
            <button
              key={b.id}
              type="button"
              className={`chip${book?.id === b.id ? " on" : ""}`}
              onClick={() => {
                setBook(b);
                setContacts([]);
                load(b).catch((e) => notice.fail(e instanceof Error ? e.message : "Could not load contacts."));
              }}
            >
              {b.name}
            </button>
          ))}
        </div>
      ) : null}
      <Notices notice={notice} />
      {filtered.length === 0 ? (
        <div className="empty">
          {contacts.length === 0
            ? readOnly
              ? "No other people on this server yet."
              : "No contacts yet. Add one, or import a .vcf file or a .zip of vCards."
            : "No contacts match that search."}
        </div>
      ) : (
        <div className="table-wrap">
        <table className="data">
          <thead>
            <tr>
              <th>Name</th>
              <th>Email</th>
              <th className="hide-narrow">Phone</th>
              <th className="hide-narrow">Birthday</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {filtered.map((c) => (
              <tr key={c.href} className="row-link" onClick={() => setDialog({ href: c.href })}>
                <td>
                  {c.fn || c.uid}
                  {c.org ? <div className="muted">{c.title ? `${c.title} · ${c.org}` : c.org}</div> : null}
                </td>
                <td className="muted">{c.email || "—"}</td>
                <td className="muted hide-narrow">{c.tel || "—"}</td>
                <td className="muted hide-narrow">{c.bday || "—"}</td>
                <td>
                  <div className="btn-row">
                    <IconBtn label="Export vCard" onClick={() => downloadVCard(c.href)}>
                      <Download size={16} />
                    </IconBtn>
                    <IconBtn label={readOnly ? "View" : "Edit"} onClick={() => setDialog({ href: c.href })}>
                      <Pencil size={16} />
                    </IconBtn>
                    {!readOnly ? (
                      <IconBtn label="Delete" danger onClick={() => remove(c.href)}>
                        <Trash2 size={16} />
                      </IconBtn>
                    ) : null}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        </div>
      )}
      {dialog && book ? (
        <ContactDialog
          bookId={book.id}
          href={dialog.href}
          readOnly={readOnly}
          onClose={() => setDialog(null)}
          onSaved={async (msg) => {
            notice.done(msg);
            setDialog(null);
            await load();
            await refreshOverview();
          }}
        />
      ) : null}
    </>
  );
}
