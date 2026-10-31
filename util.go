package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func step(n, total int, format string, a ...any) {
	fmt.Printf("[%d/%d] %s\n", n, total, fmt.Sprintf(format, a...))
}

func info(format string, a ...any) {
	fmt.Printf("      %s\n", fmt.Sprintf(format, a...))
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "barecraft: error: %v\n", err)
	promptExit()
	os.Exit(1)
}

func promptExit() {
	if runtime.GOOS == "windows" && cfgFromOverlay {
		fmt.Print("\nPress Enter to close...")
		fmt.Scanln()
	}
}

func homeDir() string {
	if v := os.Getenv("BARECRAFT_HOME"); v != "" {
		return v
	}
	h, err := os.UserHomeDir()
	if err != nil {
		fatal(err)
	}
	return h
}

// rootDir is the shared launcher root: libraries, assets, versions, java, natives.
func rootDir() string {
	if v := os.Getenv("BARECRAFT_ROOT"); v != "" {
		return v
	}
	if runtime.GOOS == "windows" {
		if v := os.Getenv("LOCALAPPDATA"); v != "" {
			return filepath.Join(v, "barecraft")
		}
	}
	return filepath.Join(homeDir(), ".barecraft")
}

func osName() string {
	switch runtime.GOOS {
	case "windows":
		return "windows"
	case "darwin":
		return "osx"
	default:
		return "linux"
	}
}

func mojangArch() string {
	switch runtime.GOARCH {
	case "386":
		return "x86"
	case "amd64":
		return "x86_64"
	case "arm64":
		return "arm64"
	case "arm":
		return "arm"
	}
	return runtime.GOARCH
}

func javaExeName() string {
	if runtime.GOOS == "windows" {
		return "java.exe"
	}
	return "java"
}

// candidateJavas returns java binaries to probe, best guess first.
func candidateJavas() []string {
	var out []string
	add := func(p string) {
		if p != "" {
			out = append(out, p)
		}
	}
	if p, err := exec.LookPath("java"); err == nil {
		add(p)
	}
	if v := os.Getenv("JAVA_HOME"); v != "" {
		add(filepath.Join(v, "bin", javaExeName()))
	}
	switch runtime.GOOS {
	case "linux":
		if ents, err := filepath.Glob("/usr/lib/jvm/*/bin/" + javaExeName()); err == nil {
			out = append(out, ents...)
		}
		if ents, err := filepath.Glob(homeDir() + "/.sdkman/candidates/java/*/bin/" + javaExeName()); err == nil {
			out = append(out, ents...)
		}
	case "darwin":
		if ents, err := filepath.Glob("/Library/Java/JavaVirtualMachines/*/Contents/Home/bin/" + javaExeName()); err == nil {
			out = append(out, ents...)
		}
		if ents, err := filepath.Glob(homeDir() + "/Library/Java/JavaVirtualMachines/*/bin/" + javaExeName()); err == nil {
			out = append(out, ents...)
		}
	case "windows":
		for _, base := range []string{
			os.Getenv("ProgramFiles"),
			os.Getenv("ProgramFiles(x86)"),
			os.Getenv("ProgramW6432"),
		} {
			if base == "" {
				continue
			}
			for _, pat := range []string{
				filepath.Join(base, "Java", "*", "bin", "java.exe"),
				filepath.Join(base, "Eclipse Adoptium", "*", "bin", "java.exe"),
				filepath.Join(base, "Microsoft", "jdk-*", "bin", "java.exe"),
				filepath.Join(base, "Zulu", "*", "bin", "java.exe"),
				filepath.Join(base, "Temurin", "*", "bin", "java.exe"),
			} {
				if ents, err := filepath.Glob(pat); err == nil {
					out = append(out, ents...)
				}
			}
		}
	}
	return out
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '-' || r == '_' || r == '.' || r == '/':
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	s = strings.Trim(b.String(), "-")
	if s == "" {
		s = "instance"
	}
	if len(s) > 48 {
		s = s[:48]
	}
	return s
}

// shquote single-quotes a token for /bin/sh.
func shquote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// batQuote quotes a token for cmd.exe (and passes for .lnk Arguments).
func batQuote(s string) string {
	needs := false
	for _, r := range s {
		if r == ' ' || r == '"' || r == '&' || r == '(' || r == ')' || r == '^' || r == '%' || r == '!' {
			needs = true
			break
		}
	}
	if !needs {
		return s
	}
	s = strings.ReplaceAll(s, "%", "%%")
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func run(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, truncate(string(out), 2000))
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
