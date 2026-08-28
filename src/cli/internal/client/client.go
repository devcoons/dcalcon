package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

const SessionCookie = "dcalcon_session"
const maxResponseBytes = 96 << 20

type Error struct {
	Status int
	Msg    string
}

func (e *Error) Error() string {
	if e.Msg == "" {
		return fmt.Sprintf("http %d", e.Status)
	}
	return e.Msg
}

type Client struct {
	Base    string
	Session string
	HTTP    *http.Client
}

func New(base, session string) *Client {
	return &Client{
		Base:    strings.TrimRight(strings.TrimSpace(base), "/"),
		Session: strings.TrimSpace(session),
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *Client) Get(p string, q url.Values) ([]byte, error) {
	if q != nil {
		p = p + "?" + q.Encode()
	}
	return c.do(http.MethodGet, p, nil, "")
}

func (c *Client) JSON(method, p string, body any) ([]byte, error) {
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(raw)
	}
	ct := ""
	if body != nil {
		ct = "application/json"
	}
	return c.do(method, p, rdr, ct)
}

func (c *Client) Raw(method, p string, body []byte, contentType string) ([]byte, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	return c.do(method, p, rdr, contentType)
}

func (c *Client) Download(p string) (data []byte, filename string, err error) {
	req, err := c.newRequest(http.MethodGet, p, nil, "")
	if err != nil {
		return nil, "", err
	}
	return c.readDownload(req)
}

func (c *Client) DownloadJSON(method, p string, body any) (data []byte, filename string, err error) {
	var rdr io.Reader
	ct := ""
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, "", err
		}
		rdr = bytes.NewReader(raw)
		ct = "application/json"
	}
	req, err := c.newRequest(method, p, rdr, ct)
	if err != nil {
		return nil, "", err
	}
	return c.readDownload(req)
}

func (c *Client) readDownload(req *http.Request) ([]byte, string, error) {
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	raw, err := readLimited(res.Body)
	if err != nil {
		return nil, "", err
	}
	if res.StatusCode >= 400 {
		return nil, "", apiErr(res.StatusCode, raw)
	}
	return raw, filenameFromDisposition(res.Header.Get("Content-Disposition"), "download"), nil
}

func filenameFromDisposition(d, fallback string) string {
	if i := strings.Index(d, `filename="`); i >= 0 {
		rest := d[i+10:]
		if j := strings.Index(rest, `"`); j >= 0 && rest[:j] != "" {
			return rest[:j]
		}
	}
	return fallback
}

func (c *Client) UploadForm(p, field, filename string, data []byte, fields map[string]string) ([]byte, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile(field, path.Base(filename))
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(data); err != nil {
		return nil, err
	}
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	return c.do(http.MethodPost, p, &buf, mw.FormDataContentType())
}

func (c *Client) Upload(p, field, filename string, data []byte) ([]byte, error) {
	return c.UploadForm(p, field, filename, data, nil)
}

func (c *Client) Login(username, password, totp string) (User, string, error) {
	body := map[string]string{"username": username, "password": password}
	if totp != "" {
		body["totp"] = totp
	}
	raw, hdr, err := c.doHdr(http.MethodPost, "/api/v1/auth/login", mustJSON(body), "application/json")
	if err != nil {
		return User{}, "", err
	}
	var u User
	if err := json.Unmarshal(raw, &u); err != nil {
		return User{}, "", err
	}
	sid := readCookie(hdr, SessionCookie)
	if sid == "" {
		return User{}, "", fmt.Errorf("server did not return a session cookie")
	}
	c.Session = sid
	return u, sid, nil
}

func (c *Client) do(method, p string, body io.Reader, contentType string) ([]byte, error) {
	raw, _, err := c.doHdr(method, p, body, contentType)
	return raw, err
}

func (c *Client) doHdr(method, p string, body io.Reader, contentType string) ([]byte, http.Header, error) {
	req, err := c.newRequest(method, p, body, contentType)
	if err != nil {
		return nil, nil, err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer res.Body.Close()
	raw, err := readLimited(res.Body)
	if err != nil {
		return nil, nil, err
	}
	if res.StatusCode >= 400 {
		return nil, res.Header, apiErr(res.StatusCode, raw)
	}
	return raw, res.Header, nil
}

func (c *Client) newRequest(method, p string, body io.Reader, contentType string) (*http.Request, error) {
	u := c.Base + p
	req, err := http.NewRequest(method, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.Session != "" {
		req.AddCookie(&http.Cookie{Name: SessionCookie, Value: c.Session})
		req.Header.Set("Authorization", "Bearer "+c.Session)
	}
	req.Header.Set("User-Agent", "dcalcon-cli")
	return req, nil
}

func apiErr(status int, raw []byte) error {
	msg := strings.TrimSpace(string(raw))
	var m struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(raw, &m) == nil && m.Error != "" {
		msg = m.Error
	}
	if status == http.StatusUnauthorized {
		if msg == "" || strings.EqualFold(msg, "unauthorized") {
			msg = "not signed in — run: dcalcon-cli login"
		}
	}
	return &Error{Status: status, Msg: msg}
}

func mustJSON(v any) io.Reader {
	raw, _ := json.Marshal(v)
	return bytes.NewReader(raw)
}

func readCookie(h http.Header, name string) string {
	for _, line := range h.Values("Set-Cookie") {
		parts := strings.SplitN(line, ";", 2)
		kv := strings.SplitN(strings.TrimSpace(parts[0]), "=", 2)
		if len(kv) == 2 && kv[0] == name {
			v, err := url.QueryUnescape(kv[1])
			if err != nil {
				return kv[1]
			}
			return v
		}
	}
	return ""
}

func Itoa(n int64) string { return strconv.FormatInt(n, 10) }

func Enc(s string) string { return url.PathEscape(s) }

func readLimited(r io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxResponseBytes {
		return nil, fmt.Errorf("response larger than %d bytes", maxResponseBytes)
	}
	return raw, nil
}
