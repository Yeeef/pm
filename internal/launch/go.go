package launch

// A Go pin (>= 0.2.0) runs its release binary, kept at pins/<pin>/pm. The first launch on a machine downloads
// SHA256SUMS and the tarball for this OS and architecture from release pm-v<pin> (10 s to connect, 300 s for the
// whole download), checks the tarball against its SHA256SUMS line and against the sha256 a former download kept in
// pins/<pin>/sha256, and writes the binary and then that sha256, each atomically (a temp file renamed), so another
// launch sees no file or a whole one. Every failure is a hard error that names the release and the URL.

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
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

// Asset is the release tarball of version for this machine; ok is false on a platform pm is not released for.
func Asset(version string) (name string, ok bool) {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "darwin/arm64", "linux/amd64":
		return fmt.Sprintf("pm-%s-%s-%s.tar.gz", version, runtime.GOOS, runtime.GOARCH), true
	}
	return "", false
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
	return syscall.Exec(bin, append([]string{bin}, argv...),
		environ(map[string]string{Launched: version, Launcher: buildinfo.Version}))
}

// Download fetches release pm-v<version>'s binary for this machine into pins/<version>/pm, checked against the
// release's SHA256SUMS and the sha256 a former download kept.
func Download(version string) error {
	tag := "pm-v" + version
	base := ReleaseURL() + "/" + tag
	head := fmt.Sprintf("this repo pins pm %s, which pm %s downloads from release %s", version, buildinfo.Version, tag)
	asset, ok := Asset(version)
	if !ok {
		return fmt.Errorf("%s, but release %s has no build for %s/%s (%s)", head, tag, runtime.GOOS, runtime.GOARCH, base)
	}
	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{Timeout: connectTimeout}).DialContext, TLSHandshakeTimeout: connectTimeout}}
	fail := func(url string, err error) error {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			err = fmt.Errorf("not done within %g s", downloadTimeout.Seconds())
		}
		return fmt.Errorf("%s, but %s could not be downloaded: %v; check the network and that release %s exists, then "+
			"run pm again, or move the pin with pm upgrade", head, url, err, tag)
	}
	get := func(url string, to io.Writer) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fail(url, err)
		}
		resp, err := client.Do(req)
		if err != nil {
			return fail(url, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fail(url, fmt.Errorf("HTTP %s", resp.Status))
		}
		if _, err := io.Copy(to, resp.Body); err != nil {
			return fail(url, err)
		}
		return nil
	}

	sumsURL := base + "/SHA256SUMS"
	var sums strings.Builder
	if err := get(sumsURL, &sums); err != nil {
		return err
	}
	want := ""
	sc := bufio.NewScanner(strings.NewReader(sums.String()))
	for sc.Scan() {
		if sum, name, ok := strings.Cut(sc.Text(), "  "); ok && name == asset && sumRe.MatchString(sum) {
			want = sum
		}
	}
	if want == "" {
		return fmt.Errorf("%s, but %s has no sha256 line for %s", head, sumsURL, asset)
	}

	dir := PinDir(version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tarURL := base + "/" + asset
	tarball, err := os.CreateTemp(dir, asset+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tarball.Name())
	defer tarball.Close()
	h := sha256.New()
	if err := get(tarURL, io.MultiWriter(tarball, h)); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != want {
		return fmt.Errorf("%s, but %s has sha256 %s, not the %s that %s gives it; nothing was installed", head, tarURL,
			got, want, sumsURL)
	}
	keptSum := filepath.Join(dir, "sha256")
	if b, err := os.ReadFile(keptSum); err == nil && config.PyStrip(string(b)) != got {
		return fmt.Errorf("%s, but %s has sha256 %s, not the %s kept in %s from an earlier download: the release "+
			"changed; delete %s to accept it", head, tarURL, got, config.PyStrip(string(b)), keptSum, dir)
	}
	if _, err := tarball.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := extract(tarball, filepath.Join(dir, "pm")); err != nil {
		return fmt.Errorf("%s, but %s holds no pm binary: %v", head, tarURL, err)
	}
	return writeAtomic(keptSum, []byte(got+"\n"), 0o644)
}

// extract writes the regular file pm in the gzip tar r to path, executable, atomically.
func extract(r io.Reader, path string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return errors.New("no regular file pm in the archive")
		}
		if err != nil {
			return err
		}
		if hdr.Name == "pm" && hdr.Typeflag == tar.TypeReg {
			return writeAtomicFrom(path, tr, 0o755)
		}
	}
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	return writeAtomicFrom(path, strings.NewReader(string(data)), mode)
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
