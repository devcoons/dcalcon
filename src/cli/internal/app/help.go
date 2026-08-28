package app

import (
	"fmt"
	"strings"
)

func (e *Env) help(args []string) error {
	topic := ""
	if len(args) > 1 {
		topic = args[1]
	} else if len(args) == 1 && args[0] != "help" && args[0] != "-h" && args[0] != "--help" {
		topic = args[0]
	}
	if topic != "" {
		if t := helpTopic(topic); t != "" {
			fmt.Fprint(e.Stdout, t)
			return nil
		}
	}
	fmt.Fprint(e.Stdout, usageRoot)
	return nil
}

const usageRoot = `dcalcon-cli — command line for the dCalCon dashboard API

Usage:
  dcalcon-cli [global flags] <command> [flags]

Global flags:
  --url URL       API origin (or DCALCON_URL)
  --config PATH   config file (or DCALCON_CLI_CONFIG)
  --json          print JSON instead of a table

Sign in:
  login [--user NAME] [--password PASS] [--totp CODE]
  logout
  whoami
  recover --email ADDR
  reset --token TOKEN --password PASS
  reset --totp --user NAME --code CODE --password PASS

Home:
  overview
  setup
  directory

Calendars:
  calendar list
  calendar create --name NAME [--color HEX] [--desc TEXT]
  calendar update CAL --name NAME [--color HEX] [--desc TEXT]
  calendar delete CAL
  calendar shares CAL
  calendar share CAL --user NAME [--access read|write]
  calendar unshare CAL --user NAME
  calendar export CAL [--out FILE]
  calendar import CAL --file FILE.ics
  calendar webcal CAL [show|enable|disable]

Events and tasks:
  event list CAL
  event get CAL HREF
  event create CAL --summary TEXT --start TIME [--end TIME] [--location TEXT] [--desc TEXT] [--all-day]
                    [--rrule RULE] [--alarm MINUTES] [--invite USER] [--email ADDR]
  event update CAL HREF --summary TEXT --start TIME [--end TIME] [--location TEXT] [--desc TEXT] [--all-day]
                    [--rrule RULE] [--alarm MINUTES] [--invite USER] [--email ADDR]
  event delete CAL HREF
  event invite CAL HREF [--user NAME] [--email ADDR]
  task list [CAL]
  task get CAL HREF
  task create CAL --summary TEXT [--due TIME] [--status STATUS] [--desc TEXT]
  task update CAL HREF --summary TEXT ...
  task delete CAL HREF

Files (attachments):
  file list CAL --event HREF | --task HREF
  file add CAL --event HREF|--task HREF --file PATH
  file get CAL ID [--out PATH]
  file delete CAL --event HREF|--task HREF ID

Contacts:
  contact books
  contact list [BOOK]
  contact get BOOK HREF
  contact create [BOOK] --fn NAME [--email ADDR] [--tel TEL] [--org ORG] [--title TEXT] [--note TEXT]
                 [--bday YYYY-MM-DD] [--anniversary YYYY-MM-DD]
  contact update BOOK HREF --fn NAME ...
  contact delete BOOK HREF
  contact export [BOOK] [--out FILE]
  contact import BOOK --file FILE.vcf

Invitations:
  invitation list
  invitation accept ID [--calendar CAL]
  invitation decline ID
  freebusy --users a,b [--start TIME] [--end TIME]

Account:
  me show | me update --name NAME --email ADDR --tz TZ
  me password [--current PASS] [--new PASS]
  me export [--out FILE]
  me backup data [--out FILE]
  me backup full [--out FILE] [--password PASS]
  me restore --file FILE [--password PASS]
  me revoke-sessions
  totp setup | totp enable --code CODE | totp disable [--password PASS] [--code CODE] | totp cancel
  app-password list | app-password create --name NAME | app-password delete ID

Settings:
  dates show
  dates set [--on|--off] [--birthdays|--no-birthdays] [--anniversaries|--no-anniversaries]
            [--alarm-day|--no-alarm-day] [--alarm-week|--no-alarm-week]
  mail
  account list
  account connect smtp --email ADDR --host HOST [--port N] [--username U] [--password PASS]
  account connect google|microsoft [--origin URL]
  account test ID
  account disconnect ID

Admin:
  user list
  user create --user NAME --email ADDR --password PASS [--name NAME] [--role user|admin] [--tz TZ]
  user update USER [--email ADDR] [--name NAME] [--role R] [--status active|disabled] [--tz TZ]
  user password USER --password PASS
  user recovery USER
  user disable-totp USER
  audit
  outbox

CAL, BOOK, and USER can be a numeric id, a slug, or a name.
Config is stored at ~/.config/dcalcon/cli.json (mode 0600).
`

func helpTopic(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	switch name {
	case "calendar", "calendars":
		return "calendar list|create|update|delete|shares|share|unshare|export|import|webcal\n"
	case "event", "events":
		return "event list|get|create|update|delete|invite\n"
	case "task", "tasks":
		return "task list|get|create|update|delete\n"
	case "contact", "contacts":
		return "contact books|list|get|create|update|delete|export|import\n"
	case "login", "auth":
		return "login [--user] [--password] [--totp]\nlogout\nwhoami\n"
	default:
		return ""
	}
}
