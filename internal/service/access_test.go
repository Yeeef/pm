package service

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	gosync "sync"
	"testing"
	"time"

	"github.com/Yeeef/pm/internal/work"
)

const (
	team = "team.cloudflareaccess.com"
	aud  = "aud-tag"
	site = "https://pm.example.com"
)

// teamKeys serves a JWKS of one RSA key (kid k1) as the team's certs, in place of the network, counting the fetches;
// sign makes a token with claims over valid ones.
type teamKeys struct {
	t       *testing.T
	key     *rsa.PrivateKey
	mu      gosync.Mutex
	fetches int
	body    func() (int, string) // what the certs URL answers; nil: the JWKS
}

func newTeamKeys(t *testing.T) *teamKeys {
	t.Helper()
	k := &teamKeys{t: t, key: rsaKey(t)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		k.mu.Lock()
		k.fetches++
		body := k.body
		k.mu.Unlock()
		if body != nil {
			code, text := body()
			w.WriteHeader(code)
			_, _ = io.WriteString(w, text)
			return
		}
		pub := k.key.PublicKey
		jwk := map[string]string{"kid": "k1", "kty": "RSA", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())}
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{jwk}})
	}))
	t.Cleanup(srv.Close)
	saved := CertsURL
	CertsURL = func(got string) string {
		if got != team {
			t.Errorf("certs fetched for team %q, not %q", got, team)
		}
		return srv.URL + "/cdn-cgi/access/certs"
	}
	t.Cleanup(func() { CertsURL = saved })
	return k
}

func rsaKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func (k *teamKeys) count() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.fetches
}

// sign is a token: header {alg: RS256, kid: k1} and the valid claims, each overridden by head and claims (a nil value
// drops the claim), signed by key (nil: the team's).
func (k *teamKeys) sign(key *rsa.PrivateKey, head, claims map[string]any) string {
	h := map[string]any{"alg": "RS256", "typ": "JWT", "kid": "k1"}
	c := map[string]any{"aud": []string{aud}, "iss": "https://" + team, "exp": time.Now().Unix() + 60,
		"iat": time.Now().Unix() - 5, "sub": "owner", "type": "app"}
	for dst, src := range map[*map[string]any]map[string]any{&h: head, &c: claims} {
		for name, v := range src {
			if v == nil {
				delete(*dst, name)
			} else {
				(*dst)[name] = v
			}
		}
	}
	part := func(v any) string {
		b, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	signed := part(h) + "." + part(c)
	if key == nil {
		key = k.key
	}
	sum := sha256.Sum256([]byte(signed))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		k.t.Fatal(err)
	}
	return signed + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// request is a GET of / with Host host and the headers (name, value pairs).
func request(host string, headers ...string) *http.Request {
	r := httptest.NewRequest("GET", "http://127.0.0.1/", nil)
	r.Host = host
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Set(headers[i], headers[i+1])
	}
	return r
}

func accessServer(siteURL, accessTeam, accessAud string) *server {
	return &server{d: Deps{SiteURL: siteURL, AccessTeam: accessTeam, AccessAud: accessAud, Port: 8000}}
}

