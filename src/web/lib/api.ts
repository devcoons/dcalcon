export type RecoveryMessage = {
  id: number;
  user_id: number;
  username?: string;
  email: string;
  delivered: string;
  last_error?: string;
  created_at: string;
};

export type User = {
  id: number;
  username: string;
  email: string;
  display_name: string;
  role: string;
  status: string;
  timezone: string;
  totp_enabled?: boolean;
};

export type DirectoryUser = {
  id: number;
  username: string;
  display_name: string;
  local_email?: string;
};

export type Calendar = {
  id: number;
  slug: string;
  name: string;
  description: string;
  color: string;
  kind: string;
  read_only: boolean;
  shared?: boolean;
  access?: string;
  owner_username?: string;
};

export type CalendarShare = {
  id: number;
  calendar_id: number;
  user_id: number;
  username: string;
  display_name: string;
  access: string;
  created_at: string;
};

export type Attendee = {
  value: string;
  cn: string;
  partstat: string;
};

export type Attachment = {
  id: string;
  filename: string;
  content_type: string;
  size: number;
  created_at?: string;
};

export type EventItem = {
  href: string;
  uid: string;
  summary: string;
  description: string;
  location?: string;
  dtstart: string;
  dtend: string;
  all_day?: boolean;
  rrule?: string;
  alarm_minutes?: number;
  attendees?: Attendee[];
  attachments?: Attachment[];
};

export type EventWrite = {
  summary: string;
  description?: string;
  location?: string;
  dtstart: string;
  dtend?: string;
  all_day?: boolean;
  rrule?: string;
  alarm_minutes?: number;
  invite?: string[];
  invite_emails?: string[];
};

export type TaskItem = {
  href: string;
  uid: string;
  summary: string;
  description: string;
  due: string;
  status: string;
  calendar_id: number;
  calendar_name: string;
  attachments?: Attachment[];
};

export type BusyPeriod = { start: string; end: string };

export type WebcalInfo = { enabled: boolean; token?: string; url?: string };

export type AuditEntry = {
  id: number;
  at: string;
  actor: string;
  action: string;
  detail: string;
};

export type InviteResult = {
  local: number;
  email: number;
  missing?: string[];
  mail_error?: string;
};

export type AppPassword = {
  id: number;
  name: string;
  prefix: string;
  created_at: string;
  last_used_at?: string;
};

export type AppPasswordCreated = AppPassword & { password: string };

export type MailStatus = {
  google_configured: boolean;
  microsoft_configured: boolean;
  server_smtp: boolean;
  token_key: boolean;
  google_callback_path: string;
  microsoft_callback_path: string;
};

export type ConnectedAccount = {
  id: number;
  provider: string;
  email: string;
  status: string;
  scopes?: string;
  last_error?: string;
};

export type AddressBook = {
  id: number;
  slug: string;
  name: string;
  description: string;
  read_only?: boolean;
};

export type TypedValue = {
  value: string;
  type?: string;
};

export type ContactAddress = {
  type?: string;
  po_box?: string;
  extended?: string;
  street?: string;
  city?: string;
  region?: string;
  postal_code?: string;
  country?: string;
};

export type CustomField = {
  name: string;
  value: string;
};

export type Contact = {
  href: string;
  uid: string;
  fn: string;
  nickname?: string;
  given_name?: string;
  family_name?: string;
  additional_name?: string;
  prefix?: string;
  suffix?: string;
  org?: string;
  title?: string;
  role?: string;
  email: string;
  tel: string;
  bday: string;
  anniversary: string;
  gender?: string;
  note?: string;
  categories?: string;
  kind?: string;
  lang?: string;
  tz?: string;
  emails?: TypedValue[];
  tels?: TypedValue[];
  urls?: TypedValue[];
  impps?: TypedValue[];
  addresses?: ContactAddress[];
  custom?: CustomField[];
};

export type ContactWrite = Omit<Contact, "href" | "uid" | "email" | "tel"> & {
  email?: string;
  tel?: string;
};

