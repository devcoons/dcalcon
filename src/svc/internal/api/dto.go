package api

import (
	"strings"

	"github.com/devcoons/dcalcon/internal/davpath"
	"github.com/devcoons/dcalcon/internal/icsutil"
	"github.com/devcoons/dcalcon/internal/schedule"
	"github.com/devcoons/dcalcon/internal/storage"
)

type eventWrite struct {
	Summary      string   `json:"summary"`
	Description  string   `json:"description"`
	Location     string   `json:"location"`
	DTStart      string   `json:"dtstart"`
	DTEnd        string   `json:"dtend"`
	UID          string   `json:"uid"`
	AllDay       bool     `json:"all_day"`
	RRule        *string  `json:"rrule"`
	AlarmMinutes *int     `json:"alarm_minutes"`
	Invite       []string `json:"invite"`
	InviteEmails []string `json:"invite_emails"`
}

func (b *eventWrite) prepare() string {
	b.Summary = strings.TrimSpace(b.Summary)
	if b.Summary == "" || strings.TrimSpace(b.DTStart) == "" {
		return "summary and dtstart are required"
	}
	b.DTStart, b.DTEnd = icsutil.NormalizeEventTimes(b.DTStart, b.DTEnd, b.AllDay)
	return ""
}

type eventDTO struct {
	Href         string                 `json:"href"`
	UID          string                 `json:"uid"`
	ETag         string                 `json:"etag"`
	Summary      string                 `json:"summary"`
	Description  string                 `json:"description"`
	Location     string                 `json:"location"`
	DTStart      string                 `json:"dtstart"`
	DTEnd        string                 `json:"dtend"`
	AllDay       bool                   `json:"all_day"`
	RRule        string                 `json:"rrule,omitempty"`
	AlarmMinutes int                    `json:"alarm_minutes,omitempty"`
	Attendees    []icsutil.AttendeeInfo `json:"attendees,omitempty"`
	Attachments  []storage.Attachment   `json:"attachments"`
}

type eventWithInvite struct {
	eventDTO
	Invite *inviteResult `json:"invite,omitempty"`
}

type contactDTO struct {
	Href string `json:"href"`
	UID  string `json:"uid"`
	icsutil.ContactInput
}

type invitationDTO struct {
	ID          int64  `json:"id"`
	Method      string `json:"method"`
	UID         string `json:"uid"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
	Location    string `json:"location"`
	DTStart     string `json:"dtstart"`
	DTEnd       string `json:"dtend"`
	Organizer   string `json:"organizer"`
	Attendee    string `json:"attendee"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
}

type setupDTO struct {
	PublicURL         string `json:"public_url"`
	Username          string `json:"username"`
	AuthMethod        string `json:"auth_method"`
	CalDAVWellKnown   string `json:"caldav_well_known"`
	CardDAVWellKnown  string `json:"carddav_well_known"`
	PrincipalURL      string `json:"principal_url"`
	CalendarHome      string `json:"calendar_home"`
	AddressBookHome   string `json:"addressbook_home"`
	SchedulingAddress string `json:"scheduling_address"`
	SchedulingDomain  string `json:"scheduling_domain"`
}

type overviewCal struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Color         string `json:"color"`
	Kind          string `json:"kind"`
	Shared        bool   `json:"shared"`
	ReadOnly      bool   `json:"read_only"`
	Access        string `json:"access,omitempty"`
	OwnerUsername string `json:"owner_username,omitempty"`
}

type overviewEvent struct {
	Href         string `json:"href"`
	CalendarID   int64  `json:"calendar_id"`
	Summary      string `json:"summary"`
	Location     string `json:"location,omitempty"`
	DTStart      string `json:"dtstart"`
	DTEnd        string `json:"dtend"`
	AllDay       bool   `json:"all_day"`
	Color        string `json:"color"`
	CalendarName string `json:"calendar_name"`
	Kind         string `json:"kind,omitempty"`
}