func TestThePublicHostNeedsACloudflareAccessTokenThatVerifies(t *testing.T) {
	k := newTeamKeys(t)
	s := accessServer(site, team, aud)
	now := time.Now().Unix()
	refused := "refused: the Cloudflare Access token does not verify: "
	cases := []struct {
		name, token, said string
	}{
		{"no token", "", "refused: the request carries no Cloudflare Access token (Cf-Access-Jwt-Assertion); open the site at https://pm.example.com"},
		{"not a JWT", "not-a-jwt", refused + "not a JWT"},
		{"another key", k.sign(rsaKey(t), nil, nil), refused + "signature verification failed"},
		{"claims changed after signing", func() string {
			p := strings.Split(k.sign(nil, nil, nil), ".")
			other := strings.Split(k.sign(nil, nil, map[string]any{"sub": "someone else"}), ".")
			return p[0] + "." + other[1] + "." + p[2]
		}(), refused + "signature verification failed"},
		{"HS256", k.sign(nil, map[string]any{"alg": "HS256"}, nil), refused + "its algorithm is 'HS256', not RS256"},
		{"wrong aud", k.sign(nil, nil, map[string]any{"aud": []string{"other"}}), refused + "its audience is not this Access application's"},
		{"no aud", k.sign(nil, nil, map[string]any{"aud": nil}), refused + "it has no aud claim"},
		{"wrong iss", k.sign(nil, nil, map[string]any{"iss": "https://evil.cloudflareaccess.com"}), refused + "its issuer is not the configured Access team"},
		{"no iss", k.sign(nil, nil, map[string]any{"iss": nil}), refused + "its issuer is not the configured Access team"},
		{"expired", k.sign(nil, nil, map[string]any{"exp": now - 60}), refused + "the token has expired"},
		{"no exp", k.sign(nil, nil, map[string]any{"exp": nil}), refused + "it has no numeric exp claim"},
		{"nbf ahead", k.sign(nil, nil, map[string]any{"nbf": now + 600}), refused + "the token is not yet valid (nbf)"},
		{"iat ahead", k.sign(nil, nil, map[string]any{"iat": now + 600}), refused + "the token is not yet valid (iat)"},
		{"meta token", k.sign(nil, nil, map[string]any{"type": "meta", "iss": nil}), refused + "it is not an Access application token (type app)"},
		{"org token", k.sign(nil, nil, map[string]any{"type": "org"}), refused + "it is not an Access application token (type app)"},
		{"no type", k.sign(nil, nil, map[string]any{"type": nil}), refused + "it is not an Access application token (type app)"},
		{"unknown kid", k.sign(nil, map[string]any{"kid": "k9"}, nil), refused + "the team's keys hold no key with kid 'k9'"},
	}
	for _, c := range cases {
		if got := s.refusal(request("pm.example.com:443", AccessHeader, c.token)); got != c.said {
			t.Errorf("%s: got %q, want %q", c.name, got, c.said)
		}
	}
	for _, host := range []string{"pm.example.com", "PM.Example.com:443"} {
		if got := s.refusal(request(host, AccessHeader, k.sign(nil, nil, map[string]any{"aud": aud}))); got != "" {
			t.Errorf("a valid token on %s: %q", host, got)
		}
	}
	// the keys are fetched once and kept; a kid they lack fetches them again, at most once per refetchEvery
	before := k.count()
	s.refusal(request("pm.example.com", AccessHeader, k.sign(nil, nil, nil)))
	if got := k.count(); got != before {
		t.Errorf("a known kid fetched the keys again: %d fetches, then %d", before, got)
	}
	unknown := k.sign(nil, map[string]any{"kid": "k2"}, nil)
	at := s.access.fetched
	for _, c := range []struct {
		after time.Duration
		want  int
	}{{refetchEvery - time.Second, before}, {refetchEvery, before + 1}, {refetchEvery + time.Second, before + 1}} {
		if err := s.access.verify(unknown, team, aud, at.Add(c.after)); err == nil || !strings.Contains(err.Error(), "no key with kid 'k2'") {
			t.Errorf("an unknown kid %s after the fetch: %v", c.after, err)
		}
		if got := k.count(); got != c.want {
			t.Errorf("an unknown kid %s after the last fetch: %d fetches, want %d", c.after, got, c.want)
		}
	}
}

