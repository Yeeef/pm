package launch

// A Go pin (>= 0.2.0, pre-releases such as 0.2.0-rc.1 too) runs its release binary, kept at pins/<pin>/pm, with no
// network once it is there. The first launch on a machine downloads SHA256SUMS and the tarball for this OS and
// architecture from release pm-v<pin> (10 s to connect, 300 s in all). The tarball must match its SHA256SUMS line and
// the sha256 an earlier download kept in pins/<pin>/sha256, if any: a release is never rebuilt, so a difference fails
// hard. The binary, then its sha256, is written to a temp file and renamed into place, so another launch sees no file
// or the whole one; nothing is written under pins/ before the tarball has passed every check. Every failure is a hard
// error naming the release and the URL; nothing falls back. The texts are launch.py's (the bridge release's), which
// test_launch.py holds for both.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/Yeeef/yeeef-agents/pm/internal/buildinfo"
	"github.com/Yeeef/yeeef-agents/pm/internal/config"
)

const (
	// DefaultReleaseURL is where pm's releases are: <base>/pm-v<X>/<asset>. $PM_RELEASE_URL replaces it (mirrors,
	// tests).
	DefaultReleaseURL = "https://github.com/Yeeef/yeeef-agents/releases/download"
	connectTimeout    = 10 * time.Second
	fix               = "; check the network and the release, then run pm again, or move the pin with pm upgrade"
)

// downloadTimeout bounds the whole download, SHA256SUMS and the tarball; a var for the test of it.
var downloadTimeout = 300 * time.Second

var sumRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ReleaseURL is the release base URL: $PM_RELEASE_URL without trailing slashes, or DefaultReleaseURL.
func ReleaseURL() string {
	if u := os.Getenv("PM_RELEASE_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return DefaultReleaseURL
}

// Platform is this machine's <os>-<arch> as release assets name it; ok is false on one pm is not released for.
func Platform() (plat string, ok bool) {
	plat = runtime.GOOS + "-" + runtime.GOARCH
	return plat, plat == "darwin-arm64" || plat == "linux-amd64"
}

// executable is whether path is an executable regular file.
func executable(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular() && st.Mode()&0o111 != 0
}

// execGo replaces this process with the pin's release binary, downloading it first when it is not kept.
func execGo(version string, argv []string) error {
	bin := filepath.Join(PinDir(version), "pm")
	if !executable(bin) {
		if err := Download(version); err != nil {
			return err
		}
	}
	err := syscall.Exec(bin, append([]string{bin}, argv...),
		environ(map[string]string{Launched: version, Launcher: buildinfo.Version}))
	return fmt.Errorf("this repo pins pm %s, but %s could not run: %s; delete it to download it again", version, bin,
		strerror(err))
}

// strerror is the C library's text for an errno, as Python's OSError.strerror gives it ("Exec format error").
func strerror(err error) string {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		s := errno.Error()
		return strings.ToUpper(s[:1]) + s[1:]
	}
	return err.Error()
}

// Download fetches release pm-v<version>'s binary for this machine into pins/<version>/pm, checked against the
// release's SHA256SUMS and the sha256 an earlier download kept.
func Download(version string) error {
	dir := PinDir(version)
	tag := "pm-v" + version
	base := ReleaseURL() + "/" + tag
	head := fmt.Sprintf("this repo pins pm %s, but release %s", version, tag)
	plat, ok := Platform()
	if !ok {
		return fmt.Errorf("%s has no binary for %s (only darwin-arm64 and linux-amd64): %s", head, plat, base)
	}
	asset := fmt.Sprintf("pm-%s-%s.tar.gz", version, plat)
	sumsURL, tarURL := base+"/SHA256SUMS", base+"/"+asset
	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()

	var sums bytes.Buffer
	if _, err := download(ctx, sumsURL, &sums, head); err != nil {
		return err
	}
	want := ""
	for _, line := range strings.Split(sums.String(), "\n") {
		if f := strings.Fields(line); want == "" && len(f) == 2 && f[1] == asset && sumRe.MatchString(f[0]) {
			want = f[0]
		}
	}
	if want == "" {
		return fmt.Errorf("%s could not be checked: %s has no line for %s%s", head, sumsURL, asset, fix)
	}

	tarball, err := os.CreateTemp("", "pm-release-*.tar.gz") // outside pins/: nothing is kept before every check
	if err != nil {
		return err
	}
	defer os.Remove(tarball.Name())
	defer tarball.Close()
	got, err := download(ctx, tarURL, tarball, head)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("%s could not be checked: %s has sha256 %s, but SHA256SUMS says %s%s", head, tarURL, got,
			want, fix)
	}
	keptSum := filepath.Join(dir, "sha256")
	b, err := os.ReadFile(keptSum)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err // a kept sha256 that cannot be read is no check passed
	}
	if err == nil && config.PyStrip(string(b)) != got {
		return fmt.Errorf("%s changed since this machine first downloaded it: %s has sha256 %s, but %s keeps %s; a "+
			"release is never rebuilt, so check where it came from before you delete that file", head, tarURL, got,
			keptSum, config.PyStrip(string(b)))
	}
	bad := fmt.Errorf("%s could not be unpacked: %s is not a gzip tar holding one file pm%s", head, tarURL, fix)
	if !onlyPM(tarball) {
		return bad
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := extract(tarball, filepath.Join(dir, "pm")); err != nil {
		return bad
	}
	return writeAtomicFrom(keptSum, strings.NewReader(got+"\n"), 0o644)
}

// download writes the body of GET url to out within ctx; its sha256 hex. head frames a failure's error.
func download(ctx context.Context, link string, out io.Writer, head string) (string, error) {
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: connectTimeout}).DialContext,
		TLSHandshakeTimeout: connectTimeout, ResponseHeaderTimeout: connectTimeout}}
	fail := func(err error) (string, error) {
		why := err.Error()
		var uerr *url.Error
		if errors.As(err, &uerr) {
			why = uerr.Err.Error()
		}
		var nerr net.Error
		switch {
		case ctx.Err() != nil:
			why = fmt.Sprintf("not downloaded within %g s", downloadTimeout.Seconds())
		case errors.As(err, &nerr) && nerr.Timeout():
			why = fmt.Sprintf("no answer within %g s", connectTimeout.Seconds())
		}
		return "", fmt.Errorf("%s could not be downloaded: %s: %s%s", head, link, why, fix)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return fail(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fail(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fail(fmt.Errorf("HTTP %d", resp.StatusCode))
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(out, h), resp.Body); err != nil {
		return fail(err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// onlyPM is whether the gzip tar f holds exactly one member, the regular file pm.
func onlyPM(f io.ReadSeeker) bool {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return false
	}
	gz, err := gzip.NewReader(f)
	if err != nil {
		return false
	}
	tr := tar.NewReader(gz)
	n := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return n == 1
		}
		if err != nil || hdr.Name != "pm" || hdr.Typeflag != tar.TypeReg { // the reader reports an old-style '\x00' file as TypeReg
			return false
		}
		if _, err := io.Copy(io.Discard, tr); err != nil {
			return false
		}
		n++
	}
}

// extract writes the file pm in the gzip tar f, which onlyPM checked, to path, executable, atomically.
func extract(f io.ReadSeeker, path string) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	if _, err := tr.Next(); err != nil {
		return err
	}
	return writeAtomicFrom(path, tr, 0o755)
}

// writeAtomicFrom writes r to a temp file beside path, sets its mode and renames it to path.
func writeAtomicFrom(path string, r io.Reader, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name()) // gone after the rename; left only by a failure
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
