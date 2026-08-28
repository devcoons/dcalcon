package app

import (
	"fmt"
	"os"
	"strings"

	"github.com/devcoons/dcalcon/cli/internal/client"
)

func (e *Env) cmdCalendar(args []string) error {
	if len(args) == 0 || args[0] == "list" {
		if len(args) > 0 && args[0] == "list" {
			args = args[1:]
		}
		return e.calendarList(args)
	}
	switch args[0] {
	case "create":
		return e.calendarCreate(args[1:])
	case "update":
		return e.calendarUpdate(args[1:])
	case "delete":
		return e.calendarDelete(args[1:])
	case "shares":
		return e.calendarShares(args[1:])
	case "share":
		return e.calendarShare(args[1:])
	case "unshare":
		return e.calendarUnshare(args[1:])
	case "export":
		return e.calendarExport(args[1:])
	case "import":
		return e.calendarImport(args[1:])
	case "webcal":
		return e.calendarWebcal(args[1:])
	default:
		return fmt.Errorf("calendar %s: try list, create, update, delete, shares, share, unshare, export, import, webcal", args[0])
	}
}

func (e *Env) calendarList(args []string) error {
	if err := e.flags("calendar list").Parse(args); err != nil {
		return err
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	list, err := e.calendars()
	if err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(list)
	}
	rows := make([][]string, 0, len(list))
	for _, c := range list {
		access := c.Access
		if access == "" {
			access = "owner"
		}
		rows = append(rows, []string{formatInt(c.ID), c.Slug, c.Name, c.Kind, access, yn(c.ReadOnly)})
	}
	e.table([]string{"ID", "SLUG", "NAME", "KIND", "ACCESS", "RO"}, rows)
	return nil
}

func (e *Env) calendarCreate(args []string) error {
	fs := e.flags("calendar create")
	name := fs.String("name", "", "display name")
	desc := fs.String("desc", "", "description")
	color := fs.String("color", "", "hex color")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" && fs.NArg() > 0 {
		*name = strings.Join(fs.Args(), " ")
	}
	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	raw, err := e.cli.JSON("POST", "/api/v1/calendars", map[string]string{
		"name": *name, "description": *desc, "color": *color,
	})
	if err != nil {
		return err
	}
	var c client.Calendar
	if err := decode(raw, &c); err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(c)
	}
	fmt.Fprintf(e.Stdout, "created calendar %d %s\n", c.ID, c.Slug)
	return nil
}

func (e *Env) calendarUpdate(args []string) error {
	fs := e.flags("calendar update")
	name := fs.String("name", "", "display name")
	desc := fs.String("desc", "", "description")
	color := fs.String("color", "", "hex color")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: calendar update CAL --name NAME")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	c, err := e.calendarRef(fs.Arg(0))
	if err != nil {
		return err
	}
	body := map[string]string{}
	if *name != "" {
		body["name"] = *name
	}
	if *desc != "" {
		body["description"] = *desc
	}
	if *color != "" {
		body["color"] = *color
	}
	raw, err := e.cli.JSON("PATCH", "/api/v1/calendars/"+client.Itoa(c.ID), body)
	if err != nil {
		return err
	}
	if e.JSON {
		var v any
		_ = decode(raw, &v)
		return e.printJSON(v)
	}
	fmt.Fprintf(e.Stdout, "updated calendar %s\n", c.Slug)
	return nil
}

func (e *Env) calendarDelete(args []string) error {
	fs := e.flags("calendar delete")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: calendar delete CAL")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	c, err := e.calendarRef(fs.Arg(0))
	if err != nil {
		return err
	}
	if _, err := e.cli.JSON("DELETE", "/api/v1/calendars/"+client.Itoa(c.ID), nil); err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(map[string]string{"status": "ok"})
	}
	fmt.Fprintf(e.Stdout, "deleted calendar %s\n", c.Slug)
	return nil
}

func (e *Env) calendarShares(args []string) error {
	fs := e.flags("calendar shares")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: calendar shares CAL")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	c, err := e.calendarRef(fs.Arg(0))
	if err != nil {
		return err
	}
	raw, err := e.cli.Get("/api/v1/calendars/"+client.Itoa(c.ID)+"/shares", nil)
	if err != nil {
		return err
	}
	list, err := decodeList[client.CalendarShare](raw)
	if err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(list)
	}
	rows := make([][]string, 0, len(list))
	for _, s := range list {
		rows = append(rows, []string{formatInt(s.UserID), s.Username, s.DisplayName, s.Access})
	}
	e.table([]string{"USER", "USERNAME", "NAME", "ACCESS"}, rows)
	return nil
}

