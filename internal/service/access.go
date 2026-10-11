package service

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	gosync "sync"
	"time"

	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/proc"
)

const (
	AccessHeader = "Cf-Access-Jwt-Assertion" // the token Cloudflare Access adds to each request it lets through
	EdgeHeader   = "Cf-Connecting-IP"        // Cloudflare's edge adds it to every request it forwards; a local client does not
)

// CertsURL is where a Cloudflare Access team publishes its signing keys; a variable so tests serve their own.
var CertsURL = func(team string) string { return "https://" + team + "/cdn-cgi/access/certs" }

// certsClient fetches the team's keys; a certs URL that does not answer refuses the request, never hangs it.
var certsClient = &http.Client{Timeout: 10 * time.Second}

// accessKeys is the team's signing keys by kid, fetched on first use and again when a token names a kid they lack (the
// team rotated its keys). A fetch replaces them whole, so a key the team dropped goes too.
type accessKeys struct {
	mu   gosync.Mutex
	keys map[string]*rsa.PublicKey
}

func (a *accessKeys) key(team, kid string) (*rsa.PublicKey, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if k := a.keys[kid]; k != nil {
		return k, nil
	}
	keys, err := fetchKeys(CertsURL(team))
	if err != nil {
		return nil, err
	}
	a.keys = keys
	if k := keys[kid]; k != nil {
		return k, nil
	}
	return nil, fmt.Errorf("the team's keys hold no key with kid %s", proc.Repr(kid))
}

// fetchKeys reads a JWKS: the team's RSA keys by kid.
func fetchKeys(u string) (map[string]*rsa.PublicKey, error) {
	resp, err := certsClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("the team's keys could not be fetched: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("the team's keys could not be fetched: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("the team's keys could not be fetched: %s answered %d", u, resp.StatusCode)
	}
	var set struct {
		Keys []struct{ Kid, Kty, N, E string }
	}
	if err := json.Unmarshal(body, &set); err != nil {
		return nil, fmt.Errorf("the team's keys are not a JSON key set: %w", err)
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		n, err1 := base64.RawURLEncoding.DecodeString(k.N)
		e, err2 := base64.RawURLEncoding.DecodeString(k.E)
		if err1 != nil || err2 != nil || len(e) == 0 || len(e) > 4 {
			return nil, fmt.Errorf("the team's key %s is not an RSA key", proc.Repr(k.Kid))
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	return keys, nil
}

// verifyAccess checks a Cloudflare Access token: an RS256 JWT signed by one of the team's keys, whose aud holds aud,
// whose iss is https://<team>, that has expired neither by exp nor is used before its nbf or iat.
func (a *accessKeys) verify(token, team, aud string, now time.Time) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return errors.New("not a JWT")
	}
	var head struct{ Alg, Kid string }
	if err := decodePart(parts[0], &head); err != nil {
		return fmt.Errorf("its header %w", err)
	}
	if head.Alg != "RS256" {
		return fmt.Errorf("its algorithm is %s, not RS256", proc.Repr(head.Alg))
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return errors.New("its signature is not base64url")
	}
	key, err := a.key(team, head.Kid)
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], sig) != nil {
		return errors.New("signature verification failed")
	}
	var claims struct {
		Aud           json.RawMessage
		Iss           *string
		Exp, Nbf, Iat *json.Number
	}
	if err := decodePart(parts[1], &claims); err != nil {
		return fmt.Errorf("its claims %w", err)
	}
	var auds []string
	if json.Unmarshal(claims.Aud, &auds) != nil {
		var one string
		if json.Unmarshal(claims.Aud, &one) != nil {
			return errors.New("it has no aud claim")
		}
		auds = []string{one}
	}
	found := false
	for _, x := range auds {
		found = found || x == aud
	}
	if !found {
		return errors.New("its audience is not this Access application's")
	}
	if claims.Iss == nil || *claims.Iss != "https://"+team {
		return errors.New("its issuer is not the configured Access team")
	}
	at := func(n *json.Number) (float64, bool, error) {
		if n == nil {
			return 0, false, nil
		}
		f, err := n.Float64()
		return f, true, err
	}
	t := float64(now.UnixMicro()) / 1e6
	exp, ok, err := at(claims.Exp)
	if err != nil || !ok {
		return errors.New("it has no numeric exp claim")
	}
	if t >= exp {
		return errors.New("the token has expired")
	}
	for name, n := range map[string]*json.Number{"nbf": claims.Nbf, "iat": claims.Iat} {
		v, ok, err := at(n)
		if err != nil {
			return fmt.Errorf("its %s claim is not a number", name)
		}
		if ok && v > t {
			return fmt.Errorf("the token is not yet valid (%s)", name)
		}
	}
	return nil
}

// decodePart decodes one base64url JSON part of a JWT into v, numbers kept as json.Number.
func decodePart(part string, v any) error {
	raw, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		return errors.New("is not base64url")
	}
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.UseNumber()
	if err := d.Decode(v); err != nil {
		return errors.New("is not a JSON object")
	}
	return nil
}

// hostName is a Host header's name: without the port, lower case.
func hostName(host string) string {
	if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	return strings.ToLower(host)
}

// refusal is why a request is refused, or "". The service binds 127.0.0.1, so a Host other than this machine's names
// a rebinding domain, except site_url's: the tunnel brings that, so the request must carry a Cloudflare Access token
// that verifies against the team's keys, for this application, unexpired. A request Cloudflare's edge forwarded
// (EdgeHeader) needs the token whatever its Host, so a tunnel that rewrites Host to this machine's does not skip
// Access. It fails closed: without the Access keys in the config, or the team's keys, every such request is refused.
func (s *server) refusal(r *http.Request) string {
	name := hostName(r.Host)
	_, edge := r.Header[http.CanonicalHeaderKey(EdgeHeader)]
	if (name == "127.0.0.1" || name == "localhost") && !edge {
		return ""
	}
	public := ""
	if u, err := url.Parse(s.d.SiteURL); err == nil {
		public = strings.ToLower(u.Hostname())
	}
	if (public == "" || name != public) && !edge {
		shown := "None"
		if r.Host != "" {
			shown = proc.Repr(r.Host)
		}
		return fmt.Sprintf("refused: Host %s is neither this machine nor the configured site_url; open the site at %s",
			shown, s.siteURL())
	}
	var missing []string
	if s.d.AccessTeam == "" {
		missing = append(missing, "access_team (the Cloudflare Access team domain, <team>.cloudflareaccess.com)")
	}
	if s.d.AccessAud == "" {
		missing = append(missing, "access_aud (the Access application's AUD tag)")
	}
	if len(missing) > 0 {
		lead := "refused: "
		if s.d.SiteURL != "" {
			lead = "refused: site_url is set, but "
		}
		return fmt.Sprintf("%s%s lacks %s, so no request through the tunnel is verified; set them there (pm init "+
			"--help) and restart the pm service", lead, config.Rel, strings.Join(missing, " and "))
	}
	token := r.Header.Get(AccessHeader)
	if token == "" {
		return fmt.Sprintf("refused: the request carries no Cloudflare Access token (%s); open the site at %s",
			AccessHeader, s.siteURL())
	}
	if err := s.access.verify(token, s.d.AccessTeam, s.d.AccessAud, time.Now()); err != nil {
		return "refused: the Cloudflare Access token does not verify: " + err.Error()
	}
	return ""
}
