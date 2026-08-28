package app

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/devcoons/dcalcon/cli/internal/client"
	"github.com/devcoons/dcalcon/cli/internal/config"
	"golang.org/x/term"
)

const version = "0.1.0-dev"

type Env struct {
	Args       []string
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
	ConfigPath string
	URL        string
	JSON       bool
	cfg        *config.File
	cli        *client.Client
}

func Main(args []string) int {
	e := &Env{Args: args, Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}
	if err := e.Run(); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(e.Stderr, "error: %s\n", err.Error())
		var ae *client.Error
		if errors.As(err, &ae) && ae.Status == 401 {
			return 1
		}
		return 1
	}
	return 0
}

func (e *Env) Run() error {
	args := e.Args
	if len(args) > 0 {
		args = args[1:]
	}
	rest, err := e.parseGlobal(args)
	if err != nil {
		return err
	}
	if len(rest) == 0 || rest[0] == "help" || rest[0] == "-h" || rest[0] == "--help" {
		return e.help(rest)
	}
	if rest[0] == "version" || rest[0] == "-v" || rest[0] == "--version" {
		fmt.Fprintf(e.Stdout, "dcalcon-cli %s\n", version)
		return nil
	}
	if err := e.load(); err != nil {
		return err
	}
	cmd, rest := rest[0], rest[1:]
	switch cmd {
	case "login":
		return e.cmdLogin(rest)
	case "logout":
		return e.cmdLogout(rest)
	case "whoami":
		return e.cmdWhoami(rest)
	case "recover":
		return e.cmdRecover(rest)
	case "reset":
		return e.cmdReset(rest)
	case "overview":
		return e.cmdOverview(rest)
	case "setup":
		return e.cmdSetup(rest)
	case "directory":
		return e.cmdDirectory(rest)
	case "calendar", "calendars":
		return e.cmdCalendar(rest)
	case "event", "events":
		return e.cmdEvent(rest)
	case "task", "tasks":
		return e.cmdTask(rest)
	case "file", "files", "attachment", "attachments":
		return e.cmdFile(rest)
	case "contact", "contacts":
		return e.cmdContact(rest)
	case "invitation", "invitations":
		return e.cmdInvitation(rest)
	case "freebusy":
		return e.cmdFreebusy(rest)
	case "me":
		return e.cmdMe(rest)
	case "totp":
		return e.cmdTOTP(rest)
	case "app-password", "app-passwords":
		return e.cmdAppPassword(rest)
	case "dates":
		return e.cmdDates(rest)
	case "mail":
		return e.cmdMail(rest)
	case "account", "accounts":
		return e.cmdAccount(rest)
	case "user", "users":
		return e.cmdUser(rest)
	case "audit":
		return e.cmdAudit(rest)
	case "outbox":
		return e.cmdOutbox(rest)
	default:
		return fmt.Errorf("unknown command %q — try dcalcon-cli help", cmd)
	}
}

func (e *Env) parseGlobal(args []string) ([]string, error) {
	rest, url, cfg, jsonOut, help, ver, err := extractGlobals(args)
	if err != nil {
		return nil, err
	}
	e.URL = strings.TrimRight(strings.TrimSpace(url), "/")
	if cfg != "" {
		e.ConfigPath = cfg
	}
	e.JSON = jsonOut
	if help {
		return []string{"help"}, nil
	}
	if ver {
		return []string{"version"}, nil
	}
	return rest, nil
}

