package providers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Endpoints struct {
	Auth     string
	Token    string
	Userinfo string
	Send     string
}

var Google = Endpoints{
	Auth:     "https://accounts.google.com/o/oauth2/v2/auth",
	Token:    "https://oauth2.googleapis.com/token",
	Userinfo: "https://openidconnect.googleapis.com/v1/userinfo",
	Send:     "https://gmail.googleapis.com/gmail/v1/users/me/messages/send",
}

var Microsoft = Endpoints{
	Auth:     "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
	Token:    "https://login.microsoftonline.com/common/oauth2/v2.0/token",
	Userinfo: "https://graph.microsoft.com/v1.0/me",
	Send:     "https://graph.microsoft.com/v1.0/me/sendMail",
}

func MicrosoftWithTenant(tenant string) Endpoints {
	e := Microsoft
	tenant = strings.TrimSpace(tenant)
	if tenant == "" || strings.EqualFold(tenant, "common") {
		return e
	}
	base := "https://login.microsoftonline.com/" + url.PathEscape(tenant) + "/oauth2/v2.0"
	e.Auth = base + "/authorize"
	e.Token = base + "/token"
	return e
}

type Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	Expiry       string `json:"expiry"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	Host         string `json:"host,omitempty"`
	Port         int    `json:"port,omitempty"`
	Username     string `json:"username,omitempty"`
	Password     string `json:"password,omitempty"`
	From         string `json:"from,omitempty"`
}

func (t *Token) Expired() bool {
	if t.Expiry == "" {
		return false
	}
	tm, err := time.Parse(time.RFC3339, t.Expiry)
	if err != nil {
		return true
	}
	return time.Now().Add(2 * time.Minute).After(tm)
}

func (t *Token) applyExpiresIn() {
	if t.ExpiresIn > 0 {
		t.Expiry = time.Now().Add(time.Duration(t.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	}
	t.ExpiresIn = 0
}

type OAuthApp struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Scopes       []string
	Endpoints    Endpoints
	HTTP         *http.Client
	Offline      bool
	Prompt       string
}

func GoogleScopes() []string {
	return []string{"openid", "email", "https://www.googleapis.com/auth/gmail.send"}
}

func MicrosoftScopes() []string {
	return []string{"offline_access", "openid", "email", "User.Read", "Mail.Send"}
}

func NewVerifier() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func (a OAuthApp) AuthURL(state, challenge string, offline bool) string {
	q := url.Values{}
	q.Set("client_id", a.ClientID)
	q.Set("redirect_uri", a.RedirectURI)
	q.Set("response_type", "code")
	q.Set("scope", strings.Join(a.Scopes, " "))
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	if offline || a.Offline {
		q.Set("access_type", "offline")
	}
	prompt := a.Prompt
	if prompt == "" && (offline || a.Offline) {
		prompt = "consent"
	}
	if prompt != "" {
		q.Set("prompt", prompt)
	}
	return a.Endpoints.Auth + "?" + q.Encode()
}

func (a OAuthApp) client() *http.Client {
	if a.HTTP != nil {
		return a.HTTP
	}
	return http.DefaultClient
}

func (a OAuthApp) Exchange(ctx context.Context, code, verifier string) (*Token, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", a.RedirectURI)
	form.Set("client_id", a.ClientID)
	form.Set("client_secret", a.ClientSecret)
	form.Set("code_verifier", verifier)
	return a.token(ctx, form)
}

func (a OAuthApp) Refresh(ctx context.Context, refresh string) (*Token, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refresh)
	form.Set("client_id", a.ClientID)
	form.Set("client_secret", a.ClientSecret)
	tok, err := a.token(ctx, form)
	if err != nil {
		return nil, err
	}
	if tok.RefreshToken == "" {
		tok.RefreshToken = refresh
	}
	return tok, nil
}

func (a OAuthApp) token(ctx context.Context, form url.Values) (*Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.Endpoints.Token, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := a.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("token endpoint %d: %s", res.StatusCode, truncate(raw))
	}
	tok := &Token{}
	if err := json.Unmarshal(raw, tok); err != nil {
		return nil, err
	}
	if tok.AccessToken == "" {
		return nil, fmt.Errorf("token endpoint returned no access_token")
	}
	tok.applyExpiresIn()
	return tok, nil
}

func (a OAuthApp) Email(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.Endpoints.Userinfo, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	res, err := a.client().Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return "", fmt.Errorf("userinfo %d: %s", res.StatusCode, truncate(raw))
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return "", err
	}
	for _, k := range []string{"email", "mail", "userPrincipalName"} {
		if s, ok := body[k].(string); ok && strings.Contains(s, "@") {
			return s, nil
		}
	}
	return "", fmt.Errorf("userinfo had no email")
}

func (a OAuthApp) SendGmail(ctx context.Context, accessToken string, rfc822 []byte) error {
	payload, err := json.Marshal(map[string]string{
		"raw": base64.RawURLEncoding.EncodeToString(rfc822),
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.Endpoints.Send, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	res, err := a.client().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		raw, _ := io.ReadAll(res.Body)
		return fmt.Errorf("gmail send %d: %s", res.StatusCode, truncate(raw))
	}
	return nil
}

func (a OAuthApp) SendGraph(ctx context.Context, accessToken string, rfc822 []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.Endpoints.Send, strings.NewReader(base64.StdEncoding.EncodeToString(rfc822)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "text/plain")
	res, err := a.client().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		raw, _ := io.ReadAll(res.Body)
		return fmt.Errorf("graph send %d: %s", res.StatusCode, truncate(raw))
	}
	return nil
}

func SendSMTP(ctx context.Context, host string, port int, username, password, from, to string, rfc822 []byte) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
	}
	if port == 0 {
		port = 587
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	d := &net.Dialer{Timeout: 15 * time.Second}
	raw, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = raw.SetDeadline(dl)
	}
	conn := net.Conn(raw)
	if port == 465 {
		tlsConn := tls.Client(raw, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = raw.Close()
			return err
		}
		conn = tlsConn
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer c.Close()
	if port != 465 {
		ok, _ := c.Extension("STARTTLS")
		if !ok {
			return fmt.Errorf("smtp server does not offer STARTTLS")
		}
		if err := c.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}
	if username != "" {
		if err := c.Auth(smtp.PlainAuth("", username, password, host)); err != nil {
			return err
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(rfc822); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func truncate(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 240 {
		return s[:240]
	}
	return s
}

func NewState() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
