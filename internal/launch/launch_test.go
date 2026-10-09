package launch

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Yeeef/yeeef-agents/pm/internal/buildinfo"
)

// TestMain doubles as the launcher under test: with LAUNCH_HELPER set, the test binary is pm's main, so the exec
// paths run for real in a child process.
func TestMain(m *testing.M) {
	if os.Getenv("LAUNCH_HELPER") != "" {
		buildinfo.Version = os.Getenv("LAUNCH_HELPER")
		if err := Launch(os.Args[1:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			os.Exit(1)
		}
		fmt.Println("in process")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// fakePM is the release binary the tarball holds: it prints its argv and the markers it got.
const fakePM = "#!/bin/sh\necho \"launched $* marks=$PM_LAUNCHED/$PM_LAUNCHER\"\n"

func tarball(t *testing.T, name, body string, typ byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: typ}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// release serves files under /pm-v<version>/ and counts the requests.
type release struct {
	files map[string][]byte
	hits  int
	srv   *httptest.Server
}

func serve(t *testing.T, version string, tgz []byte, sums string) *release {
	t.Helper()
	asset := assetOf(t, version)
	if sums == "" {
		sums = sum(tgz) + "  " + asset + "\n" + strings.Repeat("0", 64) + "  pm-" + version + "-other-arch.tar.gz\n"
	}
	r := &release{files: map[string][]byte{"/pm-v" + version + "/" + asset: tgz,
		"/pm-v" + version + "/SHA256SUMS": []byte(sums)}}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.hits++
		b, ok := r.files[req.URL.Path]
		if !ok {
			http.NotFound(w, req)
			return
		}
		w.Write(b)
	}))
	t.Cleanup(r.srv.Close)
	t.Setenv("PM_RELEASE_URL", r.srv.URL+"//")
	return r
}

func assetOf(t *testing.T, version string) string {
	t.Helper()
	plat, ok := Platform()
	if !ok {
		t.Skipf("pm is not released for %s", plat)
	}
	return "pm-" + version + "-" + plat + ".tar.gz"
}

func dataDir(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	t.Setenv("XDG_DATA_HOME", d)
	return filepath.Join(d, "pm/pins")
}

func list(t *testing.T, dir string) []string {
	t.Helper()
	es, _ := os.ReadDir(dir)
	var names []string
	for _, e := range es {
		names = append(names, e.Name())
	}
	return names
}

func TestDownloadKeepsTheCheckedBinaryAndItsSha(t *testing.T) {
	pins := dataDir(t)
	tgz := tarball(t, "pm", fakePM, tar.TypeReg)
	serve(t, "0.2.0", tgz, "")
	if err := Download("0.2.0"); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(pins, "0.2.0")
	b, _ := os.ReadFile(filepath.Join(dir, "pm"))
	st, _ := os.Stat(filepath.Join(dir, "pm"))
	kept, _ := os.ReadFile(filepath.Join(dir, "sha256"))
	if string(b) != fakePM || st.Mode().Perm() != 0o755 || string(kept) != sum(tgz)+"\n" {
		t.Fatalf("pm %q mode %v sha256 %q", b, st.Mode(), kept)
	}
	if got := list(t, dir); strings.Join(got, ",") != "pm,sha256" {
		t.Fatalf("temp files left: %v", got)
	}
}

