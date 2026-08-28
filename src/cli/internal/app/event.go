package app

import (
	"fmt"
	"strings"

	"github.com/devcoons/dcalcon/cli/internal/client"
)

func (e *Env) cmdEvent(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: event list|get|create|update|delete|invite")
	}
	switch args[0] {
	case "list":
		return e.eventList(args[1:])
	case "get":
		return e.eventGet(args[1:])
	case "create":
		return e.eventCreate(args[1:])
	case "update":
		return e.eventUpdate(args[1:])
	case "delete":
		return e.eventDelete(args[1:])
	case "invite":
		return e.eventInvite(args[1:])
	default:
		return fmt.Errorf("event %s: try list, get, create, update, delete, invite", args[0])
	}
}

func (e *Env) eventList(args []string) error {
	fs := e.flags("event list")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: event list CAL")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	c, err := e.calendarRef(fs.Arg(0))
	if err != nil {
		return err
	}
	raw, err := e.cli.Get("/api/v1/calendars/"+client.Itoa(c.ID)+"/events", nil)
	if err != nil {
		return err
	}
	list, err := decodeList[client.Event](raw)
	if err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(list)
	}
	rows := make([][]string, 0, len(list))
	for _, ev := range list {
		rows = append(rows, []string{ev.Href, ev.Summary, ev.DTStart, ev.DTEnd, ev.Location})
	}
	e.table([]string{"HREF", "SUMMARY", "START", "END", "LOCATION"}, rows)
	return nil
}

func (e *Env) eventGet(args []string) error {
	fs := e.flags("event get")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return fmt.Errorf("usage: event get CAL HREF")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	c, err := e.calendarRef(fs.Arg(0))
	if err != nil {
		return err
	}
	raw, err := e.cli.Get("/api/v1/calendars/"+client.Itoa(c.ID)+"/events/"+client.Enc(fs.Arg(1)), nil)
	if err != nil {
		return err
	}
	var ev client.Event
	if err := decode(raw, &ev); err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(ev)
	}
	e.kv("href", ev.Href, "summary", ev.Summary, "start", ev.DTStart, "end", ev.DTEnd, "location", ev.Location, "description", ev.Description, "rrule", ev.RRule, "alarm_minutes", formatInt(int64(ev.AlarmMinutes)))
	return nil
}

func eventBody(fsArgs func(name string) string, allDay bool, invite, emails []string, alarm int) map[string]any {
	body := map[string]any{
		"summary":     fsArgs("summary"),
		"description": fsArgs("desc"),
		"location":    fsArgs("location"),
		"dtstart":     fsArgs("start"),
		"dtend":       fsArgs("end"),
		"all_day":     allDay,
	}
	if r := fsArgs("rrule"); r != "" {
		body["rrule"] = r
	}
	if alarm >= 0 {
		body["alarm_minutes"] = alarm
	}
	if len(invite) > 0 {
		body["invite"] = invite
	}
	if len(emails) > 0 {
		body["invite_emails"] = emails
	}
	return body
}

