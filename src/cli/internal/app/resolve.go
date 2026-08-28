package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/devcoons/dcalcon/cli/internal/client"
)

func (e *Env) calendars() ([]client.Calendar, error) {
	raw, err := e.cli.Get("/api/v1/calendars", nil)
	if err != nil {
		return nil, err
	}
	return decodeList[client.Calendar](raw)
}

func (e *Env) calendarRef(ref string) (*client.Calendar, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("calendar is required")
	}
	list, err := e.calendars()
	if err != nil {
		return nil, err
	}
	if id, ok := parseID(ref); ok {
		for i := range list {
			if list[i].ID == id {
				return &list[i], nil
			}
		}
	}
	low := strings.ToLower(ref)
	var slug, name *client.Calendar
	for i := range list {
		if strings.EqualFold(list[i].Slug, ref) {
			c := list[i]
			slug = &c
		}
		if strings.ToLower(list[i].Name) == low {
			c := list[i]
			name = &c
		}
	}
	if slug != nil {
		return slug, nil
	}
	if name != nil {
		return name, nil
	}
	return nil, fmt.Errorf("calendar %q not found", ref)
}

func (e *Env) books() ([]client.AddressBook, error) {
	raw, err := e.cli.Get("/api/v1/addressbooks", nil)
	if err != nil {
		return nil, err
	}
	return decodeList[client.AddressBook](raw)
}

func (e *Env) bookRef(ref string) (*client.AddressBook, error) {
	list, err := e.books()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(ref) == "" {
		for i := range list {
			if list[i].Slug == "contacts" && !list[i].ReadOnly {
				return &list[i], nil
			}
		}
		if len(list) == 1 {
			return &list[0], nil
		}
		return nil, fmt.Errorf("address book is required")
	}
	if id, ok := parseID(ref); ok {
		for i := range list {
			if list[i].ID == id {
				return &list[i], nil
			}
		}
	}
	low := strings.ToLower(ref)
	for i := range list {
		if strings.EqualFold(list[i].Slug, ref) || strings.ToLower(list[i].Name) == low {
			return &list[i], nil
		}
	}
	return nil, fmt.Errorf("address book %q not found", ref)
}

func (e *Env) userRef(ref string) (*client.User, error) {
	raw, err := e.cli.Get("/api/v1/admin/users", nil)
	if err != nil {
		return nil, err
	}
	list, err := decodeList[client.User](raw)
	if err != nil {
		return nil, err
	}
	if id, ok := parseID(ref); ok {
		for i := range list {
			if list[i].ID == id {
				return &list[i], nil
			}
		}
	}
	for i := range list {
		if strings.EqualFold(list[i].Username, ref) {
			return &list[i], nil
		}
	}
	return nil, fmt.Errorf("user %q not found", ref)
}

func (e *Env) shareUserID(calID int64, ref string) (int64, error) {
	raw, err := e.cli.Get("/api/v1/calendars/"+client.Itoa(calID)+"/shares", nil)
	if err != nil {
		return 0, err
	}
	list, err := decodeList[client.CalendarShare](raw)
	if err != nil {
		return 0, err
	}
	if id, ok := parseID(ref); ok {
		for _, s := range list {
			if s.UserID == id {
				return s.UserID, nil
			}
		}
	}
	for _, s := range list {
		if strings.EqualFold(s.Username, ref) {
			return s.UserID, nil
		}
	}
	return 0, fmt.Errorf("share for %q not found", ref)
}

func formatInt(n int64) string {
	return strconv.FormatInt(n, 10)
}