func TestThePublicHostIsRefusedWhenTheTeamKeysCannotBeFetched(t *testing.T) {
	k := newTeamKeys(t)
	for _, c := range []struct {
		code       int
		body, said string
	}{
		{500, "down", "the team's keys could not be fetched: "},
		{200, "<html>Sign in</html>", "the team's keys are not a JSON key set: "},
	} {
		k.mu.Lock()
		k.body = func() (int, string) { return c.code, c.body }
		k.mu.Unlock()
		s := accessServer(site, team, aud) // no keys cached
		got := s.refusal(request("pm.example.com", AccessHeader, k.sign(nil, nil, nil)))
		if !strings.HasPrefix(got, "refused: the Cloudflare Access token does not verify: "+c.said) {
			t.Errorf("certs answering %d %q: %q", c.code, c.body, got)
		}
	}
	saved := CertsURL
	CertsURL = func(string) string { return "http://127.0.0.1:1/certs" } // nothing listens
	defer func() { CertsURL = saved }()
	got := accessServer(site, team, aud).refusal(request("pm.example.com", AccessHeader, k.sign(nil, nil, nil)))
	if !strings.HasPrefix(got, "refused: the Cloudflare Access token does not verify: the team's keys could not be fetched: ") {
		t.Errorf("certs unreachable: %q", got)
	}
}

func TestWithoutTheAccessKeysEveryPublicRequestIsRefusedNamingThem(t *testing.T) {
	newTeamKeys(t)
	team1 := "access_team (the Cloudflare Access team domain, <team>.cloudflareaccess.com)"
	aud1 := "access_aud (the Access application's AUD tag)"
	tail := ", so no request through the tunnel is verified; set what is missing there (pm init --help) and restart the pm service"
	for _, c := range []struct {
		site, team, aud string
		r               *http.Request
		said            string
	}{
		{site, "", aud, request("pm.example.com"), "refused: site_url is set, but .pm/config.toml lacks " + team1 + tail},
		{site, team, "", request("pm.example.com"), "refused: site_url is set, but .pm/config.toml lacks " + aud1 + tail},
		{site, "", "", request("pm.example.com", AccessHeader, "x"), "refused: site_url is set, but .pm/config.toml lacks " + team1 + " and " + aud1 + tail},
		// no site_url, but Cloudflare's edge forwarded it: the tunnel exists, so it is refused all the same
		{"", "", "", request("localhost:8000", EdgeHeader, "203.0.113.9"), "refused: .pm/config.toml lacks " + team1 + " and " + aud1 + tail},
	} {
		if got := accessServer(c.site, c.team, c.aud).refusal(c.r); got != c.said {
			t.Errorf("site %q team %q aud %q: got %q, want %q", c.site, c.team, c.aud, got, c.said)
		}
	}
}

func TestLocalhostIsServedAsBeforeAndAnEdgeForwardedRequestNeedsTheTokenWhateverItsHost(t *testing.T) {
	k := newTeamKeys(t)
	for _, s := range []*server{accessServer(site, team, aud), accessServer("", "", ""), accessServer(site, "", "")} {
		for _, host := range []string{"127.0.0.1:8000", "localhost", "LocalHost:1"} {
			if got := s.refusal(request(host)); got != "" {
				t.Errorf("site %q, Host %s: %q", s.d.SiteURL, host, got)
			}
		}
		for _, host := range []string{"evil.example:80", ""} { // a rebinding domain, and no Host at all
			shown := "None"
			if host != "" {
				shown = "'" + host + "'"
			}
			want := "refused: Host " + shown + " is neither this machine nor the configured site_url; open the site at "
			if got := s.refusal(request(host, AccessHeader, k.sign(nil, nil, nil))); !strings.HasPrefix(got, want) {
				t.Errorf("site %q, Host %q: got %q, want %q…", s.d.SiteURL, host, got, want)
			}
		}
	}
	if got := accessServer("", "", "").refusal(request("pm.example.com")); !strings.Contains(got, "is neither this machine") {
		t.Errorf("site_url's host with no site_url set: %q", got)
	}
	s := accessServer(site, team, aud)
	for _, host := range []string{"localhost", "127.0.0.1:8000", "evil.example"} { // a tunnel that rewrites Host
		if got := s.refusal(request(host, EdgeHeader, "203.0.113.9")); !strings.HasPrefix(got, "refused: the request carries no Cloudflare Access token") {
			t.Errorf("edge-forwarded, Host %s, no token: %q", host, got)
		}
		if got := s.refusal(request(host, EdgeHeader, "203.0.113.9", AccessHeader, k.sign(nil, nil, nil))); got != "" {
			t.Errorf("edge-forwarded, Host %s, a valid token: %q", host, got)
		}
	}
}

