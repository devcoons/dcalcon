# Specs

CalDAV did not get a “2.0”. Interop is RFC 4791 / 6352 plus the extras clients actually send. CalConnect’s starter list is still a decent map: https://www.calconnect.org/resources/caldav-and-carddav/

Handlers start from `github.com/emersion/go-webdav`. ctag, sync-collection, and scheduling sit on top of that.

## Transport

| Spec | |
|---|---|
| RFC 4918 WebDAV | PROPFIND, PUT, DELETE, MKCOL |
| RFC 4791 CalDAV | calendars, calendar-query / calendar-multiget |
| RFC 6352 CardDAV | address books, addressbook-query / addressbook-multiget |
| RFC 5397 current-user-principal | discovery; clients fall over without it |
| RFC 6764 | `/.well-known/caldav` and `carddav` |
| RFC 5689 extended MKCOL | create collection with properties |
| RFC 3744 ACL | privilege-set / acl on calendars; ACL method → whole-calendar shares. Not CS:invite, not per-resource ACE |
| RFC 2818 / RFC 7617 | TLS and Basic |

## Payload

| Spec | |
|---|---|
| RFC 5545 iCalendar | VEVENT / VTODO / VTIMEZONE |
| RFC 2426 vCard 3 | CardDAV must |
| RFC 6350 vCard 4 | should; keep unknown props |

## Scheduling

| Spec | |
|---|---|
| RFC 5546 iTIP | REQUEST / REPLY / CANCEL, ORGANIZER / ATTENDEE |
| RFC 6638 | inbox/outbox on CalDAV |
| RFC 6047 iMIP | same methods over email |

iTIP is the state. CalDAV, mail, and REST accept/decline are three views of it.

## Sync (needed before calling DAVx⁵ done)

| Spec | |
|---|---|
| RFC 6578 sync-collection | incremental |
| Apple `CS:getctag` | cheap change check (not IETF, still expected) |
| Apple calendar-color / calendar-order | GNOME / Apple / DAVx⁵ |
| ETags + If-Match | lost-update (RFC 4791 extra rules) |

## Preserve on the wire, maybe feature later

RFC 7986 extra properties, RFC 9073/9074, RFC 9253 RELATED-TO, RFC 7809 TZ by reference, RFC 7953 VAVAILABILITY. Unknown properties should round-trip. Do not drop them in Go structs.

## Dashboard JSON only

RFC 8984 JSCalendar is a mapping from stored iCalendar. RFC 7529 jCal / RFC 7095 jCard are optional encodings. DAV clients PUT iCalendar/vCard. JSCalendar is not a CalDAV payload.
