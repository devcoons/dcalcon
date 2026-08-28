"use client";

import { FormEvent, useEffect, useState, type ReactNode } from "react";
import {
  Briefcase,
  CalendarDays,
  ChevronDown,
  ChevronRight,
  Download,
  Globe,
  Mail,
  MapPin,
  MessageCircle,
  Phone,
  Plus,
  StickyNote,
  Tags,
  Trash2,
  UserRound,
} from "lucide-react";
import {
  api,
  type Contact,
  type ContactAddress,
  type ContactWrite,
  type CustomField,
  type TypedValue,
} from "@/lib/api";
import { Modal } from "@/lib/ui";

const emailTypes = ["work", "home", "other"];
const telTypes = ["cell", "home", "work", "fax", "other"];
const urlTypes = ["work", "home", "other"];
const addrTypes = ["home", "work", "other"];

type Section = "name" | "contact" | "address" | "work" | "dates" | "notes" | "more";

type FormState = {
  fn: string;
  nickname: string;
  given_name: string;
  family_name: string;
  additional_name: string;
  prefix: string;
  suffix: string;
  org: string;
  title: string;
  role: string;
  bday: string;
  anniversary: string;
  gender: string;
  note: string;
  categories: string;
  kind: string;
  lang: string;
  tz: string;
  emails: TypedValue[];
  tels: TypedValue[];
  urls: TypedValue[];
  impps: TypedValue[];
  addresses: ContactAddress[];
  custom: CustomField[];
};

function blank(): FormState {
  return {
    fn: "",
    nickname: "",
    given_name: "",
    family_name: "",
    additional_name: "",
    prefix: "",
    suffix: "",
    org: "",
    title: "",
    role: "",
    bday: "",
    anniversary: "",
    gender: "",
    note: "",
    categories: "",
    kind: "",
    lang: "",
    tz: "",
    emails: [{ value: "", type: "work" }],
    tels: [{ value: "", type: "cell" }],
    urls: [],
    impps: [],
    addresses: [],
    custom: [],
  };
}

function copyTyped(v: TypedValue): TypedValue {
  return { value: v.value ?? "", type: v.type ?? "" };
}

function dateInput(v: string): string {
  const m = (v || "").match(/^(\d{4})-?(\d{2})-?(\d{2})/);
  return m ? `${m[1]}-${m[2]}-${m[3]}` : "";
}

function filled(rows: TypedValue[]): TypedValue[] {
  return rows.filter((r) => r.value.trim());
}

function fromContact(c: Contact): FormState {
  return {
    fn: c.fn ?? "",
    nickname: c.nickname ?? "",
    given_name: c.given_name ?? "",
    family_name: c.family_name ?? "",
    additional_name: c.additional_name ?? "",
    prefix: c.prefix ?? "",
    suffix: c.suffix ?? "",
    org: c.org ?? "",
    title: c.title ?? "",
    role: c.role ?? "",
    bday: dateInput(c.bday),
    anniversary: dateInput(c.anniversary),
    gender: c.gender ?? "",
    note: c.note ?? "",
    categories: c.categories ?? "",
    kind: c.kind ?? "",
    lang: c.lang ?? "",
    tz: c.tz ?? "",
    emails: c.emails?.length ? c.emails.map(copyTyped) : [{ value: c.email || "", type: "work" }],
    tels: c.tels?.length ? c.tels.map(copyTyped) : [{ value: c.tel || "", type: "cell" }],
    urls: (c.urls ?? []).map(copyTyped),
    impps: (c.impps ?? []).map(copyTyped),
    addresses: (c.addresses ?? []).map((a) => ({ ...a })),
    custom: (c.custom ?? []).map((x) => ({
      name: x.name.replace(/^X-/i, ""),
      value: x.value,
    })),
  };
}

function openFrom(f: FormState, isNew: boolean): Record<Section, boolean> {
  if (isNew) {
    return { name: true, contact: true, address: false, work: false, dates: false, notes: false, more: false };
  }
  return {
    name: true,
    contact: filled(f.emails).length > 0 || filled(f.tels).length > 0 || filled(f.urls).length > 0 || filled(f.impps).length > 0,
    address: f.addresses.length > 0,
    work: Boolean(f.org || f.title || f.role || f.kind),
    dates: Boolean(f.bday || f.anniversary),
    notes: Boolean(f.note.trim()),
    more: Boolean(f.gender || f.categories || f.lang || f.tz || f.custom.some((c) => c.name || c.value)),
  };
}

