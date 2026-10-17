package service

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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

// lan is a peer on the network, not this machine.
const lan = "192.0.2.1:1234"

// request is a GET of / from a peer on the network (lan) with Host host and the headers (name, value pairs).
func request(host string, headers ...string) *http.Request {
	return from(lan, host, headers...)
}

// from is request from the TCP peer peer.
func from(peer, host string, headers ...string) *http.Request {
	r := httptest.NewRequest("GET", "http://127.0.0.1/", nil)
	r.RemoteAddr = peer
	r.Host = host
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Set(headers[i], headers[i+1])
	}
	return r
}

// accessServer is a server with access = "cloudflare" once site_url is set, as pm init writes it; without a site_url
// no access, which only a loopback listen allows.
func accessServer(siteURL, accessTeam, accessAud string) *server {
	access := ""
	if siteURL != "" {
		access = "cloudflare"
	}
	return &server{d: Deps{SiteURL: siteURL, Access: access, AccessTeam: accessTeam, AccessAud: accessAud, Port: 8000}}
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
		{"HS256", k.sign(nil, map[string]any{"alg": "HS256"}, nil), refused + "its algorithm is \"HS256\", not RS256"},
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
		{"unknown kid", k.sign(nil, map[string]any{"kid": "k9"}, nil), refused + "the team's keys hold no key with kid \"k9\""},
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
		if err := s.access.verify(unknown, team, aud, at.Add(c.after)); err == nil || !strings.Contains(err.Error(), "no key with kid \"k2\"") {
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

func TestALoopbackPeerNamingThisMachineIsLocalAndAnEdgeForwardedRequestNeedsTheTokenWhateverItsHost(t *testing.T) {
	k := newTeamKeys(t)
	for _, s := range []*server{accessServer(site, team, aud), accessServer("", "", ""), accessServer(site, "", "")} {
		for _, peer := range []string{"127.0.0.1:50000", "[::1]:50000", "127.0.0.2:1"} {
			for _, host := range []string{"127.0.0.1:8000", "localhost", "LocalHost:1", "[::1]:8000"} {
				if got := s.refusal(from(peer, host)); got != "" {
					t.Errorf("site %q, peer %s, Host %s: %q", s.d.SiteURL, peer, host, got)
				}
			}
		}
		// a rebinding domain and no Host, from this machine (a browser a rebinding domain points here) or the network;
		// and a peer on the network claiming this machine's name: none is local
		for _, peer := range []string{"127.0.0.1:50000", lan} {
			for _, host := range []string{"evil.example:80", ""} {
				want := "refused: Host " + map[bool]string{true: "None", false: `"` + host + `"`}[host == ""] +
					" is neither this machine nor the configured site_url; open the site at "
				if got := s.refusal(from(peer, host, AccessHeader, k.sign(nil, nil, nil))); !strings.HasPrefix(got, want) {
					t.Errorf("site %q, peer %s, Host %q: got %q, want %q…", s.d.SiteURL, peer, host, got, want)
				}
			}
		}
		for _, host := range []string{"localhost", "127.0.0.1:8000", "LocalHost:1"} {
			want := "refused: Host \"" + host + "\" is neither this machine nor the configured site_url; open the site at "
			if got := s.refusal(request(host)); !strings.HasPrefix(got, want) {
				t.Errorf("site %q, a LAN peer with Host %q: got %q, want %q…", s.d.SiteURL, host, got, want)
			}
		}
	}
	if got := accessServer("", "", "").refusal(request("pm.example.com")); !strings.Contains(got, "is neither this machine") {
		t.Errorf("site_url's host with no site_url set: %q", got)
	}
	s := accessServer(site, team, aud)
	for _, peer := range []string{"127.0.0.1:50000", lan} { // cloudflared connects from loopback
		for _, host := range []string{"localhost", "127.0.0.1:8000", "evil.example"} { // a tunnel that rewrites Host
			if got := s.refusal(from(peer, host, EdgeHeader, "203.0.113.9")); !strings.HasPrefix(got, "refused: the request carries no Cloudflare Access token") {
				t.Errorf("edge-forwarded from %s, Host %s, no token: %q", peer, host, got)
			}
			if got := s.refusal(from(peer, host, EdgeHeader, "203.0.113.9", AccessHeader, k.sign(nil, nil, nil))); got != "" {
				t.Errorf("edge-forwarded from %s, Host %s, a valid token: %q", peer, host, got)
			}
		}
	}
}

// With access = "none" the network is the gate: a peer on it reads site_url's host with no token, but its Host must
// still be site_url's (the rebinding guard), it is never local (so read-only: run_test.go has the reply), and a
// request Cloudflare's edge forwarded, which came from the internet, is refused.
func TestAccessNoneLetsTheNetworkReadSiteURLsHostOnly(t *testing.T) {
	s := &server{d: Deps{SiteURL: "http://devbox:8123", Listen: "0.0.0.0", Access: "none", Port: 8123}}
	for _, host := range []string{"devbox:8123", "DEVBOX", "devbox"} {
		if got := s.refusal(request(host)); got != "" {
			t.Errorf("access none, LAN peer, Host %s: %q", host, got)
		}
		if !s.readOnly(request(host)) {
			t.Errorf("access none, LAN peer, Host %s: may reply", host)
		}
	}
	for _, host := range []string{"localhost", "127.0.0.1:8123", "evil.example", ""} {
		if got := s.refusal(request(host)); !strings.Contains(got, "is neither this machine nor the configured site_url; open the site at http://devbox:8123") {
			t.Errorf("access none, LAN peer, Host %q: %q", host, got)
		}
	}
	for _, peer := range []string{lan, "127.0.0.1:50000"} {
		if got := s.refusal(from(peer, "devbox:8123", EdgeHeader, "203.0.113.9")); !strings.Contains(got, `Cloudflare's edge forwarded this request, and access = "none"`) {
			t.Errorf("access none, edge-forwarded from %s: %q", peer, got)
		}
	}
	if got := s.refusal(from("127.0.0.1:50000", "localhost")); got != "" || s.readOnly(from("127.0.0.1:50000", "localhost")) {
		t.Errorf("access none, loopback peer: refusal %q, read-only %v", got, s.readOnly(from("127.0.0.1:50000", "localhost")))
	}
	// this machine's browser on site_url's name is not local either: it reads, and replies from localhost
	if got := s.refusal(from("127.0.0.1:50000", "devbox:8123")); got != "" || !s.readOnly(from("127.0.0.1:50000", "devbox:8123")) {
		t.Errorf("access none, loopback peer on site_url's host: refusal %q", got)
	}
	if accessServer(site, team, aud).readOnly(request("pm.example.com")) {
		t.Error("access cloudflare: a viewer through Access is read-only")
	}
}

func TestTheServiceChecksAccessOnEveryRequestAndARefusalNamesNoStore(t *testing.T) {
	k := newTeamKeys(t)
	w := newFakeWork(need("p-1.2.1", work.Decision, ""))
	s := serve(t, w, "", func(d *Deps) { d.SiteURL, d.Access, d.AccessTeam, d.AccessAud = site, "cloudflare", team, aud })
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

// Under access = "none" a viewer on the network reads the pages, with the rule in place of each reply form and no
// token, and its reply is refused naming the rule; this machine's viewer replies as before. /version digests the page
// each viewer is served, so neither sees a newer page that is not there.
func TestUnderAccessNoneANetworkViewerReadsWithNoReplyFormAndItsReplyIsRefused(t *testing.T) {
	w := newFakeWork(need("p-1.2.1", work.Decision, ""))
	main := shortTemp(t)
	for _, d := range []string{".pm/store/records", ".git"} {
		if err := os.MkdirAll(filepath.Join(main, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	s, err := newServer(Deps{Main: main, Records: store(main), Remote: "origin", MainBranch: "main",
		SiteURL: "http://devbox:8123", Listen: "0.0.0.0", Access: "none", Port: 8123,
		Spool: filepath.Join(main, ".git", SpoolName), WorkDir: main, Open: w.Open, Mark: w.Mark, Site: &fakeSite{},
		Out: io.Discard, Log: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	defer s.stop()
	s.setSnap(s.refresh(nil))
	do := func(peer, method, path, host string, body url.Values) (int, string) {
		t.Helper()
		var rd io.Reader
		if body != nil {
			rd = strings.NewReader(body.Encode())
		}
		r := httptest.NewRequest(method, "http://"+host+path, rd)
		r.RemoteAddr, r.Host = peer, host
		if body != nil {
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, r)
		return rec.Code, rec.Body.String()
	}
	statusRE := regexp.MustCompile(`\[status (\w+)\]`)
	for _, c := range []struct {
		peer, host, slot string
	}{
		{lan, "devbox:8123", "[read-only p-1.2.1: " + ReadOnlyRule + "]"},
		{"127.0.0.1:50000", "devbox:8123", "[read-only p-1.2.1: " + ReadOnlyRule + "]"}, // this machine, on the network's name
		{"127.0.0.1:50000", "localhost:8123", "[form p-1.2.1 token=" + s.token + " "},
	} {
		code, body := do(c.peer, "GET", "/", c.host, nil)
		if code != 200 || !strings.Contains(body, c.slot) || (c.host != "localhost:8123" && strings.Contains(body, s.token)) {
			t.Fatalf("GET / from %s Host %s: %d %q, want %q", c.peer, c.host, code, body, c.slot)
		}
		code, version := do(c.peer, "GET", "/version?page=/", c.host, nil)
		if m := statusRE.FindStringSubmatch(body); code != 200 || m == nil || !strings.Contains(version, `"page":"`+m[1]+`"`) {
			t.Errorf("/version from %s Host %s: %d %q, page %q", c.peer, c.host, code, version, body)
		}
	}
	form := url.Values{"token": {s.token}, "id": {"p-1.2.1"}, "text": {"yes"}}
	if code, body := do(lan, "POST", "/reply", "devbox:8123", form); code != 403 || body != "refused: "+ReadOnlyRule+"\n" {
		t.Errorf("a reply from the network: %d %q", code, body)
	}
	if code, body := do("127.0.0.1:50000", "POST", "/reply", "localhost:8123", form); code != 303 {
		t.Errorf("a reply from this machine: %d %q", code, body)
	}
}

// The service does not start when the site is exposed (a non-loopback listen, or a site_url) and access is unset,
// naming the key.
func TestTheServiceRefusesToStartExposedWithNoAccess(t *testing.T) {
	for _, set := range []func(*Deps){
		func(d *Deps) { d.Listen = "0.0.0.0" },
		func(d *Deps) { d.SiteURL = site },
	} {
		d := Deps{Main: "m", Records: "r", Spool: "s", Pin: "p", WorkDir: "w", Listen: "127.0.0.1", Open: newFakeWork().Open,
			Mark: func() (string, error) { return "", nil }, Sync: func(context.Context) ([]string, error) { return nil, nil },
			GC: func(context.Context) error { return nil }, Site: &fakeSite{}, Summarize: func() (bool, string) { return true, "" },
			Out: io.Discard, Log: io.Discard}
		set(&d)
		err := Run(d)
		if err == nil || !strings.Contains(err.Error(), "sets no access, so the pm service does not start: set access = \"cloudflare\"") {
			t.Errorf("listen %s site_url %q: %v", d.Listen, d.SiteURL, err)
		}
	}
}

// Binds is the listen address, and 127.0.0.1 beside a specific other one, where this machine's probes reach it.
func TestBindsAddLoopbackBesideASpecificAddress(t *testing.T) {
	for listen, want := range map[string][]string{"127.0.0.1": {"127.0.0.1"}, "0.0.0.0": {"0.0.0.0"}, "::": {"::"},
		"192.168.1.5": {"192.168.1.5", "127.0.0.1"}, "::1": {"::1", "127.0.0.1"}} {
		if got := Binds(listen); !slices.Equal(got, want) {
			t.Errorf("Binds(%s) = %v, want %v", listen, got, want)
		}
	}
	held, err := net.Listen("tcp", "0.0.0.0:0") // the wildcard held: 127.0.0.1 at its port is not free either
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := held.Close(); err != nil {
			t.Error(err)
		}
	})
	addr, ok := held.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("%v is not a TCP address", held.Addr())
	}
	port := addr.Port
	for _, listen := range []string{"0.0.0.0", "127.0.0.1"} {
		if PortFree(listen, port) {
			t.Errorf("listen %s: port %d free while 0.0.0.0 holds it", listen, port)
		}
	}
}
