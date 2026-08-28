package app

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/devcoons/dcalcon/cli/internal/client"
)

func (e *Env) cmdInvitation(args []string) error {
	if len(args) == 0 || args[0] == "list" {
		if len(args) > 0 {
			args = args[1:]
		}
		return e.invitationList(args)
	}
	switch args[0] {
	case "accept":
		return e.invitationAccept(args[1:])
	case "decline":
		return e.invitationDecline(args[1:])
	default:
		return fmt.Errorf("invitation %s: try list, accept, decline", args[0])
	}
}

func (e *Env) invitationList(args []string) error {
	if err := e.flags("invitation list").Parse(args); err != nil {
		return err
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	raw, err := e.cli.Get("/api/v1/invitations", nil)
	if err != nil {
		return err
	}
	list, err := decodeList[client.Invitation](raw)
	if err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(list)
	}
	rows := make([][]string, 0, len(list))
	for _, it := range list {
		rows = append(rows, []string{formatInt(it.ID), it.Status, it.Summary, it.Organizer, it.DTStart})
	}
	e.table([]string{"ID", "STATUS", "SUMMARY", "FROM", "START"}, rows)
	return nil
}

func (e *Env) invitationAccept(args []string) error {
	fs := e.flags("invitation accept")
	cal := fs.String("calendar", "", "calendar to write the event into")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: invitation accept ID [--calendar CAL]")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	body := map[string]any{}
	if *cal != "" {
		c, err := e.calendarRef(*cal)
		if err != nil {
			return err
		}
		body["calendar_id"] = c.ID
	}
	if _, err := e.cli.JSON("POST", "/api/v1/invitations/"+fs.Arg(0)+"/accept", body); err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(map[string]string{"status": "accepted"})
	}
	fmt.Fprintln(e.Stdout, "accepted")
	return nil
}

func (e *Env) invitationDecline(args []string) error {
	fs := e.flags("invitation decline")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: invitation decline ID")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	if _, err := e.cli.JSON("POST", "/api/v1/invitations/"+fs.Arg(0)+"/decline", map[string]string{}); err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(map[string]string{"status": "declined"})
	}
	fmt.Fprintln(e.Stdout, "declined")
	return nil
}

func (e *Env) cmdFreebusy(args []string) error {
	fs := e.flags("freebusy")
	users := fs.String("users", "", "comma-separated usernames")
	start := fs.String("start", "", "")
	end := fs.String("end", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *users == "" {
		return fmt.Errorf("--users is required")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	q := url.Values{}
	q.Set("users", *users)
	if *start != "" {
		q.Set("start", *start)
	}
	if *end != "" {
		q.Set("end", *end)
	}
	raw, err := e.cli.Get("/api/v1/freebusy", q)
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
