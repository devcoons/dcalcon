package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/devcoons/dcalcon/cli/internal/client"
)

func (e *Env) cmdFile(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: file list|add|get|delete")
	}
	switch args[0] {
	case "list":
		return e.fileList(args[1:])
	case "add":
		return e.fileAdd(args[1:])
	case "get", "download":
		return e.fileGet(args[1:])
	case "delete":
		return e.fileDelete(args[1:])
	default:
		return fmt.Errorf("file %s: try list, add, get, delete", args[0])
	}
}

func (e *Env) itemPath(calID int64, eventHref, taskHref string) (string, error) {
	if eventHref != "" && taskHref != "" {
		return "", fmt.Errorf("use either --event or --task, not both")
	}
	if eventHref != "" {
		return "/api/v1/calendars/" + client.Itoa(calID) + "/events/" + client.Enc(eventHref) + "/attachments", nil
	}
	if taskHref != "" {
		return "/api/v1/calendars/" + client.Itoa(calID) + "/tasks/" + client.Enc(taskHref) + "/attachments", nil
	}
	return "", fmt.Errorf("--event HREF or --task HREF is required")
}

func (e *Env) fileList(args []string) error {
	fs := e.flags("file list")
	ev := fs.String("event", "", "event href")
	task := fs.String("task", "", "task href")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: file list CAL --event HREF|--task HREF")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	c, err := e.calendarRef(fs.Arg(0))
	if err != nil {
		return err
	}
	p, err := e.itemPath(c.ID, *ev, *task)
	if err != nil {
		return err
	}
	raw, err := e.cli.Get(p, nil)
	if err != nil {
		return err
	}
	list, err := decodeList[client.Attachment](raw)
	if err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(list)
	}
	rows := make([][]string, 0, len(list))
	for _, a := range list {
		rows = append(rows, []string{a.ID, a.Filename, fmt.Sprintf("%d", a.Size), a.ContentType})
	}
	e.table([]string{"ID", "FILE", "BYTES", "TYPE"}, rows)
	return nil
}

func (e *Env) fileAdd(args []string) error {
	fs := e.flags("file add")
	ev := fs.String("event", "", "event href")
	task := fs.String("task", "", "task href")
	file := fs.String("file", "", "path to upload")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 || *file == "" {
		return fmt.Errorf("usage: file add CAL --event HREF|--task HREF --file PATH")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	c, err := e.calendarRef(fs.Arg(0))
	if err != nil {
		return err
	}
	p, err := e.itemPath(c.ID, *ev, *task)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(*file)
	if err != nil {
		return err
	}
	raw, err := e.cli.Upload(p, "file", filepath.Base(*file), data)
	if err != nil {
		return err
	}
	list, err := decodeList[client.Attachment](raw)
	if err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(list)
	}
	if len(list) > 0 {
		fmt.Fprintf(e.Stdout, "uploaded %s (%s)\n", list[0].Filename, list[0].ID)
		return nil
	}
	fmt.Fprintln(e.Stdout, "uploaded")
	return nil
}

func (e *Env) fileGet(args []string) error {
	fs := e.flags("file get")
	out := fs.String("out", "", "output path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return fmt.Errorf("usage: file get CAL ID [--out PATH]")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	c, err := e.calendarRef(fs.Arg(0))
	if err != nil {
		return err
	}
	data, name, err := e.cli.Download("/api/v1/calendars/" + client.Itoa(c.ID) + "/attachments/" + client.Enc(fs.Arg(1)))
	if err != nil {
		return err
	}
	return e.saveDownload(data, name, *out)
}

func (e *Env) fileDelete(args []string) error {
	fs := e.flags("file delete")
	ev := fs.String("event", "", "event href")
	task := fs.String("task", "", "task href")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return fmt.Errorf("usage: file delete CAL --event HREF|--task HREF ID")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	c, err := e.calendarRef(fs.Arg(0))
	if err != nil {
		return err
	}
	p, err := e.itemPath(c.ID, *ev, *task)
	if err != nil {
		return err
	}
	if _, err := e.cli.JSON("DELETE", p+"/"+client.Enc(fs.Arg(1)), nil); err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(map[string]string{"status": "ok"})
	}
	fmt.Fprintf(e.Stdout, "deleted %s\n", fs.Arg(1))
	return nil
}
