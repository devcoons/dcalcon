package app

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devcoons/dcalcon/cli/internal/client"
)

func (e *Env) cmdLogin(args []string) error {
	fs := e.flags("login")
	user := fs.String("user", "", "username")
	pass := fs.String("password", "", "password (prompted if omitted)")
	totp := fs.String("totp", "", "authenticator code")
	if err := fs.Parse(args); err != nil {
		return err
	}
	username := *user
	if username == "" && fs.NArg() > 0 {
		username = fs.Arg(0)
	}
	if username == "" {
		return fmt.Errorf("username is required (--user)")
	}
	password, err := e.secret(*pass, "Password: ")
	if err != nil {
		return err
	}
	u, sid, err := e.cli.Login(username, password, *totp)
	if err != nil {
		return err
	}
	e.cfg.Session = sid
	e.cfg.Username = u.Username
	e.cfg.URL = e.cli.Base
	if err := e.save(); err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(u)
	}
	fmt.Fprintf(e.Stdout, "signed in as %s (%s)\n", u.Username, u.Role)
	return nil
}

func (e *Env) cmdLogout(args []string) error {
	if err := e.flags("logout").Parse(args); err != nil {
		return err
	}
	if e.cli.Session != "" {
		_, _ = e.cli.JSON("POST", "/api/v1/auth/logout", map[string]string{})
	}
	e.cfg.Session = ""
	if err := e.save(); err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(map[string]string{"status": "ok"})
	}
	fmt.Fprintln(e.Stdout, "signed out")
	return nil
}

func (e *Env) cmdWhoami(args []string) error {
	if err := e.flags("whoami").Parse(args); err != nil {
		return err
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	raw, err := e.cli.Get("/api/v1/me", nil)
	if err != nil {
		return err
	}
	var u client.User
	if err := decode(raw, &u); err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(u)
	}
	e.kv("username", u.Username, "name", u.DisplayName, "email", u.Email, "role", u.Role, "status", u.Status, "timezone", u.Timezone, "totp", yn(u.TOTPEnabled))
	return nil
}

func (e *Env) cmdRecover(args []string) error {
	fs := e.flags("recover")
	email := fs.String("email", "", "account email")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *email == "" {
		return fmt.Errorf("--email is required")
	}
	raw, err := e.cli.JSON("POST", "/api/v1/auth/recover", map[string]string{"email": *email})
	if err != nil {
		return err
	}
	if e.JSON {
		var v any
		_ = json.Unmarshal(raw, &v)
		return e.printJSON(v)
	}
	fmt.Fprintln(e.Stdout, "if that address exists, a reset link is on its way")
	return nil
}

func (e *Env) cmdReset(args []string) error {
	fs := e.flags("reset")
	totp := fs.Bool("totp", false, "reset with authenticator code instead of email token")
	token := fs.String("token", "", "reset token from the email link")
	user := fs.String("user", "", "username (with --totp)")
	code := fs.String("code", "", "authenticator code")
	pass := fs.String("password", "", "new password")
	if err := fs.Parse(args); err != nil {
		return err
	}
	password, err := e.secret(*pass, "New password: ")
	if err != nil {
		return err
	}
	var raw []byte
	if *totp {
		if *user == "" || *code == "" {
			return fmt.Errorf("--user and --code are required with --totp")
		}
		raw, err = e.cli.JSON("POST", "/api/v1/auth/reset-totp", map[string]string{
			"username": *user, "code": *code, "password": password,
		})
	} else {
		tok := *token
		if tok == "" && fs.NArg() > 0 {
			tok = fs.Arg(0)
		}
		if tok == "" {
			return fmt.Errorf("--token is required")
		}
		raw, err = e.cli.JSON("POST", "/api/v1/auth/reset", map[string]string{
			"token": tok, "password": password,
		})
	}
	if err != nil {
		return err
	}
	if e.JSON {
		var v any
		_ = json.Unmarshal(raw, &v)
		return e.printJSON(v)
	}
	fmt.Fprintln(e.Stdout, "password updated")
	return nil
}

func (e *Env) cmdOverview(args []string) error {
	if err := e.flags("overview").Parse(args); err != nil {
		return err
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	raw, err := e.cli.Get("/api/v1/overview", nil)
	if err != nil {
		return err
	}
	var o client.Overview
	if err := decode(raw, &o); err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(o)
	}
	e.kv(
		"calendars", fmt.Sprintf("%d", o.Calendars),
		"contacts", fmt.Sprintf("%d", o.Contacts),
		"pending invites", fmt.Sprintf("%d", o.PendingInvitations),
		"important dates", yn(o.ImportantDatesEnabled),
		"mail", strings.TrimSpace(o.MailAddress),
		"totp", yn(o.TOTPEnabled),
	)
	return nil
}

func (e *Env) cmdSetup(args []string) error {
	if err := e.flags("setup").Parse(args); err != nil {
		return err
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	raw, err := e.cli.Get("/api/v1/setup", nil)
	if err != nil {
		return err
	}
	var s client.Setup
	if err := decode(raw, &s); err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(s)
	}
	e.kv(
		"public url", s.PublicURL,
		"username", s.Username,
		"caldav", s.CalDAVWellKnown,
		"carddav", s.CardDAVWellKnown,
		"principal", s.PrincipalURL,
		"calendar home", s.CalendarHome,
		"address books", s.AddressBookHome,
		"scheduling", s.SchedulingAddress,
	)
	return nil
}

func (e *Env) cmdDirectory(args []string) error {
	if err := e.flags("directory").Parse(args); err != nil {
		return err
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	raw, err := e.cli.Get("/api/v1/directory", nil)
	if err != nil {
		return err
	}
	list, err := decodeList[client.DirectoryUser](raw)
	if err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(list)
	}
	rows := make([][]string, 0, len(list))
	for _, u := range list {
		rows = append(rows, []string{u.Username, u.DisplayName, u.LocalEmail})
	}
	e.table([]string{"USERNAME", "NAME", "ADDRESS"}, rows)
	return nil
}
