package app

import (
	"fmt"

	"github.com/devcoons/dcalcon/cli/internal/client"
)

func (e *Env) cmdDates(args []string) error {
	if len(args) == 0 || args[0] == "show" {
		if len(args) > 0 {
			args = args[1:]
		}
		if err := e.flags("dates show").Parse(args); err != nil {
			return err
		}
		if err := e.needAuth(); err != nil {
			return err
		}
		raw, err := e.cli.Get("/api/v1/settings/important-dates", nil)
		if err != nil {
			return err
		}
		var d client.ImportantDates
		if err := decode(raw, &d); err != nil {
			return err
		}
		if e.JSON {
			return e.printJSON(d)
		}
		e.kv("enabled", yn(d.Enabled), "birthdays", yn(d.IncludeBirthdays), "anniversaries", yn(d.IncludeAnniversaries), "alarms", fmt.Sprintf("%v", d.AlarmOffsets))
		return nil
	}
	if args[0] != "set" {
		return fmt.Errorf("dates %s: try show or set", args[0])
	}
	fs := e.flags("dates set")
	on := fs.Bool("on", false, "enable")
	off := fs.Bool("off", false, "disable")
	bday := fs.Bool("birthdays", false, "include birthdays")
	noBday := fs.Bool("no-birthdays", false, "")
	ann := fs.Bool("anniversaries", false, "")
	noAnn := fs.Bool("no-anniversaries", false, "")
	alarmDay := fs.Bool("alarm-day", false, "remind 1 day before")
	noAlarmDay := fs.Bool("no-alarm-day", false, "")
	alarmWeek := fs.Bool("alarm-week", false, "remind 1 week before")
	noAlarmWeek := fs.Bool("no-alarm-week", false, "")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	raw, err := e.cli.Get("/api/v1/settings/important-dates", nil)
	if err != nil {
		return err
	}
	var d client.ImportantDates
	if err := decode(raw, &d); err != nil {
		return err
	}
	if *on {
		d.Enabled = true
	}
	if *off {
		d.Enabled = false
	}
	if *bday {
		d.IncludeBirthdays = true
	}
	if *noBday {
		d.IncludeBirthdays = false
	}
	if *ann {
		d.IncludeAnniversaries = true
	}
	if *noAnn {
		d.IncludeAnniversaries = false
	}
	if d.AlarmOffsets == nil {
		d.AlarmOffsets = []string{"-P1D"}
	}
	d.AlarmOffsets = setOffset(d.AlarmOffsets, "-P1D", *alarmDay, *noAlarmDay)
	d.AlarmOffsets = setOffset(d.AlarmOffsets, "-P7D", *alarmWeek, *noAlarmWeek)
	if len(d.AlarmOffsets) == 0 {
		d.AlarmOffsets = []string{"-P1D"}
	}
	raw, err = e.cli.JSON("PUT", "/api/v1/settings/important-dates", d)
	if err != nil {
		return err
	}
	if e.JSON {
		var v any
		_ = decode(raw, &v)
		return e.printJSON(v)
	}
	fmt.Fprintln(e.Stdout, "important dates saved")
	return nil
}

func (e *Env) cmdMail(args []string) error {
	if err := e.flags("mail").Parse(args); err != nil {
		return err
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	raw, err := e.cli.Get("/api/v1/mail", nil)
	if err != nil {
		return err
	}
	var s client.MailStatus
	if err := decode(raw, &s); err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(s)
	}
	e.kv("google oauth", yn(s.GoogleConfigured), "microsoft oauth", yn(s.MicrosoftConfigured), "server smtp", yn(s.ServerSMTP), "token key", yn(s.TokenKey))
	return nil
}