function toWrite(f: FormState): ContactWrite {
  const fn = f.fn.trim() || [f.given_name, f.family_name].filter(Boolean).join(" ").trim();
  return {
    fn,
    nickname: f.nickname,
    given_name: f.given_name,
    family_name: f.family_name,
    additional_name: f.additional_name,
    prefix: f.prefix,
    suffix: f.suffix,
    org: f.org,
    title: f.title,
    role: f.role,
    bday: f.bday,
    anniversary: f.anniversary,
    gender: f.gender,
    note: f.note,
    categories: f.categories,
    kind: f.kind,
    lang: f.lang,
    tz: f.tz,
    emails: f.emails,
    tels: f.tels,
    urls: f.urls,
    impps: f.impps,
    addresses: f.addresses,
    custom: f.custom,
  };
}

export default function ContactDialog({
  bookId,
  href,
  readOnly,
  onClose,
  onSaved,
}: {
  bookId: number;
  href: string | null;
  readOnly?: boolean;
  onClose: () => void;
  onSaved: (msg: string) => void;
}) {
  const editing = href !== null;
  const [form, setForm] = useState<FormState>(blank);
  const [open, setOpen] = useState<Record<Section, boolean>>(openFrom(blank(), true));
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(editing);

  useEffect(() => {
    if (!href) {
      const next = blank();
      setForm(next);
      setOpen(openFrom(next, true));
      setLoading(false);
      return;
    }
    setLoading(true);
    api
      .getContact(bookId, href)
      .then((c) => {
        const next = fromContact(c);
        setForm(next);
        setOpen(openFrom(next, false));
      })
      .catch((e) => setErr(e instanceof Error ? e.message : "Could not load contact."))
      .finally(() => setLoading(false));
  }, [bookId, href]);

  function patch<K extends keyof FormState>(key: K, value: FormState[K]) {
    setForm((f) => ({ ...f, [key]: value }));
  }

  function toggle(id: Section) {
    setOpen((s) => ({ ...s, [id]: !s[id] }));
  }

  async function save(e: FormEvent) {
    e.preventDefault();
    if (readOnly) return;
    const body = toWrite(form);
    if (!body.fn.trim()) {
      setErr("Display name is required.");
      return;
    }
    setBusy(true);
    setErr("");
    try {
      if (editing && href) {
        await api.updateContact(bookId, href, body);
        onSaved("Contact updated.");
      } else {
        await api.createContact(bookId, body);
        onSaved("Contact saved.");
      }
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : "Could not save contact.");
    } finally {
      setBusy(false);
    }
  }

  const contactHint = [
    filled(form.emails).length ? `${filled(form.emails).length} email${filled(form.emails).length === 1 ? "" : "s"}` : "",
    filled(form.tels).length ? `${filled(form.tels).length} phone${filled(form.tels).length === 1 ? "" : "s"}` : "",
    filled(form.urls).length ? `${filled(form.urls).length} web` : "",
    filled(form.impps).length ? `${filled(form.impps).length} chat` : "",
  ]
    .filter(Boolean)
    .join(" · ");

  return (
    <Modal
      title={readOnly ? "Contact" : editing ? "Edit contact" : "Add contact"}
      lede={
        readOnly
          ? "This contact comes from an account on this server."
          : "Name and how to reach them. Open a section for the rest."
      }
      icon={<UserRound size={22} aria-hidden="true" />}
      size="lg"
      onClose={onClose}
      footer={
        <div className="form-actions">
          {!readOnly ? (
            <button className="btn" form="contact-form" type="submit" disabled={busy || loading}>
              {busy ? "Saving…" : editing ? "Save changes" : "Save contact"}
            </button>
          ) : null}
          {editing && href ? (
            <button
              className="btn secondary sm"
              type="button"
              disabled={busy || loading}
              onClick={() => {
                api.exportContact(bookId, href).catch((ex) => {
                  setErr(ex instanceof Error ? ex.message : "Could not download contact.");
                });
              }}
            >
              <Download size={16} aria-hidden />
              Download
            </button>
          ) : null}
          <button className="btn secondary sm" type="button" onClick={onClose}>
            Close
          </button>
        </div>
      }
    >
          <form id="contact-form" onSubmit={save}>
          <fieldset className="modal-form" disabled={readOnly}>
          {loading ? <p className="muted">Loading…</p> : null}
          {err ? <div className="banner err">{err}</div> : null}

          <Accordion
            title="Name"
            icon={<UserRound size={18} />}
            hint={form.fn || [form.given_name, form.family_name].filter(Boolean).join(" ")}
            open={open.name}
            onToggle={() => toggle("name")}
          >
            <div className="row">
              <Field label="Display name" value={form.fn} onChange={(v) => patch("fn", v)} required icon={<UserRound size={16} />} />
              <Field label="Given name" value={form.given_name} onChange={(v) => patch("given_name", v)} />
              <Field label="Family name" value={form.family_name} onChange={(v) => patch("family_name", v)} />
            </div>
            <div className="row">
              <Field label="Prefix" value={form.prefix} onChange={(v) => patch("prefix", v)} />
              <Field label="Additional" value={form.additional_name} onChange={(v) => patch("additional_name", v)} />
              <Field label="Suffix" value={form.suffix} onChange={(v) => patch("suffix", v)} />
              <Field label="Nickname" value={form.nickname} onChange={(v) => patch("nickname", v)} />
            </div>
          </Accordion>

          <Accordion
            title="Contact"
            icon={<Mail size={18} />}
            hint={contactHint}
            open={open.contact}
            onToggle={() => toggle("contact")}
          >
            <TypedList
              label="Email"
              icon={<Mail size={16} />}
              types={emailTypes}
              rows={form.emails}
              inputType="email"
              onChange={(rows) => patch("emails", rows)}
              onAdd={() => patch("emails", [...form.emails, { value: "", type: "work" }])}
            />
            <TypedList
              label="Phone"
              icon={<Phone size={16} />}
              types={telTypes}
              rows={form.tels}
              onChange={(rows) => patch("tels", rows)}
              onAdd={() => patch("tels", [...form.tels, { value: "", type: "cell" }])}
            />
            <TypedList
              label="Website"
              icon={<Globe size={16} />}
              types={urlTypes}
              rows={form.urls}
              inputType="url"
              onChange={(rows) => patch("urls", rows)}
              onAdd={() => patch("urls", [...form.urls, { value: "", type: "work" }])}
            />
            <TypedList
              label="Chat"
              icon={<MessageCircle size={16} />}
              types={["xmpp", "sip", "other"]}
              rows={form.impps}
              placeholder="xmpp:user@example.com"
              onChange={(rows) => patch("impps", rows)}
              onAdd={() => patch("impps", [...form.impps, { value: "", type: "xmpp" }])}
            />
          </Accordion>

          <Accordion
            title="Addresses"
            icon={<MapPin size={18} />}
            hint={form.addresses.length ? `${form.addresses.length}` : ""}
            open={open.address}
            onToggle={() => toggle("address")}
          >
            <div className="repeat">
              {form.addresses.map((a, i) => (
                <div className="addr-block" key={i}>
                  <div className="row">
                    <div className="field">
                      <span>Type</span>
                      <select
                        value={a.type ?? "home"}
                        onChange={(e) => {
                          const next = form.addresses.slice();
                          next[i] = { ...a, type: e.target.value };
                          patch("addresses", next);
                        }}
                      >
                        {addrTypes.map((t) => (
                          <option key={t}>{t}</option>
                        ))}
                      </select>
                    </div>
                    <Field
                      label="Street"
                      value={a.street ?? ""}
                      onChange={(v) => {
                        const next = form.addresses.slice();
                        next[i] = { ...a, street: v };
                        patch("addresses", next);
                      }}
                    />
                  </div>
                  <div className="row">
                    <Field
                      label="City"
                      value={a.city ?? ""}
                      onChange={(v) => {
                        const next = form.addresses.slice();
                        next[i] = { ...a, city: v };
                        patch("addresses", next);
                      }}
                    />
                    <Field
                      label="Region"
                      value={a.region ?? ""}
                      onChange={(v) => {
                        const next = form.addresses.slice();
                        next[i] = { ...a, region: v };
                        patch("addresses", next);
                      }}
                    />
                    <Field
                      label="Postal code"
                      value={a.postal_code ?? ""}
                      onChange={(v) => {
                        const next = form.addresses.slice();
                        next[i] = { ...a, postal_code: v };
                        patch("addresses", next);
                      }}
                    />
                    <Field
                      label="Country"
                      value={a.country ?? ""}
                      onChange={(v) => {
                        const next = form.addresses.slice();
                        next[i] = { ...a, country: v };
                        patch("addresses", next);
                      }}
                    />
                  </div>
                  <div className="form-actions">
                    <button
                      className="btn ghost sm icon-btn danger"
                      type="button"
                      aria-label="Remove address"
                      onClick={() => patch("addresses", form.addresses.filter((_, j) => j !== i))}
                    >
                      <Trash2 size={16} />
                    </button>
                  </div>
                </div>
              ))}
              <button
                className="btn secondary sm"
                type="button"
                onClick={() => patch("addresses", [...form.addresses, { type: "home", street: "", city: "", country: "" }])}
              >
                <Plus size={14} /> Add address
              </button>
            </div>
          </Accordion>

          <Accordion
            title="Work"
            icon={<Briefcase size={18} />}
            hint={form.org || form.title || ""}
            open={open.work}
            onToggle={() => toggle("work")}
          >
            <div className="row">
              <Field label="Organization" value={form.org} onChange={(v) => patch("org", v)} />
              <Field label="Title" value={form.title} onChange={(v) => patch("title", v)} />
              <Field label="Role" value={form.role} onChange={(v) => patch("role", v)} />
              <div className="field">
                <span>This is a</span>
                <select value={form.kind} onChange={(e) => patch("kind", e.target.value)}>
                  <option value="">Person</option>
                  <option value="org">Organization</option>
                  <option value="group">Group</option>
                </select>
              </div>
            </div>
          </Accordion>

          <Accordion
            title="Dates"
            icon={<CalendarDays size={18} />}
            hint={form.bday || form.anniversary || ""}
            open={open.dates}
            onToggle={() => toggle("dates")}
          >
            <div className="row">
              <div className="field">
                <span>Birthday</span>
                <input type="date" value={form.bday} onChange={(e) => patch("bday", e.target.value)} />
              </div>
              <div className="field">
                <span>Anniversary</span>
                <input type="date" value={form.anniversary} onChange={(e) => patch("anniversary", e.target.value)} />
              </div>
            </div>
          </Accordion>

          <Accordion
            title="Notes"
            icon={<StickyNote size={18} />}
            open={open.notes}
            onToggle={() => toggle("notes")}
          >
            <div className="field">
              <span>Note</span>
              <textarea value={form.note} onChange={(e) => patch("note", e.target.value)} rows={3} />
            </div>
          </Accordion>

          <Accordion
            title="More"
            icon={<Tags size={18} />}
            hint={form.custom.filter((c) => c.name && c.value).length ? `${form.custom.filter((c) => c.name && c.value).length} custom` : ""}
            open={open.more}
            onToggle={() => toggle("more")}
          >
            <div className="row">
              <div className="field">
                <span>Gender</span>
                <select value={form.gender} onChange={(e) => patch("gender", e.target.value)}>
                  <option value="">Unspecified</option>
                  <option value="F">Female</option>
                  <option value="M">Male</option>
                  <option value="O">Other</option>
                  <option value="N">None</option>
                  <option value="U">Unknown</option>
                </select>
              </div>
              <Field
                label="Categories"
                value={form.categories}
                onChange={(v) => patch("categories", v)}
                placeholder="Family, work"
              />
              <Field label="Language" value={form.lang} onChange={(v) => patch("lang", v)} placeholder="en" />
              <Field label="Timezone" value={form.tz} onChange={(v) => patch("tz", v)} placeholder="Europe/Athens" />
            </div>
            <p className="muted">Extra fields stay with the contact when it syncs to other apps.</p>
            <div className="repeat">
              {form.custom.map((c, i) => (
                <div className="repeat-row pair" key={i}>
                  <div className="field">
                    <span>Name</span>
                    <input
                      value={c.name}
                      placeholder="Department"
                      onChange={(e) => {
                        const next = form.custom.slice();
                        next[i] = { ...c, name: e.target.value };
                        patch("custom", next);
                      }}
                    />
                  </div>
                  <div className="field">
                    <span>Value</span>
                    <input
                      value={c.value}
                      onChange={(e) => {
                        const next = form.custom.slice();
                        next[i] = { ...c, value: e.target.value };
                        patch("custom", next);
                      }}
                    />
                  </div>
                  <button
                    className="btn ghost sm icon-btn danger"
                    type="button"
                    aria-label="Remove field"
                    onClick={() => patch("custom", form.custom.filter((_, j) => j !== i))}
                  >
                    <Trash2 size={16} />
                  </button>
                </div>
              ))}
              <button
                className="btn secondary sm"
                type="button"
                onClick={() => patch("custom", [...form.custom, { name: "", value: "" }])}
              >
                <Plus size={14} /> Add custom field
              </button>
            </div>
          </Accordion>
          </fieldset>
          </form>
    </Modal>
  );
}