func TestDownloadFailsHardAndInstallsNothing(t *testing.T) {
	good := tarball(t, "pm", fakePM, tar.TypeReg)
	asset := assetOf(t, "0.2.0")
	cases := []struct {
		name, sums, keptSum, want string
		tgz                       []byte
		missing                   bool
	}{
		{name: "checksum mismatch", tgz: good, sums: strings.Repeat("a", 64) + "  " + asset + "\n",
			want: "could not be checked: " + "URL/" + asset + " has sha256 " + sum(good) + ", but SHA256SUMS says " +
				strings.Repeat("a", 64) + fix},
		{name: "no line for the asset", tgz: good, sums: sum(good) + "  pm-0.2.0-plan9-amd64.tar.gz\n",
			want: "could not be checked: URL/SHA256SUMS has no line for " + asset + fix},
		{name: "kept sha256 differs", tgz: good, keptSum: strings.Repeat("b", 64) + "\n",
			want: "changed since this machine first downloaded it: URL/" + asset + " has sha256 " + sum(good) + ", but "},
		{name: "no pm in the tarball", tgz: tarball(t, "bin/pm", fakePM, tar.TypeReg),
			want: "could not be unpacked: URL/" + asset + " is not a gzip tar holding one file pm" + fix},
		{name: "pm is not a regular file", tgz: tarball(t, "pm", "", tar.TypeSymlink),
			want: "could not be unpacked: URL/" + asset + " is not a gzip tar holding one file pm" + fix},
		{name: "not a gzip tar", tgz: []byte("pm"), want: "could not be unpacked: URL/"},
		{name: "missing release", tgz: good, missing: true, want: "could not be downloaded: URL/SHA256SUMS: HTTP 404" + fix},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pins := dataDir(t)
			r := serve(t, "0.2.0", c.tgz, c.sums)
			if c.missing {
				r.files = map[string][]byte{}
			}
			dir := filepath.Join(pins, "0.2.0")
			if c.keptSum != "" {
				os.MkdirAll(dir, 0o755)
				os.WriteFile(filepath.Join(dir, "sha256"), []byte(c.keptSum), 0o644)
			}
			err := Download("0.2.0")
			want := "this repo pins pm 0.2.0, but release pm-v0.2.0 " +
				strings.ReplaceAll(c.want, "URL/", r.srv.URL+"/pm-v0.2.0/")
			if err == nil || !strings.HasPrefix(err.Error(), want) {
				t.Fatalf("err = %v, want %q", err, want)
			}
			if got := list(t, dir); c.keptSum == "" && got != nil || c.keptSum != "" && len(got) != 1 {
				t.Fatalf("left %v", got)
			} else if _, err := os.Stat(dir); c.keptSum == "" && err == nil {
				t.Fatal("the pin dir was made")
			}
		})
	}
}

func TestDownloadGivesUpAtItsDeadline(t *testing.T) {
	dataDir(t)
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-block }))
	defer srv.Close()
	defer close(block)
	t.Setenv("PM_RELEASE_URL", srv.URL)
	defer func(d time.Duration) { downloadTimeout = d }(downloadTimeout)
	downloadTimeout = 100 * time.Millisecond
	assetOf(t, "0.2.0")
	err := Download("0.2.0")
	if err == nil || !strings.Contains(err.Error(), "not downloaded within 0.1 s") {
		t.Fatalf("err = %v", err)
	}
}

func TestMirrorURL(t *testing.T) {
	t.Setenv("PM_RELEASE_URL", "")
	if MirrorURL() != "" {
		t.Fatal(MirrorURL())
	}
	t.Setenv("PM_RELEASE_URL", "http://mirror/x///")
	if MirrorURL() != "http://mirror/x" {
		t.Fatal(MirrorURL())
	}
}

func TestIsGo(t *testing.T) {
	for v, want := range map[string]bool{"0.2.0": true, "0.2.0-rc.1": true, "0.10.3": true, "1.0": true,
		"0.1.4": false, "0.1.9-rc.1": false, "0.2": false, "dev": false, "": false, "0.x.0": false} {
		if IsGo(v) != want {
			t.Errorf("IsGo(%q) = %v", v, !want)
		}
	}
}

func TestTargetRunsInProcessForItsOwnPinTheLaunchedPinAndUpgrade(t *testing.T) {
	defer func(v string) { buildinfo.Version = v }(buildinfo.Version)
	buildinfo.Version = "0.2.0"
	repo := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	os.MkdirAll(filepath.Join(repo, ".pm"), 0o755)
	os.WriteFile(filepath.Join(repo, ".pm/config.toml"), []byte("version = \"9.9.9\"\n"), 0o644)
	for _, c := range []struct {
		argv          []string
		cwd, launched string
		want          string
	}{
		{[]string{"show"}, repo, "", "9.9.9"},
		{[]string{"show"}, repo, "9.9.9", ""},
		{[]string{"show"}, repo, "8.8.8", "9.9.9"},
		{[]string{"upgrade"}, repo, "", ""},
		{[]string{"upgrade", "--to", "8.8.8"}, repo, "", "8.8.8"},
		{[]string{"upgrade", "--to=8.8.8"}, repo, "", "8.8.8"},
		{[]string{"upgrade", "--to", "0.2.0"}, repo, "", ""},
		{[]string{"upgrade", "--to"}, repo, "", ""},
		{[]string{"show"}, filepath.Join(repo, "nowhere"), "", ""},
	} {
		if got := Target(c.argv, c.cwd, c.launched); got != c.want {
			t.Errorf("Target(%v, launched %q) = %q, want %q", c.argv, c.launched, got, c.want)
		}
	}
}

