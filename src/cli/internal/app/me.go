package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/devcoons/dcalcon/cli/internal/client"
)

func (e *Env) cmdMe(args []string) error {
	if len(args) == 0 || args[0] == "show" {
		if len(args) > 0 {
			args = args[1:]
		}
		return e.cmdWhoami(args)
	}
	switch args[0] {
	case "update":
		return e.meUpdate(args[1:])
	case "password":
		return e.mePassword(args[1:])
	case "export":
		return e.meExport(args[1:])
	case "backup":
		return e.meBackup(args[1:])
	case "restore":
		return e.meRestore(args[1:])
	case "revoke-sessions":
		return e.meRevoke(args[1:])
	default:
		return fmt.Errorf("me %s: try show, update, password, export, backup, restore, revoke-sessions", args[0])
	}
}

func (e *Env) meUpdate(args []string) error {
	fs := e.flags("me update")
	name := fs.String("name", "", "display name")
	email := fs.String("email", "", "")
	tz := fs.String("tz", "", "timezone")
	if err := fs.Parse(args); err != nil {
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
	if *name == "" {
		*name = u.DisplayName
	}
	if *email == "" {
		*email = u.Email
	}
	if *tz == "" {
		*tz = u.Timezone
	}
	raw, err = e.cli.JSON("PATCH", "/api/v1/me", map[string]string{
		"display_name": *name, "email": *email, "timezone": *tz,
	})
	if err != nil {
		return err
	}
	if e.JSON {
		var v any
		_ = decode(raw, &v)
		return e.printJSON(v)
	}
	fmt.Fprintln(e.Stdout, "profile updated")
	return nil
}

func (e *Env) mePassword(args []string) error {
	fs := e.flags("me password")
	cur := fs.String("current", "", "current password")
	neu := fs.String("new", "", "new password")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	current, err := e.secret(*cur, "Current password: ")
	if err != nil {
		return err
	}
	next, err := e.secret(*neu, "New password: ")
	if err != nil {
		return err
	}
	if _, err := e.cli.JSON("POST", "/api/v1/me/password", map[string]string{
		"current_password": current, "new_password": next,
	}); err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(map[string]string{"status": "ok"})
	}
	fmt.Fprintln(e.Stdout, "password changed")
	return nil
}

func (e *Env) meExport(args []string) error {
	fs := e.flags("me export")
	out := fs.String("out", "", "output zip")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return e.writeBackupZip("/api/v1/me/backup?kind=data", *out)
}

func (e *Env) meBackup(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return fmt.Errorf("usage: me backup data|full [--out FILE] [--password PASS]")
	}
	kind := args[0]
	rest := args[1:]
	switch kind {
	case "data":
		fs := e.flags("me backup data")
		out := fs.String("out", "", "output zip")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		return e.writeBackupZip("/api/v1/me/backup?kind=data", *out)
	case "full":
		fs := e.flags("me backup full")
		out := fs.String("out", "", "output zip")
		pass := fs.String("password", "", "current dashboard password")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if err := e.needAuth(); err != nil {
			return err
		}
		password, err := e.secret(*pass, "Current password: ")
		if err != nil {
			return err
		}
		data, name, err := e.cli.DownloadJSON("POST", "/api/v1/me/backup/export", map[string]string{
			"kind": "full", "password": password,
		})
		if err != nil {
			return err
		}
		return e.saveDownload(data, name, *out)
	default:
		return fmt.Errorf("me backup %s: try data or full", kind)
	}
}

func (e *Env) writeBackupZip(path, out string) error {
	if err := e.needAuth(); err != nil {
		return err
	}
	data, name, err := e.cli.Download(path)
	if err != nil {
		return err
	}
	return e.saveDownload(data, name, out)
}

func (e *Env) saveDownload(data []byte, name, out string) error {
	path := out
	if path == "" {
		path = name
	}
	if path == "-" {
		_, err := e.Stdout.Write(data)
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(map[string]string{"file": path})
	}
	fmt.Fprintf(e.Stdout, "wrote %s\n", path)
	return nil
}