func (e *Env) calendarShare(args []string) error {
	fs := e.flags("calendar share")
	user := fs.String("user", "", "username")
	access := fs.String("access", "read", "read or write")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 || *user == "" {
		return fmt.Errorf("usage: calendar share CAL --user NAME [--access read|write]")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	c, err := e.calendarRef(fs.Arg(0))
	if err != nil {
		return err
	}
	raw, err := e.cli.JSON("POST", "/api/v1/calendars/"+client.Itoa(c.ID)+"/shares", map[string]string{
		"username": *user, "access": *access,
	})
	if err != nil {
		return err
	}
	if e.JSON {
		var v any
		_ = decode(raw, &v)
		return e.printJSON(v)
	}
	fmt.Fprintf(e.Stdout, "shared %s with %s (%s)\n", c.Slug, *user, *access)
	return nil
}

func (e *Env) calendarUnshare(args []string) error {
	fs := e.flags("calendar unshare")
	user := fs.String("user", "", "username or user id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 || *user == "" {
		return fmt.Errorf("usage: calendar unshare CAL --user NAME")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	c, err := e.calendarRef(fs.Arg(0))
	if err != nil {
		return err
	}
	uid, err := e.shareUserID(c.ID, *user)
	if err != nil {
		return err
	}
	if _, err := e.cli.JSON("DELETE", "/api/v1/calendars/"+client.Itoa(c.ID)+"/shares/"+client.Itoa(uid), nil); err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(map[string]string{"status": "ok"})
	}
	fmt.Fprintf(e.Stdout, "removed share for %s\n", *user)
	return nil
}

func (e *Env) calendarExport(args []string) error {
	fs := e.flags("calendar export")
	out := fs.String("out", "", "output file (default stdout / suggested name)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: calendar export CAL [--out FILE]")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	c, err := e.calendarRef(fs.Arg(0))
	if err != nil {
		return err
	}
	data, name, err := e.cli.Download("/api/v1/calendars/" + client.Itoa(c.ID) + "/export")
	if err != nil {
		return err
	}
	return e.saveDownload(data, name, *out)
}

func (e *Env) calendarImport(args []string) error {
	fs := e.flags("calendar import")
	file := fs.String("file", "", "ICS file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 || *file == "" {
		return fmt.Errorf("usage: calendar import CAL --file FILE.ics")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	c, err := e.calendarRef(fs.Arg(0))
	if err != nil {
		return err
	}
	rawFile, err := os.ReadFile(*file)
	if err != nil {
		return err
	}
	raw, err := e.cli.Raw("POST", "/api/v1/calendars/"+client.Itoa(c.ID)+"/import", rawFile, "text/calendar")
	if err != nil {
		return err
	}
	var res client.ImportResult
	if err := decode(raw, &res); err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(res)
	}
	fmt.Fprintf(e.Stdout, "created %d  updated %d  skipped %d\n", res.Created, res.Updated, res.Skipped)
	for _, m := range res.Errors {
		fmt.Fprintf(e.Stderr, "  %s\n", m)
	}
	return nil
}

func (e *Env) calendarWebcal(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: calendar webcal CAL [show|enable|disable]")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	c, err := e.calendarRef(args[0])
	if err != nil {
		return err
	}
	action := "show"
	if len(args) > 1 {
		action = args[1]
	}
	var raw []byte
	switch action {
	case "show":
		raw, err = e.cli.Get("/api/v1/calendars/"+client.Itoa(c.ID)+"/webcal", nil)
	case "enable", "rotate":
		raw, err = e.cli.JSON("POST", "/api/v1/calendars/"+client.Itoa(c.ID)+"/webcal", map[string]string{})
	case "disable":
		raw, err = e.cli.JSON("DELETE", "/api/v1/calendars/"+client.Itoa(c.ID)+"/webcal", nil)
	default:
		return fmt.Errorf("webcal action must be show, enable, or disable")
	}
	if err != nil {
		return err
	}
	var w client.Webcal
	_ = decode(raw, &w)
	if e.JSON {
		return e.printJSON(w)
	}
	if !w.Enabled {
		fmt.Fprintln(e.Stdout, "webcal off")
		return nil
	}
	if w.URL == "" {
		fmt.Fprintln(e.Stdout, "webcal on (rotate to copy URL)")
		return nil
	}
	e.kv("enabled", "yes", "url", w.URL)
	return nil
}