export type Invitation = {
  id: number;
  method: string;
  uid: string;
  summary: string;
  description?: string;
  location?: string;
  dtstart?: string;
  dtend?: string;
  organizer: string;
  attendee: string;
  status: string;
  created_at: string;
};

export type ImportantDates = {
  enabled: boolean;
  include_birthdays: boolean;
  include_anniversaries: boolean;
  alarm_offsets: string[];
};

export type Setup = {
  public_url: string;
  username: string;
  auth_method: string;
  caldav_well_known: string;
  carddav_well_known: string;
  principal_url: string;
  calendar_home: string;
  addressbook_home: string;
  scheduling_address?: string;
  scheduling_domain?: string;
};

export type OverviewCal = {
  id: number;
  name: string;
  color: string;
  kind: string;
  shared?: boolean;
  read_only?: boolean;
  access?: string;
  owner_username?: string;
};

export type OverviewEvent = {
  href: string;
  calendar_id: number;
  summary: string;
  location?: string;
  dtstart: string;
  dtend: string;
  all_day?: boolean;
  color: string;
  calendar_name: string;
  kind?: "event" | "task";
};

export type OverviewInvite = {
  id: number;
  summary: string;
  organizer: string;
  dtstart?: string;
};

export type Overview = {
  calendars: number;
  contacts: number;
  pending_invitations: number;
  important_dates_enabled: boolean;
  shared_calendars?: number;
  events_soon?: number;
  mail_connected?: boolean;
  mail_address?: string;
  totp_enabled?: boolean;
  calendar_list?: OverviewCal[];
  upcoming?: OverviewEvent[];
  pending?: OverviewInvite[];
};

export type CreatedUser = {
  user: User;
  setup: Setup;
};

export type ContactImportResult = {
  created: number;
  updated: number;
  skipped: number;
  errors?: string[];
};

export type BackupResult = {
  kind: string;
  calendars: number;
  objects: number;
  contacts: number;
  files: number;
  skipped?: string[];
};

async function downloadAuthed(path: string, fallbackName: string, init?: RequestInit) {
  const res = await fetch(path, authedInit(init));
  if (!res.ok) {
    throw await apiError(res);
  }
  const dispo = res.headers.get("Content-Disposition") || "";
  const m = /filename="([^"]+)"/.exec(dispo);
  const name = m?.[1] || fallbackName;
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

function authedInit(init?: RequestInit): RequestInit {
  const form = typeof FormData !== "undefined" && init?.body instanceof FormData;
  return {
    credentials: "include",
    ...init,
    headers: {
      Accept: "application/json",
      ...(init?.body && !form ? { "Content-Type": "application/json" } : {}),
      ...(init?.headers ?? {}),
    },
  };
}

async function apiError(res: Response, text?: string): Promise<ApiError> {
  const body = text ?? (await res.text());
  let msg = res.statusText;
  try {
    const j = JSON.parse(body) as { error?: string };
    if (j.error) msg = j.error;
  } catch {
    if (body) msg = body.trim();
  }
  if (res.status === 404 && /page not found/i.test(msg)) {
    msg = "The API on :8080 is older than this dashboard. Restart dcalcon serve, then reload.";
  }
  if (res.status >= 500 && /internal server error/i.test(msg)) {
    msg =
      "The API is not reachable. Start dCalCon on port 8080 (or rebuild the web app with API_URL pointing at the core).";
  }
  return new ApiError(res.status, msg);
}

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, authedInit(init));
  const text = await res.text();
  if (!res.ok) {
    throw await apiError(res, text);
  }
  if (!text || res.status === 204) return undefined as T;
  return JSON.parse(text) as T;
}