function Accordion({
  title,
  icon,
  hint,
  open,
  onToggle,
  children,
}: {
  title: string;
  icon: ReactNode;
  hint?: string;
  open: boolean;
  onToggle: () => void;
  children: ReactNode;
}) {
  return (
    <div className={open ? "accordion open" : "accordion"}>
      <button type="button" className="accordion-toggle" onClick={onToggle} aria-expanded={open}>
        <span className="accordion-meta">
          <span aria-hidden="true">{icon}</span>
          <span>{title}</span>
        </span>
        <span className="accordion-right">
          {hint ? <span className="accordion-hint">{hint}</span> : null}
          {open ? <ChevronDown className="accordion-chevron" size={18} /> : <ChevronRight className="accordion-chevron" size={18} />}
        </span>
      </button>
      {open ? <div className="accordion-body">{children}</div> : null}
    </div>
  );
}

function Field({
  label,
  value,
  onChange,
  required,
  placeholder,
  icon,
  type,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  required?: boolean;
  placeholder?: string;
  icon?: ReactNode;
  type?: string;
}) {
  return (
    <div className="field">
      <span>{label}</span>
      {icon ? (
        <div className="input-icon">
          <span className="input-icon-mark" aria-hidden>
            {icon}
          </span>
          <input
            type={type}
            value={value}
            onChange={(e) => onChange(e.target.value)}
            required={required}
            placeholder={placeholder}
          />
        </div>
      ) : (
        <input
          type={type}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          required={required}
          placeholder={placeholder}
        />
      )}
    </div>
  );
}

