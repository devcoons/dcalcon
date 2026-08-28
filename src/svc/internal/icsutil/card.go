package icsutil

import (
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/emersion/go-vcard"
)

var customNameRE = regexp.MustCompile(`^[A-Z0-9][A-Z0-9-]{0,62}$`)

var managedCardKeys = map[string]bool{
	vcard.FieldFormattedName: true,
	vcard.FieldName:          true,
	vcard.FieldNickname:      true,
	vcard.FieldBirthday:      true,
	vcard.FieldAnniversary:   true,
	vcard.FieldGender:        true,
	vcard.FieldEmail:         true,
	vcard.FieldTelephone:     true,
	vcard.FieldIMPP:          true,
	vcard.FieldURL:           true,
	vcard.FieldAddress:       true,
	vcard.FieldOrganization:  true,
	vcard.FieldTitle:         true,
	vcard.FieldRole:          true,
	vcard.FieldNote:          true,
	vcard.FieldCategories:    true,
	vcard.FieldTimezone:      true,
	vcard.FieldLanguage:      true,
	vcard.FieldKind:          true,
	vcard.FieldVersion:       true,
	vcard.FieldUID:           true,
	vcard.FieldProductID:     true,
	vcard.FieldRevision:      true,
	"X-ANNIVERSARY":          true,
}

type TypedValue struct {
	Value string `json:"value"`
	Type  string `json:"type,omitempty"`
}

type AddressInput struct {
	Type       string `json:"type,omitempty"`
	POBox      string `json:"po_box,omitempty"`
	Extended   string `json:"extended,omitempty"`
	Street     string `json:"street,omitempty"`
	City       string `json:"city,omitempty"`
	Region     string `json:"region,omitempty"`
	PostalCode string `json:"postal_code,omitempty"`
	Country    string `json:"country,omitempty"`
}

type CustomField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ContactInput is the dashboard view of a vCard. Unknown X-* properties are
// Custom. PHOTO and similar binary/client fields stay on the stored card.
type ContactInput struct {
	FN             string         `json:"fn"`
	Nickname       string         `json:"nickname,omitempty"`
	GivenName      string         `json:"given_name,omitempty"`
	FamilyName     string         `json:"family_name,omitempty"`
	AdditionalName string         `json:"additional_name,omitempty"`
	Prefix         string         `json:"prefix,omitempty"`
	Suffix         string         `json:"suffix,omitempty"`
	Org            string         `json:"org,omitempty"`
	Title          string         `json:"title,omitempty"`
	Role           string         `json:"role,omitempty"`
	BDAY           string         `json:"bday,omitempty"`
	Anniversary    string         `json:"anniversary,omitempty"`
	Gender         string         `json:"gender,omitempty"`
	Note           string         `json:"note,omitempty"`
	Categories     string         `json:"categories,omitempty"`
	Kind           string         `json:"kind,omitempty"`
	Lang           string         `json:"lang,omitempty"`
	TZ             string         `json:"tz,omitempty"`
	Email          string         `json:"email,omitempty"`
	Tel            string         `json:"tel,omitempty"`
	Emails         []TypedValue   `json:"emails,omitempty"`
	Tels           []TypedValue   `json:"tels,omitempty"`
	URLs           []TypedValue   `json:"urls,omitempty"`
	IMPPs          []TypedValue   `json:"impps,omitempty"`
	Addresses      []AddressInput `json:"addresses,omitempty"`
	Custom         []CustomField  `json:"custom,omitempty"`
}

