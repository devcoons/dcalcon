package app

import (
	"fmt"

	"github.com/devcoons/dcalcon/cli/internal/client"
)

func (e *Env) cmdTask(args []string) error {
	if len(args) == 0 || args[0] == "list" {
		if len(args) > 0 && args[0] == "list" {
			args = args[1:]
		}
		return e.taskList(args)
	}
	switch args[0] {
	case "create":
		return e.taskCreate(args[1:])
	case "get":
		return e.taskGet(args[1:])
	case "update":
		return e.taskUpdate(args[1:])
	case "delete":
		return e.taskDelete(args[1:])
	default:
		return fmt.Errorf("task %s: try list, get, create, update, delete", args[0])
	}
}

func (e *Env) taskList(args []string) error {
	fs := e.flags("task list")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	path := "/api/v1/tasks"
	if fs.NArg() > 0 {
		c, err := e.calendarRef(fs.Arg(0))
		if err != nil {
			return err
		}
		path = "/api/v1/calendars/" + client.Itoa(c.ID) + "/tasks"
	}
	raw, err := e.cli.Get(path, nil)
	if err != nil {
		return err
	}
	list, err := decodeList[client.Task](raw)
	if err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(list)
	}
	rows := make([][]string, 0, len(list))
	for _, t := range list {
		rows = append(rows, []string{formatInt(t.CalendarID), t.CalendarName, t.Href, t.Summary, t.Due, t.Status})
	}
	e.table([]string{"CAL", "CALENDAR", "HREF", "SUMMARY", "DUE", "STATUS"}, rows)
	return nil
}

func (e *Env) taskGet(args []string) error {
	fs := e.flags("task get")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return fmt.Errorf("usage: task get CAL HREF")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	c, err := e.calendarRef(fs.Arg(0))
	if err != nil {
		return err
	}
	raw, err := e.cli.Get("/api/v1/calendars/"+client.Itoa(c.ID)+"/tasks/"+client.Enc(fs.Arg(1)), nil)
	if err != nil {
		return err
	}
	var t client.Task
	if err := decode(raw, &t); err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(t)
	}
	e.kv("href", t.Href, "summary", t.Summary, "due", t.Due, "status", t.Status, "description", t.Description, "calendar", t.CalendarName)
	return nil
}

func (e *Env) taskCreate(args []string) error {
	fs := e.flags("task create")
	summary := fs.String("summary", "", "")
	due := fs.String("due", "", "")
	status := fs.String("status", "", "")
	desc := fs.String("desc", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 || *summary == "" {
		return fmt.Errorf("usage: task create CAL --summary TEXT")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	c, err := e.calendarRef(fs.Arg(0))
	if err != nil {
		return err
	}
	raw, err := e.cli.JSON("POST", "/api/v1/calendars/"+client.Itoa(c.ID)+"/tasks", map[string]string{
		"summary": *summary, "due": *due, "status": *status, "description": *desc,
	})
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
	}
	_ = decode(raw, &created)
	fmt.Fprintf(e.Stdout, "created %s\n", created.Href)
	return nil
}

func (e *Env) taskUpdate(args []string) error {
	fs := e.flags("task update")
	summary := fs.String("summary", "", "")
	due := fs.String("due", "", "")
	status := fs.String("status", "", "")
	desc := fs.String("desc", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 || *summary == "" {
		return fmt.Errorf("usage: task update CAL HREF --summary TEXT")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	c, err := e.calendarRef(fs.Arg(0))
	if err != nil {
		return err
	}
	raw, err := e.cli.JSON("PUT", "/api/v1/calendars/"+client.Itoa(c.ID)+"/tasks/"+client.Enc(fs.Arg(1)), map[string]string{
		"summary": *summary, "due": *due, "status": *status, "description": *desc,
	})
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

func (e *Env) taskDelete(args []string) error {
	fs := e.flags("task delete")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return fmt.Errorf("usage: task delete CAL HREF")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	c, err := e.calendarRef(fs.Arg(0))
	if err != nil {
		return err
	}
	if _, err := e.cli.JSON("DELETE", "/api/v1/calendars/"+client.Itoa(c.ID)+"/tasks/"+client.Enc(fs.Arg(1)), nil); err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(map[string]string{"status": "ok"})
	}
	fmt.Fprintf(e.Stdout, "deleted %s\n", fs.Arg(1))
	return nil
}
