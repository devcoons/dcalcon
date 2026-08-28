package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/devcoons/dcalcon/internal/auth"
	"github.com/devcoons/dcalcon/internal/httpx"
	"github.com/devcoons/dcalcon/internal/icsutil"
	"github.com/devcoons/dcalcon/internal/imip"
	"github.com/devcoons/dcalcon/internal/metrics"
	"github.com/devcoons/dcalcon/internal/providers"
	"github.com/devcoons/dcalcon/internal/schedule"
	"github.com/devcoons/dcalcon/internal/secret"
	"github.com/devcoons/dcalcon/internal/storage"
	"github.com/go-chi/chi/v5"
)

type inviteResult struct {
	Local     int      `json:"local"`
	Email     int      `json:"email"`
	Missing   []string `json:"missing,omitempty"`
	MailError string   `json:"mail_error,omitempty"`
}

type mailStatusDTO struct {
	GoogleConfigured      bool   `json:"google_configured"`
	MicrosoftConfigured   bool   `json:"microsoft_configured"`
	ServerSMTP            bool   `json:"server_smtp"`
	TokenKey              bool   `json:"token_key"`
	GoogleCallbackPath    string `json:"google_callback_path"`
	MicrosoftCallbackPath string `json:"microsoft_callback_path"`
}

func (h *Handler) httpClient() *http.Client {
	if h.HTTP != nil {
		return h.HTTP
	}
	return http.DefaultClient
}

func (h *Handler) mailStatus(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, mailStatusDTO{
		GoogleConfigured:      strings.TrimSpace(h.Cfg.OAuth.GoogleClientID) != "",
		MicrosoftConfigured:   strings.TrimSpace(h.Cfg.OAuth.MicrosoftClientID) != "",
		ServerSMTP:            h.Mailer != nil && h.Mailer.Configured(),
		TokenKey:              strings.TrimSpace(h.Cfg.Auth.TokenKey) != "",
		GoogleCallbackPath:    "/api/v1/oauth/google/callback",
		MicrosoftCallbackPath: "/api/v1/oauth/microsoft/callback",
	})
}

func (h *Handler) listAccounts(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	list, err := h.Store.ListConnectedAccounts(r.Context(), p.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []storage.ConnectedAccount{}
	}
	httpx.JSON(w, http.StatusOK, list)
}