func (in *ContactInput) Normalize() {
	in.FN = strings.TrimSpace(in.FN)
	in.Nickname = strings.TrimSpace(in.Nickname)
	in.GivenName = strings.TrimSpace(in.GivenName)
	in.FamilyName = strings.TrimSpace(in.FamilyName)
	in.AdditionalName = strings.TrimSpace(in.AdditionalName)
	in.Prefix = strings.TrimSpace(in.Prefix)
	in.Suffix = strings.TrimSpace(in.Suffix)
	in.Org = strings.TrimSpace(in.Org)
	in.Title = strings.TrimSpace(in.Title)
	in.Role = strings.TrimSpace(in.Role)
	in.BDAY = strings.TrimSpace(in.BDAY)
	in.Anniversary = strings.TrimSpace(in.Anniversary)
	in.Gender = strings.ToUpper(strings.TrimSpace(in.Gender))
	in.Note = strings.TrimSpace(in.Note)
	in.Categories = strings.TrimSpace(in.Categories)
	in.Kind = strings.ToLower(strings.TrimSpace(in.Kind))
	in.Lang = strings.TrimSpace(in.Lang)
	in.TZ = strings.TrimSpace(in.TZ)
	in.Email = strings.TrimSpace(in.Email)
	in.Tel = strings.TrimSpace(in.Tel)
	if len(in.Emails) == 0 && in.Email != "" {
		in.Emails = []TypedValue{{Value: in.Email}}
	}
	if len(in.Tels) == 0 && in.Tel != "" {
		in.Tels = []TypedValue{{Value: in.Tel}}
	}
	in.Emails = cleanTyped(in.Emails)
	in.Tels = cleanTyped(in.Tels)
	in.URLs = cleanTyped(in.URLs)
	in.IMPPs = cleanTyped(in.IMPPs)
	in.Addresses = cleanAddresses(in.Addresses)
	in.Custom = cleanCustom(in.Custom)
	sort.Slice(in.Custom, func(i, j int) bool { return in.Custom[i].Name < in.Custom[j].Name })
	if len(in.Emails) > 0 {
		in.Email = in.Emails[0].Value
	} else {
		in.Email = ""
	}
	if len(in.Tels) > 0 {
		in.Tel = in.Tels[0].Value
	} else {
		in.Tel = ""
	}
	if in.FN == "" {
		in.FN = strings.TrimSpace(strings.Join([]string{in.GivenName, in.FamilyName}, " "))
	}
	if in.FN == "" {
		in.FN = in.Nickname
	}
}

func (in ContactInput) NameOK() bool {
	return strings.TrimSpace(in.FN) != ""
}

func ParseContact(raw string) ContactInput {
	in := ContactInput{}
	card, err := ParseCard(raw)
	if err != nil || card == nil {
		in.Email = VCardEmail(raw)
		in.Tel = VCardTel(raw)
		in.Normalize()
		return in
	}
	in.FN = CardFN(card)
	in.Nickname = card.PreferredValue(vcard.FieldNickname)
	if n := card.Name(); n != nil {
		in.FamilyName = n.FamilyName
		in.GivenName = n.GivenName
		in.AdditionalName = n.AdditionalName
		in.Prefix = n.HonorificPrefix
		in.Suffix = n.HonorificSuffix
	}
	in.Org = card.PreferredValue(vcard.FieldOrganization)
	in.Title = card.PreferredValue(vcard.FieldTitle)
	in.Role = card.PreferredValue(vcard.FieldRole)
	in.BDAY = CardBDAY(card)
	in.Anniversary = CardAnniversary(card)
	sex, _ := card.Gender()
	in.Gender = string(sex)
	in.Note = card.PreferredValue(vcard.FieldNote)
	if cats := card.Categories(); len(cats) > 0 && !(len(cats) == 1 && cats[0] == "") {
		in.Categories = strings.Join(cats, ", ")
	}
	if k := card.Value(vcard.FieldKind); k != "" {
		in.Kind = k
	}
	in.Lang = card.PreferredValue(vcard.FieldLanguage)
	in.TZ = card.PreferredValue(vcard.FieldTimezone)
	in.Emails = typedFromCard(card, vcard.FieldEmail)
	in.Tels = typedFromCard(card, vcard.FieldTelephone)
	in.URLs = typedFromCard(card, vcard.FieldURL)
	in.IMPPs = typedFromCard(card, vcard.FieldIMPP)
	for _, a := range card.Addresses() {
		if a == nil {
			continue
		}
		in.Addresses = append(in.Addresses, AddressInput{
			Type:       firstType(a.Field),
			POBox:      a.PostOfficeBox,
			Extended:   a.ExtendedAddress,
			Street:     a.StreetAddress,
			City:       a.Locality,
			Region:     a.Region,
			PostalCode: a.PostalCode,
			Country:    a.Country,
		})
	}
	in.Custom = customFromCard(card)
	in.Normalize()
	return in
}