func TestScrubTakesTheMarksAndAnOldPinsToolDirs(t *testing.T) {
	pins := dataDir(t)
	old := filepath.Join(pins, "0.1.0")
	t.Setenv(Launched, "0.2.0")
	t.Setenv(Launcher, "0.1.5")
	t.Setenv("UV_TOOL_DIR", filepath.Join(old, "tools"))
	t.Setenv("UV_TOOL_BIN_DIR", "/elsewhere/bin")
	t.Setenv("PATH", filepath.Join(old, "bin")+":/usr/bin:"+pins+"x/bin")
	defer func(v string) { buildinfo.Version = v }(buildinfo.Version)
	buildinfo.Version = "0.2.0"
	Scrub()
	_, l1 := os.LookupEnv(Launched)
	_, l2 := os.LookupEnv(Launcher)
	_, td := os.LookupEnv("UV_TOOL_DIR")
	if l1 || l2 || td || os.Getenv("UV_TOOL_BIN_DIR") != "/elsewhere/bin" || os.Getenv("PATH") != "/usr/bin:"+pins+"x/bin" {
		t.Fatalf("env after scrub: PATH=%s", os.Getenv("PATH"))
	}
	if !IsLaunched() || !strings.HasPrefix(How(), "this repo's pin, "+filepath.Join(pins, "0.2.0/pm")+
		" from release pm-v0.2.0, launched by pm 0.1.5; delete ") {
		t.Fatal(How())
	}
}

// Markers hands the launcher's markers to a child that is this same pm (the session-start pm init), and only when
// this pm was launched: such a child runs as the pin itself, and without them would take the bin dir from the launcher.
func TestMarkersAreThoseScrubTookAndNoneUnlaunched(t *testing.T) {
	dataDir(t)
	defer func(v string) { buildinfo.Version = v }(buildinfo.Version)
	buildinfo.Version = "0.3.0"
	t.Setenv(Launched, "0.3.0")
	t.Setenv(Launcher, "0.2.0")
	Scrub()
	if got := strings.Join(Markers(), " "); got != Launched+"=0.3.0 "+Launcher+"=0.2.0" {
		t.Fatalf("launched: %q", got)
	}
	os.Unsetenv(Launched)
	os.Unsetenv(Launcher)
	Scrub()
	if got := Markers(); got != nil {
		t.Fatalf("not launched: %q", got)
	}
}

// helper runs the test binary as a launcher of version in repo.
func helper(t *testing.T, version, repo string, env ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "show", "--x")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), append([]string{"LAUNCH_HELPER=" + version}, env...)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func pinnedRepo(t *testing.T, version string) string {
	t.Helper()
	repo := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	os.MkdirAll(filepath.Join(repo, ".pm"), 0o755)
	os.WriteFile(filepath.Join(repo, ".pm/config.toml"), []byte(fmt.Sprintf("version = %q\n", version)), 0o644)
	return repo
}

func TestAGoPinIsDownloadedOnceThenExecdWithTheMarks(t *testing.T) {
	dataDir(t)
	r := serve(t, "0.3.0-rc.1", tarball(t, "pm", fakePM, tar.TypeReg), "")
	repo := pinnedRepo(t, "0.3.0-rc.1")
	for i := 0; i < 2; i++ {
		out, err := helper(t, "0.2.0", repo)
		if err != nil || out != "launched show --x marks=0.3.0-rc.1/0.2.0\n" {
			t.Fatalf("run %d: %v %q", i, err, out)
		}
	}
	if r.hits != 2 { // SHA256SUMS and the tarball, on the first launch only
		t.Fatalf("%d requests", r.hits)
	}
	out, err := helper(t, "0.2.0", repo, Launched+"=0.3.0-rc.1")
	if err != nil || out != "in process\n" {
		t.Fatalf("launched for its pin: %v %q", err, out)
	}
}