func TestTheServiceChecksAccessOnEveryRequestAndARefusalNamesNoStore(t *testing.T) {
	k := newTeamKeys(t)
	w := newFakeWork(need("p-1.2.1", work.Decision, ""))
	s := serve(t, w, "", func(d *Deps) { d.SiteURL, d.AccessTeam, d.AccessAud = site, team, aud })
	tok := s.token() // from localhost, served as before
	do := func(method, path, host string, body url.Values, headers ...string) (response, string) {
		t.Helper()
		var rd io.Reader
		if body != nil {
			rd = strings.NewReader(body.Encode())
		}
		req, _ := http.NewRequest(method, s.base+path, rd)
		req.Host = host
		if body != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		for i := 0; i+1 < len(headers); i += 2 {
			req.Header.Set(headers[i], headers[i+1])
		}
		client := http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return response{StatusCode: resp.StatusCode, Header: resp.Header}, string(b)
	}
	form := url.Values{"token": {tok}, "id": {"p-1.2.1"}, "text": {"yes"}}
	for _, c := range []struct {
		method, path, host string
		body               url.Values
		headers            []string
	}{
		{"GET", "/", "pm.example.com", nil, nil},
		{"GET", "/style.css", "pm.example.com:443", nil, nil},
		{"GET", "/version", "pm.example.com", nil, nil},
		{"HEAD", "/", "pm.example.com", nil, nil},
		{"POST", "/reply", "pm.example.com", form, nil},
		{"GET", "/", "localhost", nil, []string{EdgeHeader, "203.0.113.9"}},
		{"POST", "/reply", "127.0.0.1", form, []string{EdgeHeader, "203.0.113.9"}},
		{"GET", "/", "pm.example.com", nil, []string{AccessHeader, k.sign(nil, nil, map[string]any{"exp": time.Now().Unix() - 1})}},
		{"GET", "/", "evil.example", nil, nil},
	} {
		resp, body := do(c.method, c.path, c.host, c.body, c.headers...)
		if resp.StatusCode != 403 || (c.method != "HEAD" && !strings.HasPrefix(body, "refused: ")) {
			t.Errorf("%s %s Host %s %v: %d %q", c.method, c.path, c.host, c.headers, resp.StatusCode, body)
		}
		if resp.Header.Get(StoreHeader) != "" || resp.Header.Get(VersionHeader) != "" || strings.Contains(body, s.main) {
			t.Errorf("%s %s Host %s: a refusal names the store or the build: %v %q", c.method, c.path, c.host, resp.Header, body)
		}
	}
	if len(w.item("p-1.2.1").Comments) != 0 {
		t.Fatal("a refused reply was stored")
	}
	good := k.sign(nil, nil, nil)
	if resp, body := do("GET", "/", "pm.example.com", nil, AccessHeader, good); resp.StatusCode != 200 || resp.Header.Get(StoreHeader) == "" {
		t.Fatalf("a valid token: %d %q", resp.StatusCode, body)
	}
	resp, body := do("POST", "/reply", "localhost", form, EdgeHeader, "203.0.113.9", AccessHeader, good)
	if resp.StatusCode != 303 {
		t.Fatalf("a reply with a valid token through a tunnel that rewrote Host: %d %q", resp.StatusCode, body)
	}
	eventually(t, "stored", func() bool { return len(w.item("p-1.2.1").Comments) == 1 })
}