func EncodeContact(uid string, in ContactInput, existingRaw string) (string, error) {
	customProvided := in.Custom != nil
	in.Normalize()
	if in.FN == "" {
		in.FN = "Unnamed"
	}
	var card vcard.Card
	var existing vcard.Card
	if parsed, err := ParseCard(existingRaw); err == nil && parsed != nil {
		existing = parsed
		card = clonePreserved(parsed)
	} else {
		card = vcard.Card{}
	}
	card.SetValue(vcard.FieldVersion, "3.0")
	if uid != "" {
		card.SetValue(vcard.FieldUID, uid)
	}
	card.SetValue(vcard.FieldProductID, "-//dCalCon//EN")
	card.SetValue(vcard.FieldFormattedName, in.FN)
	if in.GivenName != "" || in.FamilyName != "" || in.AdditionalName != "" || in.Prefix != "" || in.Suffix != "" {
		card.SetName(&vcard.Name{
			FamilyName:      in.FamilyName,
			GivenName:       in.GivenName,
			AdditionalName:  in.AdditionalName,
			HonorificPrefix: in.Prefix,
			HonorificSuffix: in.Suffix,
		})
	} else {
		parts := strings.Fields(in.FN)
		family, given := "", in.FN
		if len(parts) > 1 {
			family = parts[len(parts)-1]
			given = strings.Join(parts[:len(parts)-1], " ")
		}
		card.SetName(&vcard.Name{FamilyName: family, GivenName: given})
	}
	setCardValue(card, vcard.FieldNickname, in.Nickname)
	setCardValue(card, vcard.FieldOrganization, in.Org)
	setCardValue(card, vcard.FieldTitle, in.Title)
	setCardValue(card, vcard.FieldRole, in.Role)
	setCardValue(card, vcard.FieldBirthday, in.BDAY)
	setCardValue(card, vcard.FieldAnniversary, in.Anniversary)
	delete(card, "X-ANNIVERSARY")
	switch in.Gender {
	case "F", "M", "O", "N", "U":
		card.SetGender(vcard.Sex(in.Gender), "")
	default:
		delete(card, vcard.FieldGender)
	}
	setCardValue(card, vcard.FieldNote, in.Note)
	if in.Categories != "" {
		parts := splitCSV(in.Categories)
		card.SetCategories(parts)
	} else {
		delete(card, vcard.FieldCategories)
	}
	if in.Kind == "org" || in.Kind == "group" || in.Kind == "location" {
		card.SetKind(vcard.Kind(in.Kind))
	} else {
		delete(card, vcard.FieldKind)
	}
	setCardValue(card, vcard.FieldLanguage, in.Lang)
	setCardValue(card, vcard.FieldTimezone, in.TZ)
	setTyped(card, vcard.FieldEmail, in.Emails)
	setTyped(card, vcard.FieldTelephone, in.Tels)
	setTyped(card, vcard.FieldURL, in.URLs)
	setTyped(card, vcard.FieldIMPP, in.IMPPs)
	delete(card, vcard.FieldAddress)
	for _, a := range in.Addresses {
		addr := &vcard.Address{
			PostOfficeBox:   a.POBox,
			ExtendedAddress: a.Extended,
			StreetAddress:   a.Street,
			Locality:        a.City,
			Region:          a.Region,
			PostalCode:      a.PostalCode,
			Country:         a.Country,
		}
		if t := strings.TrimSpace(a.Type); t != "" {
			addr.Field = &vcard.Field{Params: vcard.Params{vcard.ParamType: []string{t}}}
		}
		card.AddAddress(addr)
	}
	if customProvided {
		applyCustomFields(card, existing, in.Custom)
	}
	return EncodeCard(card)
}

func UpdateCard(raw, fn, email, tel, bday, anniversary string) (string, error) {
	in := ContactInput{FN: fn, Email: email, Tel: tel, BDAY: bday, Anniversary: anniversary}
	uid := ""
	if card, err := ParseCard(raw); err == nil && card != nil {
		uid = CardUID(card)
	}
	return EncodeContact(uid, in, raw)
}

func CustomPropName(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	s = strings.ReplaceAll(s, " ", "-")
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			b.WriteRune(r)
		}
	}
	s = b.String()
	if s == "" {
		return "", false
	}
	if !strings.HasPrefix(s, "X-") {
		s = "X-" + s
	}
	inner := strings.TrimPrefix(s, "X-")
	if !customNameRE.MatchString(s) || managedCardKeys[s] || managedCardKeys[inner] {
		return "", false
	}
	return s, true
}

func setCardValue(card vcard.Card, name, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		delete(card, name)
		return
	}
	card.SetValue(name, value)
}

func cleanTyped(in []TypedValue) []TypedValue {
	out := make([]TypedValue, 0, len(in))
	for _, v := range in {
		v.Value = strings.TrimSpace(v.Value)
		v.Type = strings.ToLower(strings.TrimSpace(v.Type))
		if v.Value == "" {
			continue
		}
		out = append(out, v)
		if len(out) >= 20 {
			break
		}
	}
	return out
}

func cleanAddresses(in []AddressInput) []AddressInput {
	out := make([]AddressInput, 0, len(in))
	for _, a := range in {
		a.Type = strings.ToLower(strings.TrimSpace(a.Type))
		a.POBox = strings.TrimSpace(a.POBox)
		a.Extended = strings.TrimSpace(a.Extended)
		a.Street = strings.TrimSpace(a.Street)
		a.City = strings.TrimSpace(a.City)
		a.Region = strings.TrimSpace(a.Region)
		a.PostalCode = strings.TrimSpace(a.PostalCode)
		a.Country = strings.TrimSpace(a.Country)
		if a.Street == "" && a.City == "" && a.Region == "" && a.PostalCode == "" && a.Country == "" && a.POBox == "" && a.Extended == "" {
			continue
		}
		out = append(out, a)
		if len(out) >= 10 {
			break
		}
	}
	return out
}

