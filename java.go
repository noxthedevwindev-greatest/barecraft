package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type javaInfo struct {
	Path  string
	Major int
}

// javaMajor runs `java -version` and parses the major version.
func javaMajor(bin string) int {
	out, err := exec.Command(bin, "-version").CombinedOutput()
	if len(out) == 0 && err != nil {
		return 0
	}
	s := string(out)
	i := strings.Index(s, `"`)
	if i < 0 {
		return 0
	}
	rest := s[i+1:]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return 0
	}
	ver := rest[:j]
	parts := strings.Split(ver, ".")
	if len(parts) >= 2 && parts[0] == "1" {
		if n, err := strconv.Atoi(parts[1]); err == nil {
			return n
		}
		return 0
	}
	if n, err := strconv.Atoi(parts[0]); err == nil {
		return n
	}
	return 0
}

// pickJava finds a suitable JVM: exact for the old Java-8 era, otherwise a
// close-enough modern one (req..req+3), else downloads Temurin JRE.
func pickJava(root string, req int) (javaInfo, error) {
	if req <= 0 {
		req = 8
	}
	lo, hi := req, req
	if req > 8 {
		hi = req + 3
	}
	var all []string
	seen := map[string]bool{}
	for _, c := range candidateJavas() {
		if seen[c] {
			continue
		}
		seen[c] = true
		all = append(all, c)
	}
	for _, bin := range all {
		if maj := javaMajor(bin); maj >= lo && maj <= hi {
			return javaInfo{Path: bin, Major: maj}, nil
		}
	}
	// nothing suitable: download Temurin
	bin, err := downloadTemurin(root, req)
	if err != nil {
		return javaInfo{}, fmt.Errorf("need Java %d but none found, and download failed: %w", req, err)
	}
	maj := javaMajor(bin)
	if maj == 0 {
		return javaInfo{}, fmt.Errorf("downloaded Java %d at %s won't run", req, bin)
	}
	return javaInfo{Path: bin, Major: maj}, nil
}

type adoptiumResp []struct {
	Binary struct {
		Package struct {
			Link     string `json:"link"`
			Name     string `json:"name"`
			Checksum string `json:"checksum"`
		} `json:"package"`
		ImageType string `json:"image_type"`
	} `json:"binary"`
}

func temurinArch() (string, error) {
	switch runtime.GOARCH {
	case "amd64":
		return "x64", nil
	case "arm64":
		return "aarch64", nil
	}
	return "", fmt.Errorf("no prebuilt Java for architecture %s", runtime.GOARCH)
}

func temurinOS() string {
	switch runtime.GOOS {
	case "windows":
		return "windows"
	case "darwin":
		return "mac"
	default:
		return "linux"
	}
}

// downloadTemurin fetches a Temurin JRE into root/java/jre-<req>-<os>-<arch>.
func downloadTemurin(root string, req int) (string, error) {
	arch, err := temurinArch()
	if err != nil {
		return "", err
	}
	osName := temurinOS()
	home := filepath.Join(root, "java", fmt.Sprintf("jre-%d-%s-%s", req, osName, arch))
	bin := filepath.Join(home, "bin", javaExeName())
	if _, err := os.Stat(bin); err == nil {
		return bin, nil
	}

	info("downloading Java %d (Temurin JRE, %s/%s)...", req, osName, arch)
	url := fmt.Sprintf("https://api.adoptium.net/v3/assets/latest/%d/hotspot?architecture=%s&image_type=jre&os=%s&vendor=eclipse",
		req, arch, osName)
	var res adoptiumResp
	if err := getJSON(url, &res); err != nil {
		return "", fmt.Errorf("query Adoptium: %w", err)
	}
	var pkg *adoptiumResp
	for i := range res {
		if res[i].Binary.Package.Link != "" && (res[i].Binary.ImageType == "" || res[i].Binary.ImageType == "jre") {
			pkg = &res
			break
		}
	}
	if pkg == nil {
		return "", fmt.Errorf("Adoptium has no JRE %d for %s/%s — install Java %d yourself and re-run", req, osName, arch, req)
	}
	p := (*pkg)[0].Binary.Package
	archivePath := filepath.Join(root, "cache", p.Name)
	if err := downloadFile(p.Link, archivePath, 0, "", p.Checksum); err != nil {
		return "", fmt.Errorf("fetch JRE: %w", err)
	}

	tmp := filepath.Join(root, "cache", fmt.Sprintf("jre-extract-%d", req))
	os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)

	var extractRoot string
	switch {
	case strings.HasSuffix(p.Name, ".tar.gz"), strings.HasSuffix(p.Name, ".tgz"):
		extractRoot = tmp
		if err := extractTarGz(archivePath, tmp); err != nil {
			return "", fmt.Errorf("extract JRE: %w", err)
		}
	case strings.HasSuffix(p.Name, ".zip"):
		extractRoot = tmp
		if err := extractZip(archivePath, tmp); err != nil {
			return "", fmt.Errorf("extract JRE: %w", err)
		}
	default:
		return "", fmt.Errorf("unknown JRE archive %s", p.Name)
	}

	// find bin/java inside (archives have a single top-level dir)
	found := ""
	filepath.Walk(extractRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if filepath.Base(path) == javaExeName() && filepath.Base(filepath.Dir(path)) == "bin" {
			found = path
			return io.EOF
		}
		return nil
	})
	if found == "" {
		return "", fmt.Errorf("no %s inside %s", javaExeName(), p.Name)
	}
	javaHome := filepath.Dir(filepath.Dir(found))

	if err := os.MkdirAll(filepath.Dir(home), 0o755); err != nil {
		return "", err
	}
	os.RemoveAll(home)
	if err := os.Rename(javaHome, home); err != nil {
		return "", fmt.Errorf("install JRE: %w", err)
	}
	if runtime.GOOS != "windows" {
		filepath.Walk(filepath.Join(home, "bin"), func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				os.Chmod(path, 0o755)
			}
			return nil
		})
	}
	if _, err := os.Stat(bin); err != nil {
		return "", fmt.Errorf("JRE install incomplete: %s missing", bin)
	}
	info("Java %d installed at %s", req, home)
	return bin, nil
}

func extractTarGz(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dest, hdr.Name)
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
			continue // path traversal guard
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0o755)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0o755)
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
}

func extractZip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		target := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
			continue
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(target, 0o755)
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		os.MkdirAll(filepath.Dir(target), 0o755)
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			rc.Close()
			out.Close()
			return err
		}
		rc.Close()
		out.Close()
	}
	return nil
}