func (e *Env) meRestore(args []string) error {
	fs := e.flags("me restore")
	file := fs.String("file", "", "backup zip")
	pass := fs.String("password", "", "current dashboard password (full backup)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return fmt.Errorf("--file is required")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	data, err := os.ReadFile(*file)
	if err != nil {
		return err
	}
	fields := map[string]string{}
	if *pass != "" {
		password, err := e.secret(*pass, "")
		if err != nil {
			return err
		}
		fields["password"] = password
	}
	raw, err := e.cli.UploadForm("/api/v1/me/backup", "file", filepath.Base(*file), data, fields)
	if err != nil {
		var ae *client.Error
		if errors.As(err, &ae) && ae.Status == 401 && fields["password"] == "" {
			password, perr := e.secret("", "Current password: ")
			if perr != nil {
				return perr
			}
			fields["password"] = password
			raw, err = e.cli.UploadForm("/api/v1/me/backup", "file", filepath.Base(*file), data, fields)
		}
		if err != nil {
			return err
		}
	}
	if e.JSON {
		var v any
		_ = decode(raw, &v)
		return e.printJSON(v)
	}
	fmt.Fprintln(e.Stdout, "restore complete")
	return nil
}

func (e *Env) meRevoke(args []string) error {
	if err := e.flags("me revoke-sessions").Parse(args); err != nil {
		return err
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	if _, err := e.cli.JSON("POST", "/api/v1/me/sessions/revoke", map[string]string{}); err != nil {
		return err
	}
	if e.JSON {
		return e.printJSON(map[string]string{"status": "ok"})
	}
	fmt.Fprintln(e.Stdout, "other sessions revoked")
	return nil
}

func (e *Env) cmdTOTP(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: totp setup|enable|disable|cancel")
	}
	if err := e.needAuth(); err != nil {
		return err
	}
	switch args[0] {
	case "setup":
		raw, err := e.cli.JSON("POST", "/api/v1/me/totp/setup", map[string]string{})
		if err != nil {
			return err
		}
		if e.JSON {
			var v any
			_ = decode(raw, &v)
			return e.printJSON(v)
		}
		var out struct {
			Secret  string `json:"secret"`
			OTPAuth string `json:"otpauth"`
		}
		_ = decode(raw, &out)
		e.kv("secret", out.Secret, "otpauth", out.OTPAuth)
		return nil
	case "enable":
		fs := e.flags("totp enable")
		code := fs.String("code", "", "authenticator code")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *code == "" {
			return fmt.Errorf("--code is required")
		}
		if _, err := e.cli.JSON("POST", "/api/v1/me/totp/enable", map[string]string{"code": *code}); err != nil {
			return err
		}
		fmt.Fprintln(e.Stdout, "authenticator enabled")
		return nil
	case "disable":
		fs := e.flags("totp disable")
		code := fs.String("code", "", "")
		pass := fs.String("password", "", "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		body := map[string]string{}
		if *code != "" {
			body["code"] = *code
		}
		if *pass != "" {
			body["password"] = *pass
		}
		if _, err := e.cli.JSON("POST", "/api/v1/me/totp/disable", body); err != nil {
			return err
		}
		fmt.Fprintln(e.Stdout, "authenticator disabled")
		return nil
	case "cancel":
		if _, err := e.cli.JSON("DELETE", "/api/v1/me/totp/setup", nil); err != nil {
			return err
		}
		fmt.Fprintln(e.Stdout, "pending setup cancelled")
		return nil
	default:
		return fmt.Errorf("totp %s: try setup, enable, disable, cancel", args[0])
	}
}

func (e *Env) cmdAppPassword(args []string) error {
	if len(args) == 0 || args[0] == "list" {
		if len(args) > 0 {
			args = args[1:]
		}
		if err := e.flags("app-password list").Parse(args); err != nil {
			return err
		}
		if err := e.needAuth(); err != nil {
			return err
		}
		raw, err := e.cli.Get("/api/v1/me/app-passwords", nil)
		if err != nil {
			return err
		}
		list, err := decodeList[client.AppPassword](raw)
		if err != nil {
			return err
		}
		if e.JSON {
			return e.printJSON(list)
		}
		rows := make([][]string, 0, len(list))
		for _, p := range list {
			rows = append(rows, []string{formatInt(p.ID), p.Name, p.Prefix, p.CreatedAt})
		}
		e.table([]string{"ID", "NAME", "PREFIX", "CREATED"}, rows)
		return nil
	}
	switch args[0] {
	case "create":
		fs := e.flags("app-password create")
		name := fs.String("name", "", "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if err := e.needAuth(); err != nil {
			return err
		}
		raw, err := e.cli.JSON("POST", "/api/v1/me/app-passwords", map[string]string{"name": *name})
		if err != nil {
			return err
		}
		var p client.AppPassword
		if err := decode(raw, &p); err != nil {
			return err
		}
		if e.JSON {
			return e.printJSON(p)
		}
		fmt.Fprintf(e.Stdout, "created %d %s\npassword (shown once): %s\n", p.ID, p.Name, p.Password)
		return nil
	case "delete":
		fs := e.flags("app-password delete")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() < 1 {
			return fmt.Errorf("usage: app-password delete ID")
		}
		if err := e.needAuth(); err != nil {
			return err
		}
		if _, err := e.cli.JSON("DELETE", "/api/v1/me/app-passwords/"+fs.Arg(0), nil); err != nil {
			return err
		}
		fmt.Fprintln(e.Stdout, "revoked")
		return nil
	default:
		return fmt.Errorf("app-password %s: try list, create, delete", args[0])
	}
}