function TypedList({
  label,
  icon,
  types,
  rows,
  onChange,
  onAdd,
  inputType,
  placeholder,
}: {
  label: string;
  icon?: ReactNode;
  types: string[];
  rows: TypedValue[];
  onChange: (rows: TypedValue[]) => void;
  onAdd: () => void;
  inputType?: string;
  placeholder?: string;
}) {
  return (
    <div className="repeat">
      <div className="section-label">
        {icon}
        {label}s
      </div>
      {rows.map((row, i) => (
        <div className="repeat-row" key={i}>
          <div className="field">
            <span>Type</span>
            <select
              value={row.type ?? ""}
              onChange={(e) => {
                const next = rows.slice();
                next[i] = { ...row, type: e.target.value };
                onChange(next);
              }}
            >
              {types.map((t) => (
                <option key={t} value={t}>
                  {t}
                </option>
              ))}
            </select>
          </div>
          <div className="field">
            <span>{label}</span>
            <div className="input-icon">
              {icon ? (
                <span className="input-icon-mark" aria-hidden>
                  {icon}
                </span>
              ) : null}
              <input
                type={inputType}
                value={row.value}
                placeholder={placeholder}
                onChange={(e) => {
                  const next = rows.slice();
                  next[i] = { ...row, value: e.target.value };
                  onChange(next);
                }}
              />
            </div>
          </div>
          <button
            className="btn ghost sm icon-btn danger"
            type="button"
            aria-label={`Remove ${label}`}
            onClick={() => onChange(rows.filter((_, j) => j !== i))}
          >
            <Trash2 size={16} />
          </button>
        </div>
      ))}
      <button className="btn secondary sm" type="button" onClick={onAdd}>
        <Plus size={14} /> Add {label.toLowerCase()}
      </button>
    </div>
  );
}