export const api = {
  login: (username: string, password?: string, totp?: string) =>
    req<User>("/api/v1/auth/login", {
      method: "POST",
      body: JSON.stringify({ username, password: password || undefined, totp: totp || undefined }),
    }),
  logout: () => req<{ status: string }>("/api/v1/auth/logout", { method: "POST" }),
  recover: (email: string) =>
    req<{ ok: boolean }>("/api/v1/auth/recover", { method: "POST", body: JSON.stringify({ email }) }),
  resetWithToken: (token: string, password: string) =>
    req("/api/v1/auth/reset", { method: "POST", body: JSON.stringify({ token, password }) }),
  resetWithTotp: (username: string, code: string, password: string) =>
    req("/api/v1/auth/reset-totp", { method: "POST", body: JSON.stringify({ username, code, password }) }),
  me: (init?: RequestInit) => req<User>("/api/v1/me", init),
  patchMe: (body: { display_name: string; email: string; timezone: string }) =>
    req<User>("/api/v1/me", { method: "PATCH", body: JSON.stringify(body) }),
  changePassword: (current_password: string, new_password: string) =>
    req("/api/v1/me/password", { method: "POST", body: JSON.stringify({ current_password, new_password }) }),
  totpSetup: () => req<{ secret: string; otpauth: string }>("/api/v1/me/totp/setup", { method: "POST", body: "{}" }),
  totpEnable: (code: string) =>
    req("/api/v1/me/totp/enable", { method: "POST", body: JSON.stringify({ code }) }),
  totpDisable: (body: { password?: string; code?: string }) =>
    req("/api/v1/me/totp/disable", { method: "POST", body: JSON.stringify(body) }),
  totpCancel: () => req("/api/v1/me/totp/setup", { method: "DELETE" }),
  revokeSessions: () => req("/api/v1/me/sessions/revoke", { method: "POST", body: "{}" }),
  exportTakeout: () => downloadAuthed("/api/v1/me/backup?kind=data", "dcalcon-data.zip"),
  exportBackup: (kind: "data" | "full", password?: string) =>
    downloadAuthed(`/api/v1/me/backup/export`, `dcalcon-${kind}.zip`, {
      method: "POST",
      body: JSON.stringify({ kind, password: password || undefined }),
    }),
  restoreBackup: (file: File, password?: string) => {
    const fd = new FormData();
    fd.append("file", file);
    if (password) fd.append("password", password);
    return req<BackupResult>("/api/v1/me/backup", { method: "POST", body: fd });
  },
  appPasswords: async () => (await req<AppPassword[] | null>("/api/v1/me/app-passwords")) ?? [],
  createAppPassword: (name: string) =>
    req<AppPasswordCreated>("/api/v1/me/app-passwords", { method: "POST", body: JSON.stringify({ name }) }),
  deleteAppPassword: (id: number) => req(`/api/v1/me/app-passwords/${id}`, { method: "DELETE" }),
  overview: (init?: RequestInit) => req<Overview>("/api/v1/overview", init),
  setup: () => req<Setup>("/api/v1/setup"),
  directory: async (init?: RequestInit) => (await req<DirectoryUser[] | null>("/api/v1/directory", init)) ?? [],
  calendars: async (init?: RequestInit) => (await req<Calendar[] | null>("/api/v1/calendars", init)) ?? [],
  createCalendar: (body: { name: string; description?: string; color?: string }) =>
    req<Calendar>("/api/v1/calendars", { method: "POST", body: JSON.stringify(body) }),
  patchCalendar: (id: number, body: { name?: string; description?: string; color?: string }) =>
    req<Calendar>(`/api/v1/calendars/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
  calendarShares: async (id: number) =>
    (await req<CalendarShare[] | null>(`/api/v1/calendars/${id}/shares`)) ?? [],
  shareCalendar: (id: number, username: string, access: string) =>
    req<CalendarShare[]>(`/api/v1/calendars/${id}/shares`, {
      method: "POST",
      body: JSON.stringify({ username, access }),
    }),
  unshareCalendar: (id: number, userId: number) =>
    req(`/api/v1/calendars/${id}/shares/${userId}`, { method: "DELETE" }),
  deleteCalendar: (id: number) => req(`/api/v1/calendars/${id}`, { method: "DELETE" }),
  events: async (id: number, init?: RequestInit) => (await req<EventItem[] | null>(`/api/v1/calendars/${id}/events`, init)) ?? [],
  createEvent: (id: number, body: EventWrite) =>
    req<{ href: string; uid: string; invite?: InviteResult }>(`/api/v1/calendars/${id}/events`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  updateEvent: (id: number, href: string, body: EventWrite) =>
    req<EventItem & { invite?: InviteResult }>(`/api/v1/calendars/${id}/events/${encodeURIComponent(href)}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  deleteEvent: (id: number, href: string) =>
    req(`/api/v1/calendars/${id}/events/${encodeURIComponent(href)}`, { method: "DELETE" }),
  exportCalendar: (id: number) => downloadAuthed(`/api/v1/calendars/${id}/export`, "calendar.ics"),
  importCalendar: (id: number, body: string) =>
    req<ContactImportResult>(`/api/v1/calendars/${id}/import`, {
      method: "POST",
      body,
      headers: { "Content-Type": "text/calendar", Accept: "application/json" },
    }),
  webcal: (id: number) => req<WebcalInfo>(`/api/v1/calendars/${id}/webcal`),
  rotateWebcal: (id: number) => req<WebcalInfo>(`/api/v1/calendars/${id}/webcal`, { method: "POST", body: "{}" }),
  deleteWebcal: (id: number) => req(`/api/v1/calendars/${id}/webcal`, { method: "DELETE" }),
  tasks: async (init?: RequestInit) => (await req<TaskItem[] | null>("/api/v1/tasks", init)) ?? [],
  createTask: (id: number, body: { summary: string; description?: string; due?: string; status?: string }) =>
    req<{ href: string; uid: string }>(`/api/v1/calendars/${id}/tasks`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  updateTask: (id: number, href: string, body: { summary: string; description?: string; due?: string; status?: string }) =>
    req<TaskItem>(`/api/v1/calendars/${id}/tasks/${encodeURIComponent(href)}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  deleteTask: (id: number, href: string) =>
    req(`/api/v1/calendars/${id}/tasks/${encodeURIComponent(href)}`, { method: "DELETE" }),
  uploadEventAttachment: (id: number, href: string, file: File) => {
    const body = new FormData();
    body.append("file", file, file.name);
    return req<Attachment[]>(`/api/v1/calendars/${id}/events/${encodeURIComponent(href)}/attachments`, {
      method: "POST",
      body,
    });
  },
  uploadTaskAttachment: (id: number, href: string, file: File) => {
    const body = new FormData();
    body.append("file", file, file.name);
    return req<Attachment[]>(`/api/v1/calendars/${id}/tasks/${encodeURIComponent(href)}/attachments`, {
      method: "POST",
      body,
    });
  },
  deleteEventAttachment: (id: number, href: string, attId: string) =>
    req(`/api/v1/calendars/${id}/events/${encodeURIComponent(href)}/attachments/${encodeURIComponent(attId)}`, {
      method: "DELETE",
    }),
  deleteTaskAttachment: (id: number, href: string, attId: string) =>
    req(`/api/v1/calendars/${id}/tasks/${encodeURIComponent(href)}/attachments/${encodeURIComponent(attId)}`, {
      method: "DELETE",
    }),
  downloadAttachment: (calendarId: number, attId: string, filename: string) =>
    downloadAuthed(`/api/v1/calendars/${calendarId}/attachments/${encodeURIComponent(attId)}`, filename),
  freebusy: (users: string[], start: string, end: string) =>
    req<Record<string, BusyPeriod[]>>(
      `/api/v1/freebusy?users=${encodeURIComponent(users.join(","))}&start=${encodeURIComponent(start)}&end=${encodeURIComponent(end)}`,
    ),
  addressbooks: async () => (await req<AddressBook[] | null>("/api/v1/addressbooks")) ?? [],
  contacts: async (id: number) =>
    (await req<Contact[] | null>(`/api/v1/addressbooks/${id}/contacts`)) ?? [],
  getContact: (id: number, href: string) =>
    req<Contact>(`/api/v1/addressbooks/${id}/contacts/${encodeURIComponent(href)}`),
  createContact: (id: number, body: ContactWrite) =>
    req<{ href: string; uid: string }>(`/api/v1/addressbooks/${id}/contacts`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  updateContact: (id: number, href: string, body: ContactWrite) =>
    req<Contact>(`/api/v1/addressbooks/${id}/contacts/${encodeURIComponent(href)}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  deleteContact: (id: number, href: string) =>
    req(`/api/v1/addressbooks/${id}/contacts/${encodeURIComponent(href)}`, { method: "DELETE" }),
  exportContacts: (id: number) => downloadAuthed(`/api/v1/addressbooks/${id}/contacts/export`, "contacts.vcf"),
  exportContact: (id: number, href: string) =>
    downloadAuthed(`/api/v1/addressbooks/${id}/contacts/${encodeURIComponent(href)}/vcard`, "contact.vcf"),
  importContacts: (id: number, body: string | ArrayBuffer, contentType = "text/vcard") =>
    req<ContactImportResult>(`/api/v1/addressbooks/${id}/contacts/import`, {
      method: "POST",
      body,
      headers: { "Content-Type": contentType, Accept: "application/json" },
    }),
  invitations: () => req<Invitation[]>("/api/v1/invitations"),
  accept: (id: number, calendarId?: number) =>
    req(`/api/v1/invitations/${id}/accept`, {
      method: "POST",
      body: JSON.stringify(calendarId ? { calendar_id: calendarId } : {}),
    }),
  decline: (id: number) => req(`/api/v1/invitations/${id}/decline`, { method: "POST" }),
  importantDates: () => req<ImportantDates>("/api/v1/settings/important-dates"),
  saveImportantDates: (s: ImportantDates) =>
    req("/api/v1/settings/important-dates", { method: "PUT", body: JSON.stringify(s) }),
  mailStatus: () => req<MailStatus>("/api/v1/mail"),
  accounts: async () => (await req<ConnectedAccount[] | null>("/api/v1/accounts")) ?? [],
  connectAccount: (body: Record<string, unknown>) =>
    req<{ authorize_url?: string; id?: number; provider?: string; email?: string; status?: string }>("/api/v1/accounts", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  disconnectAccount: (id: number) => req(`/api/v1/accounts/${id}`, { method: "DELETE" }),
  testAccount: (id: number) => req(`/api/v1/accounts/${id}/test`, { method: "POST", body: "{}" }),
  users: () => req<User[]>("/api/v1/admin/users"),
  createUser: (body: {
    username: string;
    email: string;
    password: string;
    display_name: string;
    role: string;
    timezone: string;
  }) => req<CreatedUser>("/api/v1/admin/users", { method: "POST", body: JSON.stringify(body) }),
  patchUser: (id: number, body: Record<string, string>) =>
    req<User>(`/api/v1/admin/users/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
  resetPassword: (id: number, password: string) =>
    req(`/api/v1/admin/users/${id}/password`, { method: "POST", body: JSON.stringify({ password }) }),
  sendRecovery: (id: number) =>
    req<{ ok: boolean; recovery_url: string; emailed: boolean; delivered?: string }>(
      `/api/v1/admin/users/${id}/recovery`,
      { method: "POST" },
    ),
  disableUserTotp: (id: number) => req(`/api/v1/admin/users/${id}/totp/disable`, { method: "POST" }),
  audit: async () => (await req<AuditEntry[] | null>("/api/v1/admin/audit")) ?? [],
  recoveryOutbox: async () => (await req<RecoveryMessage[] | null>("/api/v1/admin/recovery-outbox")) ?? [],
};