type overviewInvite struct {
	ID        int64  `json:"id"`
	Summary   string `json:"summary"`
	Organizer string `json:"organizer"`
	DTStart   string `json:"dtstart"`
}

type overviewDTO struct {
	Calendars             int              `json:"calendars"`
	Contacts              int              `json:"contacts"`
	PendingInvitations    int              `json:"pending_invitations"`
	ImportantDatesEnabled bool             `json:"important_dates_enabled"`
	SharedCalendars       int              `json:"shared_calendars"`
	EventsSoon            int              `json:"events_soon"`
	MailConnected         bool             `json:"mail_connected"`
	MailAddress           string           `json:"mail_address,omitempty"`
	TotpEnabled           bool             `json:"totp_enabled"`
	CalendarList          []overviewCal    `json:"calendar_list"`
	Upcoming              []overviewEvent  `json:"upcoming"`
	Pending               []overviewInvite `json:"pending"`
}

func toEventDTO(o storage.CalendarObject, atts []storage.Attachment) eventDTO {
	if atts == nil {
		atts = []storage.Attachment{}
	}
	f := icsutil.EventFieldsFromICS(o.ICS)
	return eventDTO{
		Href: o.Href, UID: o.UID, ETag: o.ETag, Summary: o.Summary,
		Description: f.Description,
		Location:    f.Location,
		DTStart:     o.DTStart, DTEnd: o.DTEnd,
		AllDay:       f.AllDay,
		RRule:        f.RRule,
		AlarmMinutes: f.AlarmMinutes,
		Attendees:    f.Attendees,
		Attachments:  atts,
	}
}

func isTodo(o storage.CalendarObject) bool {
	return strings.EqualFold(o.Component, "VTODO")
}

func applyEventExtras(ics string, body eventWrite) string {
	if body.RRule != nil {
		if next, err := icsutil.SetRRule(ics, *body.RRule); err == nil {
			ics = next
		}
	}
	if body.AlarmMinutes != nil {
		if next, err := icsutil.SetDisplayAlarm(ics, *body.AlarmMinutes); err == nil {
			ics = next
		}
	}
	return ics
}

func toContactDTO(o storage.AddressObject) contactDTO {
	in := icsutil.ParseContact(o.VCard)
	in.Normalize()
	return contactDTO{Href: o.Href, UID: o.UID, ContactInput: in}
}

func toInvitationDTO(s storage.ScheduleItem) invitationDTO {
	start, end, summary, desc, loc := "", "", "", "", ""
	if cal, err := icsutil.ParseCalendar(s.ICS); err == nil {
		start, end = icsutil.CalendarRange(cal)
		summary = icsutil.CalendarSummary(cal)
		desc = icsutil.CalendarDescription(cal)
		loc = icsutil.LocationFromCal(cal)
	}
	return invitationDTO{
		ID: s.ID, Method: s.Method, UID: s.UID, Summary: summary,
		Description: desc,
		Location:    loc,
		DTStart:     start, DTEnd: end,
		Organizer: s.Organizer, Attendee: s.Attendee, Status: s.Status, CreatedAt: s.CreatedAt,
	}
}

func (h *Handler) setupFor(username string) setupDTO {
	base := h.publicURL()
	return setupDTO{
		PublicURL:         base,
		Username:          username,
		AuthMethod:        "HTTP Basic (app password recommended)",
		CalDAVWellKnown:   base + "/.well-known/caldav",
		CardDAVWellKnown:  base + "/.well-known/carddav",
		PrincipalURL:      base + davpath.PrincipalPath(username),
		CalendarHome:      base + davpath.CalendarHome(username),
		AddressBookHome:   base + davpath.AddressBookHome(username),
		SchedulingAddress: schedule.LocalMailbox(username),
		SchedulingDomain:  schedule.LocalDomain(),
	}
}