func (e *Env) eventCreate(args []string) error {
	fs := e.flags("event create")
	summary := fs.String("summary", "", "")
	start := fs.String("start", "", "start (e.g. 2026-08-28T09:00 or 20260828T090000Z)")
	end := fs.String("end", "", "end")
	loc := fs.String("location", "", "")
	desc := fs.String("desc", "", "")
	rrule := fs.String("rrule", "", "")
	allDay := fs.Bool("all-day", false, "")
	alarm := fs.Int("alarm", -1, "minutes before start (0 = none)")
	var invite, emails arrayFlag
	fs.Var(&invite, "invite", "local username (repeatable)")
	fs.Var(&emails, "email", "external email (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 || *summary == "" || *start == "" {
		return fmt.Errorf("usage: event create CAL --summary TEXT --start TIME")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	c, err := e.calendarRef(fs.Arg(0))
	if err != nil {
		return err
	}
	get := func(name string) string {
		switch name {
		case "summary":
			return *summary
		case "start":
			return *start
		case "end":
			return *end
		case "location":
			return *loc
		case "desc":
			return *desc
		case "rrule":
			return *rrule
		}
		return ""
	}
	raw, err := e.cli.JSON("POST", "/api/v1/calendars/"+client.Itoa(c.ID)+"/events", eventBody(get, *allDay, invite, emails, *alarm))
	if err != nil {
		return err
	}
	if e.JSON {
		var v any
		_ = decode(raw, &v)
		return e.printJSON(v)
	}
	var created struct {
		Href string `json:"href"`
		UID  string `json:"uid"`
	}
	_ = decode(raw, &created)
	fmt.Fprintf(e.Stdout, "created %s\n", created.Href)
	return nil
}

func (e *Env) eventUpdate(args []string) error {
	fs := e.flags("event update")
	summary := fs.String("summary", "", "")
	start := fs.String("start", "", "")
	end := fs.String("end", "", "")
	loc := fs.String("location", "", "")
	desc := fs.String("desc", "", "")
	rrule := fs.String("rrule", "", "")
	allDay := fs.Bool("all-day", false, "")
	alarm := fs.Int("alarm", -1, "minutes before start (0 = none)")
	var invite, emails arrayFlag
	fs.Var(&invite, "invite", "local username (repeatable)")
	fs.Var(&emails, "email", "external email (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 || *summary == "" || *start == "" {
		return fmt.Errorf("usage: event update CAL HREF --summary TEXT --start TIME")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	c, err := e.calendarRef(fs.Arg(0))
	if err != nil {
		return err
	}
	get := func(name string) string {
		switch name {
		case "summary":
			return *summary
		case "start":
			return *start
		case "end":
			return *end
		case "location":
			return *loc
		case "desc":
			return *desc
		case "rrule":
			return *rrule
		}
		return ""
	}
	raw, err := e.cli.JSON("PUT", "/api/v1/calendars/"+client.Itoa(c.ID)+"/events/"+client.Enc(fs.Arg(1)), eventBody(get, *allDay, invite, emails, *alarm))
	if err != nil {
		return err
	}
	if e.JSON {
		var v any
		_ = decode(raw, &v)
		return e.printJSON(v)
	}
	fmt.Fprintf(e.Stdout, "updated %s\n", fs.Arg(1))
	return nil
}

func (e *Env) eventDelete(args []string) error {
	fs := e.flags("event delete")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return fmt.Errorf("usage: event delete CAL HREF")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	c, err := e.calendarRef(fs.Arg(0))
	if err != nil {
		return err
	}
	if _, err := e.cli.JSON("DELETE", "/api/v1/calendars/"+client.Itoa(c.ID)+"/events/"+client.Enc(fs.Arg(1)), nil); err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(map[string]string{"status": "ok"})
	}
	fmt.Fprintf(e.Stdout, "deleted %s\n", fs.Arg(1))
	return nil
}

func (e *Env) eventInvite(args []string) error {
	fs := e.flags("event invite")
	var users, emails arrayFlag
	fs.Var(&users, "user", "local username (repeatable)")
	fs.Var(&emails, "email", "external email (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 || (len(users) == 0 && len(emails) == 0) {
		return fmt.Errorf("usage: event invite CAL HREF --user NAME [--email ADDR]")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	c, err := e.calendarRef(fs.Arg(0))
	if err != nil {
		return err
	}
	raw, err := e.cli.JSON("POST", "/api/v1/events/"+client.Itoa(c.ID)+"/invite", map[string]any{
		"href": fs.Arg(1), "usernames": []string(users), "emails": []string(emails),
	})
	if err != nil {
		return err
	}
	if e.JSON {
		var v any
		_ = decode(raw, &v)
		return e.printJSON(v)
	}
	fmt.Fprintln(e.Stdout, strings.TrimSpace(string(raw)))
	return nil
}

type arrayFlag []string

func (a *arrayFlag) String() string { return strings.Join(*a, ",") }
func (a *arrayFlag) Set(v string) error {
	*a = append(*a, v)
	return nil
}
