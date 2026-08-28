package app

import (
	"flag"
	"fmt"
	"os"

	"github.com/devcoons/dcalcon/cli/internal/client"
)

func (e *Env) cmdContact(args []string) error {
	if len(args) == 0 || args[0] == "list" {
		if len(args) > 0 && args[0] == "list" {
			args = args[1:]
		}
		return e.contactList(args)
	}
	switch args[0] {
	case "books":
		return e.contactBooks(args[1:])
	case "get":
		return e.contactGet(args[1:])
	case "create":
		return e.contactCreate(args[1:])
	case "update":
		return e.contactUpdate(args[1:])
	case "delete":
		return e.contactDelete(args[1:])
	case "export":
		return e.contactExport(args[1:])
	case "import":
		return e.contactImport(args[1:])
	default:
		return fmt.Errorf("contact %s: try books, list, get, create, update, delete, export, import", args[0])
	}
}

func (e *Env) contactBooks(args []string) error {
	if err := e.flags("contact books").Parse(args); err != nil {
		return err
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	list, err := e.books()
	if err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(list)
	}
	rows := make([][]string, 0, len(list))
	for _, b := range list {
		rows = append(rows, []string{formatInt(b.ID), b.Slug, b.Name, yn(b.ReadOnly)})
	}
	e.table([]string{"ID", "SLUG", "NAME", "RO"}, rows)
	return nil
}

func (e *Env) contactList(args []string) error {
	fs := e.flags("contact list")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	ref := ""
	if fs.NArg() > 0 {
		ref = fs.Arg(0)
	}
	b, err := e.bookRef(ref)
	if err != nil {
		return err
	}
	raw, err := e.cli.Get("/api/v1/addressbooks/"+client.Itoa(b.ID)+"/contacts", nil)
	if err != nil {
		return err
	}
	list, err := decodeList[client.Contact](raw)
	if err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(list)
	}
	rows := make([][]string, 0, len(list))
	for _, c := range list {
		rows = append(rows, []string{c.Href, c.FN, c.Email, c.Tel, c.Org})
	}
	e.table([]string{"HREF", "NAME", "EMAIL", "TEL", "ORG"}, rows)
	return nil
}

func (e *Env) contactGet(args []string) error {
	fs := e.flags("contact get")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return fmt.Errorf("usage: contact get BOOK HREF")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	b, err := e.bookRef(fs.Arg(0))
	if err != nil {
		return err
	}
	raw, err := e.cli.Get("/api/v1/addressbooks/"+client.Itoa(b.ID)+"/contacts/"+client.Enc(fs.Arg(1)), nil)
	if err != nil {
		return err
	}
	if e.JSON {
		var v any
		_ = decode(raw, &v)
		return e.printJSON(v)
	}
	var c client.Contact
	if err := decode(raw, &c); err != nil {
		return err
	}
	e.kv("href", c.Href, "name", c.FN, "email", c.Email, "tel", c.Tel, "org", c.Org, "title", c.Title, "birthday", c.BDay, "anniversary", c.Anniversary, "note", c.Note)
	return nil
}

func contactWrite(fn, email, tel, org, note, title, bday, ann string) map[string]string {
	return map[string]string{
		"fn": fn, "email": email, "tel": tel, "org": org, "note": note,
		"title": title, "bday": bday, "anniversary": ann,
	}
}