func (h *Handler) connectAccount(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	var body struct {
		Provider string `json:"provider"`
		Origin   string `json:"origin"`
		Email    string `json:"email"`
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
		From     string `json:"from"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	switch strings.ToLower(strings.TrimSpace(body.Provider)) {
	case "google", "microsoft":
		url, err := h.startOAuth(r.Context(), p.ID, strings.ToLower(body.Provider), body.Origin)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]string{"authorize_url": url})
	case "smtp":
		acc, err := h.saveSMTPAccount(r.Context(), p.ID, body.Email, body.Host, body.Port, body.Username, body.Password, body.From)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.JSON(w, http.StatusCreated, acc)
	default:
		httpx.Error(w, http.StatusBadRequest, "provider must be google, microsoft, or smtp")
	}
}

func (h *Handler) deleteAccount(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusNotFound, "account")
		return
	}
	if err := h.Store.DeleteConnectedAccount(r.Context(), p.ID, id); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "account")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) testAccount(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusNotFound, "account")
		return
	}
	acc, err := h.Store.ConnectedAccountByID(r.Context(), p.ID, id)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "account")
		return
	}
	msg := imip.Plain(acc.Email, acc.Email, "dCalCon mail test", "This is a test message from dCalCon. Your connected account can send mail.")
	if err := h.sendRFC822(r.Context(), p.ID, acc, acc.Email, msg); err != nil {
		_ = h.Store.SetConnectedAccountError(r.Context(), p.ID, acc.ID, err.Error())
		slog.Error("mail test send", "err", err, "provider", acc.Provider)
		httpx.Error(w, http.StatusBadGateway, "could not send test message")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func (h *Handler) oauthCallback(w http.ResponseWriter, r *http.Request) {
	provider := strings.ToLower(chi.URLParam(r, "provider"))
	q := r.URL.Query()
	state := q.Get("state")
	origin := h.publicURL()
	if origin == "" {
		origin = "http://localhost:3000"
	}
	st, err := h.Store.TakeOAuthState(r.Context(), state)
	if err == nil && st.Origin != "" {
		origin = st.Origin
	}
	fail := func(detail string) {
		http.Redirect(w, r, origin+"/app/settings?mail=err&detail="+url.QueryEscape(publicOAuthError(detail)), http.StatusFound)
	}
	if err != nil {
		fail("invalid or expired OAuth state")
		return
	}
	if provider != st.Provider {
		fail("OAuth provider mismatch")
		return
	}
	if msg := q.Get("error"); msg != "" {
		fail(msg)
		return
	}
	code := q.Get("code")
	if code == "" {
		fail("missing authorization code")
		return
	}
	app, err := h.oauthApp(st.Provider, st.Origin)
	if err != nil {
		fail(err.Error())
		return
	}
	tok, err := app.Exchange(r.Context(), code, st.CodeVerifier)
	if err != nil {
		fail(err.Error())
		return
	}
	email, err := app.Email(r.Context(), tok.AccessToken)
	if err != nil {
		fail(err.Error())
		return
	}
	key, err := secret.Key(h.Cfg.Auth.TokenKey)
	if err != nil {
		fail("token encryption key is not set")
		return
	}
	raw, err := json.Marshal(tok)
	if err != nil {
		fail(err.Error())
		return
	}
	ct, nonce, err := secret.Seal(key, raw)
	if err != nil {
		fail(err.Error())
		return
	}
	if _, err := h.Store.UpsertConnectedAccount(r.Context(), st.UserID, st.Provider, email, "connected", strings.Join(app.Scopes, " "), ct, nonce); err != nil {
		fail(err.Error())
		return
	}
	http.Redirect(w, r, origin+"/app/settings?mail=ok", http.StatusFound)
}

func (h *Handler) startOAuth(ctx context.Context, userID int64, provider, origin string) (string, error) {
	origin, err := h.normalizeOrigin(origin)
	if err != nil {
		return "", err
	}
	app, err := h.oauthApp(provider, origin)
	if err != nil {
		return "", err
	}
	if _, err := secret.Key(h.Cfg.Auth.TokenKey); err != nil {
		return "", fmt.Errorf("set DCALCON_TOKEN_KEY (or let the server create a key next to the database) before connecting an account")
	}
	verifier, challenge, err := providers.NewVerifier()
	if err != nil {
		return "", err
	}
	state, err := providers.NewState()
	if err != nil {
		return "", err
	}
	if err := h.Store.PutOAuthState(ctx, storage.OAuthState{
		State:        state,
		UserID:       userID,
		Provider:     provider,
		Origin:       origin,
		CodeVerifier: verifier,
		ExpiresAt:    time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339),
	}); err != nil {
		return "", err
	}
	return app.AuthURL(state, challenge, true), nil
}

func (h *Handler) oauthApp(provider, origin string) (providers.OAuthApp, error) {
	redirect := strings.TrimRight(origin, "/") + "/api/v1/oauth/" + provider + "/callback"
	switch provider {
	case "google":
		if strings.TrimSpace(h.Cfg.OAuth.GoogleClientID) == "" {
			return providers.OAuthApp{}, fmt.Errorf("Google OAuth is not configured (GOOGLE_OAUTH_CLIENT_ID)")
		}
		return providers.OAuthApp{
			ClientID:     h.Cfg.OAuth.GoogleClientID,
			ClientSecret: h.Cfg.OAuth.GoogleClientSecret,
			RedirectURI:  redirect,
			Scopes:       providers.GoogleScopes(),
			Endpoints:    providers.Google,
			HTTP:         h.httpClient(),
			Offline:      true,
			Prompt:       "consent",
		}, nil
	case "microsoft":
		if strings.TrimSpace(h.Cfg.OAuth.MicrosoftClientID) == "" {
			return providers.OAuthApp{}, fmt.Errorf("Microsoft OAuth is not configured (MICROSOFT_OAUTH_CLIENT_ID)")
		}
		return providers.OAuthApp{
			ClientID:     h.Cfg.OAuth.MicrosoftClientID,
			ClientSecret: h.Cfg.OAuth.MicrosoftClientSecret,
			RedirectURI:  redirect,
			Scopes:       providers.MicrosoftScopes(),
			Endpoints:    providers.MicrosoftWithTenant(h.Cfg.OAuth.MicrosoftTenant),
			HTTP:         h.httpClient(),
			Prompt:       "consent",
		}, nil
	default:
		return providers.OAuthApp{}, fmt.Errorf("unknown provider")
	}
}

func (h *Handler) saveSMTPAccount(ctx context.Context, userID int64, email, host string, port int, username, password, from string) (*storage.ConnectedAccount, error) {
	key, err := secret.Key(h.Cfg.Auth.TokenKey)
	if err != nil {
		return nil, fmt.Errorf("set DCALCON_TOKEN_KEY (or let the server create a key next to the database) before connecting an account")
	}
	addr, err := parseInviteEmail(email)
	if err != nil {
		return nil, fmt.Errorf("a valid from-address email is required")
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, fmt.Errorf("SMTP host is required")
	}
	if port == 0 {
		port = 587
	}
	if from == "" {
		from = addr
	}
	if username == "" {
		username = addr
	}
	if password == "" {
		return nil, fmt.Errorf("SMTP password is required")
	}
	tok := providers.Token{Host: host, Port: port, Username: username, Password: password, From: from}
	raw, err := json.Marshal(tok)
	if err != nil {
		return nil, err
	}
	ct, nonce, err := secret.Seal(key, raw)
	if err != nil {
		return nil, err
	}
	return h.Store.UpsertConnectedAccount(ctx, userID, "smtp", addr, "connected", "smtp", ct, nonce)
}

func (h *Handler) normalizeOrigin(origin string) (string, error) {
	origin = strings.TrimRight(strings.TrimSpace(origin), "/")
	if origin == "" {
		origin = h.publicURL()
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("invalid origin")
	}
	if u.Path != "" && u.Path != "/" {
		return "", fmt.Errorf("invalid origin")
	}
	if !h.originAllowed(origin) {
		return "", fmt.Errorf("origin is not allowed for OAuth redirects")
	}
	return origin, nil
}

func (h *Handler) originAllowed(origin string) bool {
	allowed := []string{
		h.publicURL(),
		"http://localhost:3000",
		"http://127.0.0.1:3000",
		"http://localhost:8080",
		"http://127.0.0.1:8080",
	}
	for _, a := range allowed {
		if strings.EqualFold(origin, a) {
			return true
		}
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "localhost" || host == "127.0.0.1" {
		return true
	}
	if pub, err := url.Parse(h.Cfg.HTTP.PublicURL); err == nil && strings.EqualFold(host, pub.Hostname()) {
		return true
	}
	return false
}

func parseInviteEmail(s string) (string, error) {
	s = strings.TrimSpace(s)
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address == "" {
		return "", fmt.Errorf("invalid email")
	}
	return a.Address, nil
}

func publicOAuthError(msg string) string {
	l := strings.ToLower(strings.TrimSpace(msg))
	switch {
	case l == "" || strings.Contains(l, "access_denied"):
		return "Connection was cancelled."
	case strings.Contains(l, "invalid or expired"):
		return "That sign-in expired. Connect the account again."
	case strings.Contains(l, "token encryption") || strings.Contains(l, "token key"):
		return "Server token encryption is not ready."
	case strings.Contains(l, "token endpoint") || strings.Contains(l, "userinfo") || strings.Contains(l, "gmail send") || strings.Contains(l, "graph send"):
		return "The provider rejected the connection. Check the OAuth client and redirect URI."
	}
	msg = strings.Join(strings.Fields(msg), " ")
	if len(msg) > 140 {
		return "Could not connect the email account."
	}
	return msg
}

func (h *Handler) maybeInvite(r *http.Request, p auth.Principal, c *storage.Calendar, obj *storage.CalendarObject, usernames, emails []string) *inviteResult {
	if len(usernames) == 0 && len(emails) == 0 {
		return nil
	}
	inv, err := h.sendInvites(r, p, c, obj, usernames, emails)
	if err != nil {
		inv.MailError = err.Error()
	}
	return &inv
}

func (h *Handler) sendInvites(r *http.Request, p auth.Principal, c *storage.Calendar, obj *storage.CalendarObject, usernames, emails []string) (inviteResult, error) {
	var out inviteResult
	org, err := h.Store.UserByID(r.Context(), p.ID)
	if err != nil {
		return out, err
	}
	seen := map[string]bool{}
	var local []*storage.User
	var external []string
	addIdent := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		key := strings.ToLower(raw)
		if seen[key] || strings.EqualFold(raw, p.Username) {
			return
		}
		if u, err := schedule.FindUser(r.Context(), h.Store, raw); err == nil {
			if u.ID == p.ID || u.Status != "active" {
				return
			}
			lk := strings.ToLower(u.Username)
			if seen[lk] {
				return
			}
			seen[lk] = true
			seen[strings.ToLower(u.Email)] = true
			seen[strings.ToLower(schedule.LocalMailbox(u.Username))] = true
			local = append(local, u)
			return
		}
		if schedule.IsLocalMailbox(raw) {
			out.Missing = append(out.Missing, raw)
			return
		}
		if addr, err := parseInviteEmail(raw); err == nil {
			if strings.EqualFold(addr, org.Email) {
				return
			}
			ak := strings.ToLower(addr)
			if seen[ak] {
				return
			}
			seen[ak] = true
			external = append(external, addr)
			return
		}
		out.Missing = append(out.Missing, raw)
	}
	for _, n := range usernames {
		addIdent(n)
	}
	for _, n := range emails {
		addIdent(n)
	}
	if len(local) == 0 && len(external) == 0 {
		if len(out.Missing) == 0 {
			return out, fmt.Errorf("no invitees")
		}
		return out, fmt.Errorf("no matching local users: %s", strings.Join(out.Missing, ", "))
	}
	if len(local) > 0 {
		if err := schedule.ApplyLocalInvite(r.Context(), h.Store, org, c, obj, local); err != nil {
			return out, err
		}
		out.Local = len(local)
	}
	if len(external) > 0 {
		if err := schedule.MergeExternalEmails(r.Context(), h.Store, org, c, obj, external); err != nil {
			return out, err
		}
		sent, err := h.sendIMIP(r.Context(), org, obj, external)
		out.Email = sent
		if err != nil {
			out.MailError = err.Error()
		}
	}
	return out, nil
}

func (h *Handler) sendIMIP(ctx context.Context, org *storage.User, obj *storage.CalendarObject, emails []string) (int, error) {
	ics, err := icsutil.WithMethod(obj.ICS, "REQUEST")
	if err != nil {
		ics = obj.ICS
	}
	cn := org.DisplayName
	if cn == "" {
		cn = org.Username
	}
	when := strings.TrimSpace(obj.DTStart)
	if obj.DTEnd != "" {
		when = strings.TrimSpace(obj.DTStart + " – " + obj.DTEnd)
	}
	loc := icsutil.LocationFromICS(obj.ICS)
	subject := "Invitation: " + obj.Summary
	body := imip.InviteBody(cn, obj.Summary, when, loc)

	accs, err := h.Store.ListConnectedAccounts(ctx, org.ID)
	if err != nil {
		return 0, err
	}
	var acc *storage.ConnectedAccount
	if id := pickAccountID(accs, org.Email); id != 0 {
		acc, err = h.Store.ConnectedAccountByID(ctx, org.ID, id)
		if err != nil {
			acc = nil
		}
	}

	sent := 0
	var last error
	for _, to := range emails {
		if schedule.IsLocalMailbox(to) {
			continue
		}
		if acc != nil {
			msg := imip.Build(acc.Email, to, subject, body, ics, "")
			if err := h.sendRFC822(ctx, org.ID, acc, to, msg); err != nil {
				_ = h.Store.SetConnectedAccountError(ctx, org.ID, acc.ID, err.Error())
				metrics.IncIMIPError()
				last = err
				continue
			}
			metrics.IncIMIPSent()
			sent++
			continue
		}
		if h.Mailer != nil && h.Mailer.Configured() {
			from := h.Mailer.FromAddress()
			if from == "" {
				from = org.Email
			}
			msg := imip.Build(from, to, subject, body, ics, org.Email)
			if err := h.Mailer.SendMIME(ctx, to, msg); err != nil {
				metrics.IncIMIPError()
				last = err
				continue
			}
			metrics.IncIMIPSent()
			sent++
			continue
		}
		return sent, fmt.Errorf("connect an email account in Settings to send invitations")
	}
	return sent, last
}

func pickAccountID(accs []storage.ConnectedAccount, organizerEmail string) int64 {
	var match, google, ms, smtp int64
	for _, a := range accs {
		if a.Status != "connected" && a.Status != "error" {
			continue
		}
		if strings.EqualFold(a.Email, organizerEmail) {
			match = a.ID
		}
		switch a.Provider {
		case "google":
			if google == 0 {
				google = a.ID
			}
		case "microsoft":
			if ms == 0 {
				ms = a.ID
			}
		case "smtp":
			if smtp == 0 {
				smtp = a.ID
			}
		}
	}
	if match != 0 {
		return match
	}
	if google != 0 {
		return google
	}
	if ms != 0 {
		return ms
	}
	return smtp
}

func (h *Handler) sendRFC822(ctx context.Context, userID int64, acc *storage.ConnectedAccount, to string, rfc822 []byte) error {
	tok, err := h.openAccountToken(acc)
	if err != nil {
		return err
	}
	switch acc.Provider {
	case "smtp":
		from := tok.From
		if from == "" {
			from = acc.Email
		}
		return providers.SendSMTP(ctx, tok.Host, tok.Port, tok.Username, tok.Password, from, to, rfc822)
	case "google", "microsoft":
		tok, err = h.ensureAccess(ctx, userID, acc, tok)
		if err != nil {
			return err
		}
		app, err := h.oauthApp(acc.Provider, h.publicURL())
		if err != nil {
			return err
		}
		if acc.Provider == "google" {
			return app.SendGmail(ctx, tok.AccessToken, rfc822)
		}
		return app.SendGraph(ctx, tok.AccessToken, rfc822)
	default:
		return fmt.Errorf("unknown provider %s", acc.Provider)
	}
}

func (h *Handler) openAccountToken(acc *storage.ConnectedAccount) (*providers.Token, error) {
	key, err := secret.Key(h.Cfg.Auth.TokenKey)
	if err != nil {
		return nil, err
	}
	plain, err := secret.Open(key, acc.Cipher, acc.Nonce)
	if err != nil {
		return nil, fmt.Errorf("could not decrypt account tokens")
	}
	tok := &providers.Token{}
	if err := json.Unmarshal(plain, tok); err != nil {
		return nil, err
	}
	return tok, nil
}

func (h *Handler) ensureAccess(ctx context.Context, userID int64, acc *storage.ConnectedAccount, tok *providers.Token) (*providers.Token, error) {
	if tok.AccessToken != "" && !tok.Expired() {
		return tok, nil
	}
	if tok.RefreshToken == "" {
		return nil, fmt.Errorf("account %s needs to be reconnected", acc.Email)
	}
	app, err := h.oauthApp(acc.Provider, h.publicURL())
	if err != nil {
		return nil, err
	}
	next, err := app.Refresh(ctx, tok.RefreshToken)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(next)
	if err != nil {
		return nil, err
	}
	key, err := secret.Key(h.Cfg.Auth.TokenKey)
	if err != nil {
		return nil, err
	}
	ct, nonce, err := secret.Seal(key, raw)
	if err != nil {
		return nil, err
	}
	if err := h.Store.SaveConnectedTokens(ctx, userID, acc.ID, ct, nonce); err != nil {
		return nil, err
	}
	acc.Cipher, acc.Nonce = ct, nonce
	return next, nil
}
