# Standards we target

CalDAV/CardDAV were not replaced by a “v2”. Interoperability is the original RFCs **plus** the extensions clients probe.

CalConnect starter list: https://www.calconnect.org/resources/caldav-and-carddav/

## Transport and identity (must)

| Spec | Role |
|---|---|
| RFC 4918 WebDAV | PROPFIND, PUT, DELETE, MKCOL |
| RFC 4791 CalDAV `calendar-access` | Calendars, calendar-query / calendar-multiget |
| RFC 6352 CardDAV | Address books, addressbook-query / addressbook-multiget |
| RFC 5397 current-user-principal | Fast discovery — **clients will fail without this** |
| RFC 6764 well-known URIs | `/.well-known/caldav` and `carddav` |
| RFC 5689 extended MKCOL | Create collections with properties |
| RFC 3744 ACL | Privilege-set / acl on calendars; ACL method maps to whole-calendar shares. Not CS:invite or per-resource ACE. |
| RFC 2818 TLS / RFC 7617 Basic | Production transport and DAV login |

## Payload (must)

| Spec | Role |
|---|---|
| RFC 5545 iCalendar | VEVENT / VTODO / VTIMEZONE on the wire |
| RFC 2426 vCard 3 | CardDAV **MUST** |
| RFC 6350 vCard 4 | CardDAV **SHOULD**; store without dropping unknown props |

## Scheduling (must for invites; phased)

| Spec | Role |
|---|---|
| RFC 5546 iTIP | Semantics: REQUEST / REPLY / CANCEL, ORGANIZER / ATTENDEE |
| RFC 6638 CalDAV scheduling | How native calendar apps transport iTIP (inbox/outbox) |
| RFC 6047 iMIP | Same iTIP methods bound to email for Gmail/Outlook attendees |

Domain `schedule` should speak **iTIP**. CalDAV is one transport. Email is another. REST accept/decline is a third view of the same state.

## Sync performance (should, before calling DAVx⁵ “done”)

| Spec | Role |
|---|---|
| RFC 6578 sync-collection | Incremental collection sync |
| Apple `CS:getctag` | Cheap change detection (not IETF, but expected) |
| Apple `calendar-color` / `calendar-order` | Display in GNOME / Apple / DAVx⁵ |
| ETags + If-Match | Lost-update protection (RFC 4791 extra rules) |

## Newer iCalendar (preserve now, feature later)

| Spec | Role |
|---|---|
| RFC 7986 | Extra properties (NAME, COLOR, IMAGE, CONFERENCE, …) |
| RFC 9073 / RFC 9074 | Event publishing, VALARM extensions |
| RFC 9253 | iCalendar relationships (`RELATED-TO`, CONCEPT, LINK) |
| RFC 7809 | Time zones by reference (smaller objects; clients still send VTIMEZONE) |
| RFC 7953 | VAVAILABILITY — richer free/busy, not required for first sync |

Unknown properties must **round-trip**. Do not strip them in Go structs.

## Web JSON only (must not replace DAV)

| Spec | Role |
|---|---|
| RFC 8984 JSCalendar | Dashboard/API mapping from stored iCalendar |
| RFC 7529 jCal / RFC 7095 jCard | Alternate encodings; optional |

DAV clients PUT iCalendar/vCard. JSCalendar is not a CalDAV payload.

## Library baseline

Handlers start from `github.com/emersion/go-webdav`. Scheduling, ctag, and sync-collection will need work on top of it.