func (e *Env) contactCreate(args []string) error {
	fs := e.flags("contact create")
	fn := fs.String("fn", "", "full name")
	email := fs.String("email", "", "")
	tel := fs.String("tel", "", "")
	org := fs.String("org", "", "")
	note := fs.String("note", "", "")
	title := fs.String("title", "", "")
	bday := fs.String("bday", "", "birthday YYYY-MM-DD")
	ann := fs.String("anniversary", "", "anniversary YYYY-MM-DD")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *fn == "" {
		return fmt.Errorf("--fn is required")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	ref := ""
	if fs.NArg() > 0 {
		ref = fs.Arg(0)
	}
	b, err := e.bookRef(ref)
	if err != nil {
		return err
	}
	raw, err := e.cli.JSON("POST", "/api/v1/addressbooks/"+client.Itoa(b.ID)+"/contacts", contactWrite(*fn, *email, *tel, *org, *note, *title, *bday, *ann))
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

func (e *Env) contactUpdate(args []string) error {
	fs := e.flags("contact update")
	fn := fs.String("fn", "", "full name")
	email := fs.String("email", "", "")
	tel := fs.String("tel", "", "")
	org := fs.String("org", "", "")
	note := fs.String("note", "", "")
	title := fs.String("title", "", "")
	bday := fs.String("bday", "", "birthday YYYY-MM-DD")
	ann := fs.String("anniversary", "", "anniversary YYYY-MM-DD")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 || *fn == "" {
		return fmt.Errorf("usage: contact update BOOK HREF --fn NAME")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	b, err := e.bookRef(fs.Arg(0))
	if err != nil {
		return err
	}
	raw, err := e.cli.Get("/api/v1/addressbooks/"+client.Itoa(b.ID)+"/contacts/"+client.Enc(fs.Arg(1)), nil)
	if err != nil {
		return err
	}
	var cur client.Contact
	if err := decode(raw, &cur); err != nil {
		return err
	}
	body := contactWrite(cur.FN, cur.Email, cur.Tel, cur.Org, cur.Note, cur.Title, cur.BDay, cur.Anniversary)
	body["fn"] = *fn
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "email":
			body["email"] = *email
		case "tel":
			body["tel"] = *tel
		case "org":
			body["org"] = *org
		case "note":
			body["note"] = *note
		case "title":
			body["title"] = *title
		case "bday":
			body["bday"] = *bday
		case "anniversary":
			body["anniversary"] = *ann
		}
	})
	raw, err = e.cli.JSON("PUT", "/api/v1/addressbooks/"+client.Itoa(b.ID)+"/contacts/"+client.Enc(fs.Arg(1)), body)
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

func (e *Env) contactDelete(args []string) error {
	fs := e.flags("contact delete")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return fmt.Errorf("usage: contact delete BOOK HREF")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	b, err := e.bookRef(fs.Arg(0))
	if err != nil {
		return err
	}
	if _, err := e.cli.JSON("DELETE", "/api/v1/addressbooks/"+client.Itoa(b.ID)+"/contacts/"+client.Enc(fs.Arg(1)), nil); err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(map[string]string{"status": "ok"})
	}
	fmt.Fprintf(e.Stdout, "deleted %s\n", fs.Arg(1))
	return nil
}

func (e *Env) contactExport(args []string) error {
	fs := e.flags("contact export")
	out := fs.String("out", "", "output file")
	href := fs.String("href", "", "single contact href")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	ref := ""
	if fs.NArg() > 0 {
		ref = fs.Arg(0)
	}
	b, err := e.bookRef(ref)
	if err != nil {
		return err
	}
	p := "/api/v1/addressbooks/" + client.Itoa(b.ID) + "/contacts/export"
	if *href != "" {
		p = "/api/v1/addressbooks/" + client.Itoa(b.ID) + "/contacts/" + client.Enc(*href) + "/vcard"
	}
	data, name, err := e.cli.Download(p)
	if err != nil {
		return err
	}
	return e.saveDownload(data, name, *out)
}

func (e *Env) contactImport(args []string) error {
	fs := e.flags("contact import")
	file := fs.String("file", "", "vCard file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 || *file == "" {
		return fmt.Errorf("usage: contact import BOOK --file FILE.vcf")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	b, err := e.bookRef(fs.Arg(0))
	if err != nil {
		return err
	}
	rawFile, err := os.ReadFile(*file)
	if err != nil {
		return err
	}
	raw, err := e.cli.Raw("POST", "/api/v1/addressbooks/"+client.Itoa(b.ID)+"/contacts/import", rawFile, "text/vcard")
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
	return nil
}