func (e *Env) cmdAccount(args []string) error {
	if len(args) == 0 || args[0] == "list" {
		if len(args) > 0 {
			args = args[1:]
		}
		if err := e.flags("account list").Parse(args); err != nil {
			return err
		}
		if err := e.needAuth(); err != nil {
			return err
		}
		raw, err := e.cli.Get("/api/v1/accounts", nil)
		if err != nil {
			return err
		}
		list, err := decodeList[client.Account](raw)
		if err != nil {
			return err
		}
		if e.JSON {
			return e.printJSON(list)
		}
		rows := make([][]string, 0, len(list))
		for _, a := range list {
			rows = append(rows, []string{formatInt(a.ID), a.Provider, a.Email, a.Status, a.LastError})
		}
		e.table([]string{"ID", "PROVIDER", "EMAIL", "STATUS", "ERROR"}, rows)
		return nil
	}
	switch args[0] {
	case "connect":
		return e.accountConnect(args[1:])
	case "test":
		fs := e.flags("account test")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() < 1 {
			return fmt.Errorf("usage: account test ID")
		}
		if err := e.needAuth(); err != nil {
			return err
		}
		if _, err := e.cli.JSON("POST", "/api/v1/accounts/"+fs.Arg(0)+"/test", map[string]string{}); err != nil {
			return err
		}
		fmt.Fprintln(e.Stdout, "ok")
		return nil
	case "disconnect", "delete":
		fs := e.flags("account disconnect")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() < 1 {
			return fmt.Errorf("usage: account disconnect ID")
		}
		if err := e.needAuth(); err != nil {
			return err
		}
		if _, err := e.cli.JSON("DELETE", "/api/v1/accounts/"+fs.Arg(0), nil); err != nil {
			return err
		}
		fmt.Fprintln(e.Stdout, "disconnected")
		return nil
	default:
		return fmt.Errorf("account %s: try list, connect, test, disconnect", args[0])
	}
}

func (e *Env) accountConnect(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: account connect smtp|google|microsoft ...")
	}
	provider := args[0]
	fs := e.flags("account connect")
	email := fs.String("email", "", "")
	host := fs.String("host", "", "")
	port := fs.Int("port", 587, "")
	user := fs.String("username", "", "")
	pass := fs.String("password", "", "")
	from := fs.String("from", "", "")
	origin := fs.String("origin", "", "dashboard origin for OAuth redirect")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	body := map[string]any{"provider": provider}
	switch provider {
	case "smtp":
		if *email == "" || *host == "" {
			return fmt.Errorf("smtp requires --email and --host")
		}
		password, err := e.secret(*pass, "SMTP password: ")
		if err != nil {
			return err
		}
		userName := *user
		if userName == "" {
			userName = *email
		}
		body["email"] = *email
		body["host"] = *host
		body["port"] = *port
		body["username"] = userName
		body["password"] = password
		if *from != "" {
			body["from"] = *from
		}
	case "google", "microsoft":
		if *origin != "" {
			body["origin"] = *origin
		}
	default:
		return fmt.Errorf("provider must be smtp, google, or microsoft")
	}
	raw, err := e.cli.JSON("POST", "/api/v1/accounts", body)
	if err != nil {
		return err
	}
	if e.JSON {
		var v any
		_ = decode(raw, &v)
		return e.printJSON(v)
	}
	var out struct {
		AuthorizeURL string `json:"authorize_url"`
		ID           int64  `json:"id"`
		Email        string `json:"email"`
	}
	_ = decode(raw, &out)
	if out.AuthorizeURL != "" {
		fmt.Fprintf(e.Stdout, "open this URL to finish connecting:\n%s\n", out.AuthorizeURL)
		return nil
	}
	fmt.Fprintf(e.Stdout, "connected %s\n", out.Email)
	return nil
}

func setOffset(list []string, code string, on, off bool) []string {
	if !on && !off {
		return list
	}
	next := make([]string, 0, len(list)+1)
	for _, a := range list {
		if a != code {
			next = append(next, a)
		}
	}
	if on {
		next = append(next, code)
	}
	return next
}
