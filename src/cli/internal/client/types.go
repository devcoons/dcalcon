package client

type User struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Status      string `json:"status"`
	Timezone    string `json:"timezone"`
	TOTPEnabled bool   `json:"totp_enabled"`
}

type DirectoryUser struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	LocalEmail  string `json:"local_email"`
}

type Calendar struct {
	ID            int64  `json:"id"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Color         string `json:"color"`
	Kind          string `json:"kind"`
	ReadOnly      bool   `json:"read_only"`
	Shared        bool   `json:"shared"`
	Access        string `json:"access"`
	OwnerUsername string `json:"owner_username"`
}

type CalendarShare struct {
	ID          int64  `json:"id"`
	CalendarID  int64  `json:"calendar_id"`
	UserID      int64  `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Access      string `json:"access"`
	CreatedAt   string `json:"created_at"`
}

type Attachment struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	CreatedAt   string `json:"created_at"`
}

type Event struct {
	Href         string       `json:"href"`
	UID          string       `json:"uid"`
	ETag         string       `json:"etag"`
	Summary      string       `json:"summary"`
	Description  string       `json:"description"`
	Location     string       `json:"location"`
	DTStart      string       `json:"dtstart"`
	DTEnd        string       `json:"dtend"`
	AllDay       bool         `json:"all_day"`
	RRule        string       `json:"rrule"`
	AlarmMinutes int          `json:"alarm_minutes"`
	Attachments  []Attachment `json:"attachments"`
}

type Task struct {
	Href         string       `json:"href"`
	UID          string       `json:"uid"`
	Summary      string       `json:"summary"`
	Description  string       `json:"description"`
	Due          string       `json:"due"`
	Status       string       `json:"status"`
	CalendarID   int64        `json:"calendar_id"`
	CalendarName string       `json:"calendar_name"`
	Attachments  []Attachment `json:"attachments"`
}

type AddressBook struct {
	ID          int64  `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ReadOnly    bool   `json:"read_only"`
}

type Contact struct {
	Href        string `json:"href"`
	UID         string `json:"uid"`
	FN          string `json:"fn"`
	Email       string `json:"email"`
	Tel         string `json:"tel"`
	Org         string `json:"org"`
	Title       string `json:"title"`
	BDay        string `json:"bday"`
	Anniversary string `json:"anniversary"`
	Note        string `json:"note"`
	Nickname    string `json:"nickname"`
	GivenName   string `json:"given_name"`
	Family      string `json:"family_name"`
}

type Invitation struct {
	ID        int64  `json:"id"`
	Method    string `json:"method"`
	UID       string `json:"uid"`
	Summary   string `json:"summary"`
	DTStart   string `json:"dtstart"`
	Organizer string `json:"organizer"`
	Attendee  string `json:"attendee"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type Setup struct {
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

type Overview struct {
	Calendars             int    `json:"calendars"`
	Contacts              int    `json:"contacts"`
	PendingInvitations    int    `json:"pending_invitations"`
	ImportantDatesEnabled bool   `json:"important_dates_enabled"`
	SharedCalendars       int    `json:"shared_calendars"`
	EventsSoon            int    `json:"events_soon"`
	MailConnected         bool   `json:"mail_connected"`
	MailAddress           string `json:"mail_address"`
	TOTPEnabled           bool   `json:"totp_enabled"`
}

type ImportantDates struct {
	Enabled              bool     `json:"enabled"`
	IncludeBirthdays     bool     `json:"include_birthdays"`
	IncludeAnniversaries bool     `json:"include_anniversaries"`
	AlarmOffsets         []string `json:"alarm_offsets"`
}

type AppPassword struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Prefix     string `json:"prefix"`
	Password   string `json:"password,omitempty"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at"`
}

type Account struct {
	ID        int64  `json:"id"`
	Provider  string `json:"provider"`
	Email     string `json:"email"`
	Status    string `json:"status"`
	LastError string `json:"last_error"`
}

type MailStatus struct {
	GoogleConfigured    bool `json:"google_configured"`
	MicrosoftConfigured bool `json:"microsoft_configured"`
	ServerSMTP          bool `json:"server_smtp"`
	TokenKey            bool `json:"token_key"`
}

type AuditEntry struct {
	ID     int64  `json:"id"`
	At     string `json:"at"`
	Actor  string `json:"actor"`
	Action string `json:"action"`
	Detail string `json:"detail"`
}

type ImportResult struct {
	Created int      `json:"created"`
	Updated int      `json:"updated"`
	Skipped int      `json:"skipped"`
	Errors  []string `json:"errors"`
}

type Webcal struct {
	Enabled bool   `json:"enabled"`
	Token   string `json:"token"`
	URL     string `json:"url"`
}

type RecoveryOutbox struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	Delivered string `json:"delivered"`
	LastError string `json:"last_error"`
	CreatedAt string `json:"created_at"`
}