// extractGlobals pulls --url, --config, and --json from anywhere in argv so
// `dcalcon-cli calendar list --json` and `dcalcon-cli login --url http://…` work.
func extractGlobals(args []string) (rest []string, url, config string, jsonOut, help, ver bool, err error) {
	seenCmd := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			rest = append(rest, args[i+1:]...)
			break
		}
		name, val, hasVal, isFlag := splitFlag(a)
		if !isFlag {
			seenCmd = true
			rest = append(rest, a)
			continue
		}
		preCmdGlobal := !seenCmd && (name == "help" || name == "h" || name == "version" || name == "v")
		if name != "url" && name != "config" && name != "json" && !preCmdGlobal {
			if !seenCmd {
				return nil, "", "", false, false, false, fmt.Errorf("unknown flag: -%s", name)
			}
			rest = append(rest, a)
			continue
		}
		switch name {
		case "json":
			jsonOut = true
		case "help", "h":
			help = true
		case "version", "v":
			ver = true
		case "url", "config":
			if !hasVal {
				if i+1 >= len(args) {
					return nil, "", "", false, false, false, fmt.Errorf("flag needs an argument: --%s", name)
				}
				i++
				val = args[i]
			}
			if name == "url" {
				url = val
			} else {
				config = val
			}
		}
	}
	return rest, url, config, jsonOut, help, ver, nil
}

func splitFlag(a string) (name, val string, hasVal, ok bool) {
	if a == "-" || a == "" || a[0] != '-' {
		return "", "", false, false
	}
	body := a[1:]
	if strings.HasPrefix(body, "-") {
		body = body[1:]
	}
	if body == "" {
		return "", "", false, false
	}
	if i := strings.IndexByte(body, '='); i >= 0 {
		return body[:i], body[i+1:], true, true
	}
	return body, "", false, true
}

func (e *Env) load() error {
	if e.ConfigPath == "" {
		e.ConfigPath = config.Path()
	}
	f, err := config.Load(e.ConfigPath)
	if err != nil {
		return err
	}
	if e.URL != "" {
		f.URL = e.URL
	}
	if s := strings.TrimSpace(os.Getenv("DCALCON_SESSION")); s != "" {
		f.Session = s
	}
	e.cfg = f
	e.cli = client.New(f.URL, f.Session)
	return nil
}

func (e *Env) save() error {
	if e.cfg == nil {
		return nil
	}
	return e.cfg.Save(e.ConfigPath)
}

func (e *Env) flags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(e.Stderr)
	return fs
}

func (e *Env) needAuth() error {
	if e.cli == nil || e.cli.Session == "" {
		return fmt.Errorf("not signed in — run: dcalcon-cli login")
	}
	return nil
}

func (e *Env) printJSON(v any) error {
	enc := json.NewEncoder(e.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func (e *Env) kv(pairs ...string) {
	if e.JSON {
		return
	}
	w := 0
	for i := 0; i+1 < len(pairs); i += 2 {
		if len(pairs[i]) > w {
			w = len(pairs[i])
		}
	}
	for i := 0; i+1 < len(pairs); i += 2 {
		fmt.Fprintf(e.Stdout, "%-*s  %s\n", w, pairs[i], pairs[i+1])
	}
}

func (e *Env) table(cols []string, rows [][]string) {
	if e.JSON {
		return
	}
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = len(c)
	}
	for _, row := range rows {
		for i := 0; i < len(cols) && i < len(row); i++ {
			if len(row[i]) > widths[i] {
				widths[i] = len(row[i])
			}
		}
	}
	for i, c := range cols {
		if i > 0 {
			fmt.Fprint(e.Stdout, "  ")
		}
		fmt.Fprintf(e.Stdout, "%-*s", widths[i], c)
	}
	fmt.Fprintln(e.Stdout)
	for _, row := range rows {
		for i := 0; i < len(cols); i++ {
			if i > 0 {
				fmt.Fprint(e.Stdout, "  ")
			}
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			fmt.Fprintf(e.Stdout, "%-*s", widths[i], cell)
		}
		fmt.Fprintln(e.Stdout)
	}
}

func (e *Env) secret(flagVal, prompt string) (string, error) {
	if strings.TrimSpace(flagVal) != "" {
		return flagVal, nil
	}
	if f, ok := e.Stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fmt.Fprint(e.Stderr, prompt)
		b, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(e.Stderr)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	raw, err := io.ReadAll(e.Stdin)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

func decode[T any](raw []byte, dest *T) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, dest)
}

func decodeList[T any](raw []byte) ([]T, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return []T{}, nil
	}
	var out []T
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []T{}
	}
	return out, nil
}

func parseID(s string) (int64, bool) {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n, err == nil && n > 0
}

func yn(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
