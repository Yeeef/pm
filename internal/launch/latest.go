package launch

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// latestTimeout bounds each request for the latest release: pm doctor waits on it.
const latestTimeout = 10 * time.Second

// Release is a published pm release: its version and the URL of its page, which holds its notes.
type Release struct{ Version, Notes string }

// Latest is pm's latest release, a pre-release never, as GitHub's API names it (<api>/releases/latest): asked with no
// token, then, when that fails, with a token from $GH_TOKEN or gh auth token, as the launcher downloads. Fails, never
// guessing, when neither answers with a pm-v<X> release.
func Latest() (Release, error) {
	r, err := latest("")
	if err == nil {
		return r, nil
	}
	if token := GitHubToken(); token != "" {
		if r, err2 := latest(token); err2 == nil {
			return r, nil
		}
	}
	return Release{}, err
}

func latest(token string) (Release, error) {
	link := apiURL() + "/releases/latest"
	req, err := http.NewRequest(http.MethodGet, link, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := (&http.Client{Timeout: latestTimeout}).Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("GET %s: %s", link, strerror(err))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Release{}, fmt.Errorf("GET %s: %s", link, strerror(err))
	}
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("GET %s: HTTP %d", link, resp.StatusCode)
	}
	var data struct {
		Tag  string `json:"tag_name"`
		Page string `json:"html_url"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return Release{}, fmt.Errorf("GET %s: not a release: %v", link, err)
	}
	v, ok := strings.CutPrefix(data.Tag, "pm-v")
	if _, numeric := Key(v); !ok || !numeric || data.Page == "" {
		return Release{}, fmt.Errorf("GET %s: not a pm release (tag %q, page %q)", link, data.Tag, data.Page)
	}
	return Release{Version: v, Notes: data.Page}, nil
}

// Newer is whether version a is newer than b: a higher number, or the release of b's pre-release (0.4.0 over
// 0.4.0-rc.1). False when either is not a version.
func Newer(a, b string) bool {
	ka, okA := Key(a)
	kb, okB := Key(b)
	if !okA || !okB {
		return false
	}
	if Less(kb, ka) {
		return true
	}
	return !Less(ka, kb) && strings.Contains(b, "-") && !strings.Contains(a, "-")
}