func cleanCustom(in []CustomField) []CustomField {
	if in == nil {
		return nil
	}
	out := make([]CustomField, 0, len(in))
	seen := map[string]bool{}
	for _, c := range in {
		name, ok := CustomPropName(c.Name)
		val := strings.TrimSpace(c.Value)
		if !ok || val == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, CustomField{Name: name, Value: val})
		if len(out) >= 30 {
			break
		}
	}
	return out
}

func typedFromCard(card vcard.Card, key string) []TypedValue {
	fields := card[key]
	out := make([]TypedValue, 0, len(fields))
	for _, f := range fields {
		if f == nil || strings.TrimSpace(f.Value) == "" {
			continue
		}
		out = append(out, TypedValue{Value: f.Value, Type: firstType(f)})
	}
	return out
}

func firstType(f *vcard.Field) string {
	if f == nil {
		return ""
	}
	for _, t := range f.Params.Types() {
		switch t {
		case "pref", "internet", "voice":
			continue
		default:
			return t
		}
	}
	return ""
}

func customFromCard(card vcard.Card) []CustomField {
	var out []CustomField
	for k, fields := range card {
		ku := strings.ToUpper(k)
		if !strings.HasPrefix(ku, "X-") || managedCardKeys[ku] {
			continue
		}
		for _, f := range fields {
			if f == nil || strings.TrimSpace(f.Value) == "" {
				continue
			}
			out = append(out, CustomField{Name: ku, Value: f.Value})
		}
	}
	return out
}

func setTyped(card vcard.Card, key string, values []TypedValue) {
	delete(card, key)
	for _, v := range values {
		f := &vcard.Field{Value: v.Value}
		if t := strings.TrimSpace(v.Type); t != "" {
			f.Params = vcard.Params{vcard.ParamType: []string{t}}
		}
		card.Add(key, f)
	}
}

func cloneField(f *vcard.Field) *vcard.Field {
	if f == nil {
		return nil
	}
	out := &vcard.Field{Value: f.Value, Group: f.Group}
	if len(f.Params) > 0 {
		out.Params = make(vcard.Params, len(f.Params))
		for k, vs := range f.Params {
			out.Params[k] = append([]string(nil), vs...)
		}
	}
	return out
}

func clonePreserved(src vcard.Card) vcard.Card {
	dst := vcard.Card{}
	for k, fields := range src {
		if managedCardKeys[strings.ToUpper(k)] {
			continue
		}
		copied := make([]*vcard.Field, 0, len(fields))
		for _, f := range fields {
			copied = append(copied, cloneField(f))
		}
		dst[k] = copied
	}
	return dst
}

func cardFields(card vcard.Card, name string) []*vcard.Field {
	if card == nil {
		return nil
	}
	if f, ok := card[name]; ok {
		return f
	}
	for k, f := range card {
		if strings.EqualFold(k, name) {
			return f
		}
	}
	return nil
}

func visibleCustomKeys(card vcard.Card) []string {
	if card == nil {
		return nil
	}
	var keys []string
	for k, fields := range card {
		ku := strings.ToUpper(k)
		if !strings.HasPrefix(ku, "X-") || managedCardKeys[ku] {
			continue
		}
		if _, ok := CustomPropName(ku); !ok {
			continue
		}
		okVal := false
		for _, f := range fields {
			if f != nil && strings.TrimSpace(f.Value) != "" {
				okVal = true
				break
			}
		}
		if okVal {
			keys = append(keys, k)
		}
	}
	return keys
}

func applyCustomFields(card, existing vcard.Card, custom []CustomField) {
	for _, k := range visibleCustomKeys(existing) {
		delete(card, k)
	}
	used := map[string]map[int]bool{}
	for _, c := range custom {
		name, ok := CustomPropName(c.Name)
		val := strings.TrimSpace(c.Value)
		if !ok || val == "" {
			continue
		}
		if f := takeMatchingCustom(existing, name, val, used); f != nil {
			card.Add(name, f)
			continue
		}
		card.AddValue(name, val)
	}
}

func takeMatchingCustom(existing vcard.Card, name, val string, used map[string]map[int]bool) *vcard.Field {
	fields := cardFields(existing, name)
	if len(fields) == 0 {
		return nil
	}
	seen := used[name]
	if seen == nil {
		seen = map[int]bool{}
		used[name] = seen
	}
	for i, f := range fields {
		if f == nil || seen[i] || f.Value != val {
			continue
		}
		seen[i] = true
		return cloneField(f)
	}
	return nil
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