func TestAGoPinThatCannotBeDownloadedFailsHard(t *testing.T) {
	dataDir(t)
	r := serve(t, "0.3.0", nil, "")
	r.files = map[string][]byte{}
	out, err := helper(t, "0.2.0", pinnedRepo(t, "0.3.0"))
	want := "error: this repo pins pm 0.3.0, but release pm-v0.3.0 could not be downloaded: " + r.srv.URL +
		"/pm-v0.3.0/SHA256SUMS: HTTP 404" + fix + "\n"
	if err == nil || out != want {
		t.Fatalf("%v %q", err, out)
	}
}

func TestAKeptBinaryThatCannotRunFailsHard(t *testing.T) {
	pins := dataDir(t)
	bin := filepath.Join(pins, "0.3.0/pm")
	os.MkdirAll(filepath.Dir(bin), 0o755)
	os.WriteFile(bin, []byte{0, 1, 2, 3}, 0o755)
	out, err := helper(t, "0.2.0", pinnedRepo(t, "0.3.0"))
	want := "error: this repo pins pm 0.3.0, but " + bin + " could not run: Exec format error; delete it to " +
		"download it again\n"
	if err == nil || out != want {
		t.Fatalf("%v %q", err, out)
	}
}

// fakeUV logs its argv and markers, and fakeGit answers ls-remote, so the Python pin path runs without a network.
const fakeUV = "#!/bin/sh\necho \"uv $* marks=$PM_LAUNCHED/$PM_LAUNCHER tool=$UV_TOOL_DIR\" >> \"$LOG\"\necho ran\n"
const fakeGit = "#!/bin/sh\nif [ \"$1\" = ls-remote ]; then echo \"git $* prompt=$GIT_TERMINAL_PROMPT\" >> \"$LOG\"; " +
	"printf '%s\\trefs/tags/pm-v%s\\n' 0123456789abcdef0123456789abcdef01234567 0.1.4; exit 0; fi\nexec \"$REALGIT\" \"$@\"\n"

func TestAPythonPinResolvesItsTagBuildsOnceAndExecsUV(t *testing.T) {
	pins := dataDir(t)
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "uv"), []byte(fakeUV), 0o755)
	os.WriteFile(filepath.Join(bin, "git"), []byte(fakeGit), 0o755)
	git, _ := exec.LookPath("git")
	log := filepath.Join(t.TempDir(), "log")
	repo := pinnedRepo(t, "0.1.4")
	env := []string{"PATH=" + bin + ":" + os.Getenv("PATH"), "LOG=" + log, "REALGIT=" + git}
	for i := 0; i < 2; i++ {
		if out, err := helper(t, "0.2.0", repo, env...); err != nil || out != "ran\n" {
			t.Fatalf("%v %q", err, out)
		}
	}
	b, _ := os.ReadFile(log)
	sha := "0123456789abcdef0123456789abcdef01234567"
	req := "git+https://github.com/Yeeef/yeeef-agents@" + sha + "#subdirectory=pm"
	want := "git ls-remote https://github.com/Yeeef/yeeef-agents refs/tags/pm-v0.1.4 refs/tags/pm-v0.1.4^{} prompt=0\n" +
		"uv tool run --from " + req + " pm --help marks=0.1.4/0.2.0 tool=\n" +
		strings.Repeat("uv --quiet tool run --from "+req+" pm show --x marks=0.1.4/0.2.0 tool=\n", 2)
	if string(b) != want {
		t.Fatalf("log:\n%s\nwant:\n%s", b, want)
	}
	if c, _ := os.ReadFile(filepath.Join(pins, "0.1.4/commit")); string(c) != sha+"\n" {
		t.Fatalf("commit %q", c)
	}
}
