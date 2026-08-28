package app

import (
	"fmt"

	"github.com/devcoons/dcalcon/cli/internal/client"
)

func (e *Env) cmdUser(args []string) error {
	if len(args) == 0 || args[0] == "list" {
		if len(args) > 0 {
			args = args[1:]
		}
		if err := e.flags("user list").Parse(args); err != nil {
			return err
		}
		if err := e.needAuth(); err != nil {
			return err
		}
		raw, err := e.cli.Get("/api/v1/admin/users", nil)
		if err != nil {
			return err
		}
		list, err := decodeList[client.User](raw)
		if err != nil {
			return err
		}
		if e.JSON {
			return e.printJSON(list)
		}
		rows := make([][]string, 0, len(list))
		for _, u := range list {
			rows = append(rows, []string{formatInt(u.ID), u.Username, u.DisplayName, u.Email, u.Role, u.Status, yn(u.TOTPEnabled)})
		}
		e.table([]string{"ID", "USERNAME", "NAME", "EMAIL", "ROLE", "STATUS", "TOTP"}, rows)
		return nil
	}
	switch args[0] {
	case "create":
		return e.userCreate(args[1:])
	case "update":
		return e.userUpdate(args[1:])
	case "password":
		return e.userPassword(args[1:])
	case "recovery":
		return e.userRecovery(args[1:])
	case "disable-totp":
		return e.userDisableTOTP(args[1:])
	default:
		return fmt.Errorf("user %s: try list, create, update, password, recovery, disable-totp", args[0])
	}
}

func (e *Env) userCreate(args []string) error {
	fs := e.flags("user create")
	user := fs.String("user", "", "username")
	email := fs.String("email", "", "")
	pass := fs.String("password", "", "")
	name := fs.String("name", "", "display name")
	role := fs.String("role", "user", "user or admin")
	tz := fs.String("tz", "UTC", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *user == "" || *email == "" {
		return fmt.Errorf("--user and --email are required")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	password, err := e.secret(*pass, "Password: ")
	if err != nil {
		return err
	}
	display := *name
	if display == "" {
		display = *user
	}
	raw, err := e.cli.JSON("POST", "/api/v1/admin/users", map[string]string{
		"username": *user, "email": *email, "password": password,
		"display_name": display, "role": *role, "timezone": *tz,
	})
	if err != nil {
		return err
	}
	if e.JSON {
		var v any
		_ = decode(raw, &v)
		return e.printJSON(v)
	}
	fmt.Fprintf(e.Stdout, "created user %s\n", *user)
	return nil
}

func (e *Env) userUpdate(args []string) error {
	fs := e.flags("user update")
	email := fs.String("email", "", "")
	name := fs.String("name", "", "")
	role := fs.String("role", "", "")
	status := fs.String("status", "", "active or disabled")
	tz := fs.String("tz", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: user update USER [--email] [--name] [--role] [--status] [--tz]")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	u, err := e.userRef(fs.Arg(0))
	if err != nil {
		return err
	}
	body := map[string]string{}
	if *email != "" {
		body["email"] = *email
	}
	if *name != "" {
		body["display_name"] = *name
	}
	if *role != "" {
		body["role"] = *role
	}
	if *status != "" {
		body["status"] = *status
	}
	if *tz != "" {
		body["timezone"] = *tz
	}
	raw, err := e.cli.JSON("PATCH", "/api/v1/admin/users/"+client.Itoa(u.ID), body)
	if err != nil {
		return err
	}
	if e.JSON {
		var v any
		_ = decode(raw, &v)
		return e.printJSON(v)
	}
	fmt.Fprintf(e.Stdout, "updated %s\n", u.Username)
	return nil
}

func (e *Env) userPassword(args []string) error {
	fs := e.flags("user password")
	pass := fs.String("password", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: user password USER --password PASS")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	u, err := e.userRef(fs.Arg(0))
	if err != nil {
		return err
	}
	password, err := e.secret(*pass, "New password: ")
	if err != nil {
		return err
	}
	if _, err := e.cli.JSON("POST", "/api/v1/admin/users/"+client.Itoa(u.ID)+"/password", map[string]string{"password": password}); err != nil {
		return err
	}
	fmt.Fprintf(e.Stdout, "password reset for %s\n", u.Username)
	return nil
}

func (e *Env) userRecovery(args []string) error {
	fs := e.flags("user recovery")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: user recovery USER")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	u, err := e.userRef(fs.Arg(0))
	if err != nil {
		return err
	}
	raw, err := e.cli.JSON("POST", "/api/v1/admin/users/"+client.Itoa(u.ID)+"/recovery", map[string]string{})
	if err != nil {
		return err
	}
	if e.JSON {
		var v any
		_ = decode(raw, &v)
		return e.printJSON(v)
	}
	var out struct {
		RecoveryURL string `json:"recovery_url"`
		Emailed     bool   `json:"emailed"`
	}
	_ = decode(raw, &out)
	if out.Emailed {
		fmt.Fprintf(e.Stdout, "reset email sent to %s\n", u.Email)
	} else {
		fmt.Fprintln(e.Stdout, "SMTP is not configured. Copy this reset link:")
	}
	if out.RecoveryURL != "" {
		fmt.Fprintln(e.Stdout, out.RecoveryURL)
	}
	return nil
}

func (e *Env) userDisableTOTP(args []string) error {
	fs := e.flags("user disable-totp")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: user disable-totp USER")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	u, err := e.userRef(fs.Arg(0))
	if err != nil {
		return err
	}
	if _, err := e.cli.JSON("POST", "/api/v1/admin/users/"+client.Itoa(u.ID)+"/totp/disable", map[string]string{}); err != nil {
		return err
	}
	fmt.Fprintf(e.Stdout, "authenticator disabled for %s\n", u.Username)
	return nil
}

func (e *Env) cmdAudit(args []string) error {
	if err := e.flags("audit").Parse(args); err != nil {
		return err
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	raw, err := e.cli.Get("/api/v1/admin/audit", nil)
	if err != nil {
		return err
	}
	list, err := decodeList[client.AuditEntry](raw)
	if err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(list)
	}
	rows := make([][]string, 0, len(list))
	for _, a := range list {
		rows = append(rows, []string{formatInt(a.ID), a.At, a.Actor, a.Action, a.Detail})
	}
	e.table([]string{"ID", "AT", "ACTOR", "ACTION", "DETAIL"}, rows)
	return nil
}

func (e *Env) cmdOutbox(args []string) error {
	if err := e.flags("outbox").Parse(args); err != nil {
		return err
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	raw, err := e.cli.Get("/api/v1/admin/recovery-outbox", nil)
	if err != nil {
		return err
	}
	list, err := decodeList[client.RecoveryOutbox](raw)
	if err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(list)
	}
	rows := make([][]string, 0, len(list))
	for _, m := range list {
		rows = append(rows, []string{formatInt(m.ID), m.Username, m.Email, m.Delivered, m.CreatedAt})
	}
	e.table([]string{"ID", "USER", "EMAIL", "DELIVERED", "AT"}, rows)
	return nil
}
