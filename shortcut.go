package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf16"
)

// createShortcuts drops a double-clickable game launcher into the OS's
// "start menu" and onto the desktop. Returns human-readable descriptions.
func createShortcuts(instance, slug string, cfg *Config, spec *launchSpec, pngPath, icoPath string) ([]string, error) {
	switch runtime.GOOS {
	case "linux":
		return linuxShortcuts(instance, slug, cfg, pngPath)
	case "darwin":
		return macShortcuts(instance, slug, cfg)
	case "windows":
		return windowsShortcuts(instance, slug, cfg, spec, icoPath)
	}
	return nil, fmt.Errorf("no shortcut support on %s", runtime.GOOS)
}

// ---------- Linux (.desktop, used by GNOME/KDE/XFCE/openbox/etc) ----------

func userAppsDir() string {
	// BARECRAFT_HOME redirects everything (tests, portable setups)
	if os.Getenv("BARECRAFT_HOME") != "" {
		return filepath.Join(homeDir(), ".local", "share", "applications")
	}
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, "applications")
	}
	return filepath.Join(homeDir(), ".local", "share", "applications")
}

// desktopDir resolves XDG_DESKTOP_DIR (user-dirs.dirs) with a ~/Desktop fallback.
func desktopDir() string {
	home := homeDir()
	if b, err := os.ReadFile(filepath.Join(home, ".config", "user-dirs.dirs")); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "XDG_DESKTOP_DIR=") {
				v := strings.TrimPrefix(line, "XDG_DESKTOP_DIR=")
				v = strings.Trim(v, `"`)
				v = strings.ReplaceAll(v, "$HOME", home)
				if v != "" {
					os.MkdirAll(v, 0o755)
					return v
				}
			}
		}
	}
	d := filepath.Join(home, "Desktop")
	os.MkdirAll(d, 0o755)
	return d
}

func linuxShortcuts(instance, slug string, cfg *Config, pngPath string) ([]string, error) {
	launchSh := filepath.Join(instance, "launch.sh")
	name := sanitizeOneLine(cfg.Name)
	body := "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Version=1.0\n" +
		"Name=" + name + "\n" +
		"Comment=Minecraft " + cfg.MC + launcherSuffix(cfg) + " set up by Barecraft\n" +
		"Exec=\"" + launchSh + "\"\n" +
		"Icon=" + pngPath + "\n" +
		"Terminal=false\n" +
		"Categories=Game;\n" +
		"StartupNotify=true\n"

	fileName := "barecraft-" + slug + ".desktop"
	var made []string
	for _, dir := range []string{userAppsDir(), desktopDir()} {
		p := filepath.Join(dir, fileName)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return made, err
		}
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			return made, err
		}
		os.Chmod(p, 0o755)
		// GNOME file managers only launch desktop files marked trusted
		if gio, err := exec.LookPath("gio"); err == nil {
			exec.Command(gio, "set", p, "metadata::trusted", "true").Run()
		}
		made = append(made, p)
	}
	return made, nil
}

// ---------- macOS (.app bundle; Launchpad shows ~/Applications) ----------

func macShortcuts(instance, slug string, cfg *Config) ([]string, error) {
	appName := sanitizeAppName(cfg.Name) + ".app"
	launchSh := filepath.Join(instance, "launch.sh")
	script, err := os.ReadFile(launchSh)
	if err != nil {
		return nil, err
	}
	var made []string
	for _, dir := range []string{filepath.Join(homeDir(), "Applications"), desktopDir()} {
		bundle := filepath.Join(dir, appName)
		os.RemoveAll(bundle)
		if err := writeAppBundle(bundle, cfg, slug, script); err != nil {
			return made, err
		}
		made = append(made, bundle)
	}
	return made, nil
}

func writeAppBundle(bundle string, cfg *Config, slug string, script []byte) error {
	if err := os.MkdirAll(filepath.Join(bundle, "Contents", "MacOS"), 0o755); err != nil {
		return err
	}
	var plist bytes.Buffer
	esc := func(s string) string {
		var b bytes.Buffer
		xml.EscapeText(&b, []byte(s))
		return b.String()
	}
	plist.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	plist.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	plist.WriteString(`<plist version="1.0">` + "\n<dict>\n")
	plist.WriteString("\t<key>CFBundleExecutable</key><string>barecraft-launch</string>\n")
	plist.WriteString("\t<key>CFBundleName</key><string>" + esc(sanitizeOneLine(cfg.Name)) + "</string>\n")
	plist.WriteString("\t<key>CFBundleIdentifier</key><string>dev.barecraft." + esc(slug) + "</string>\n")
	plist.WriteString("\t<key>CFBundlePackageType</key><string>APPL</string>\n")
	plist.WriteString("\t<key>CFBundleShortVersionString</key><string>1.0</string>\n")
	plist.WriteString("\t<key>NSHighResolutionCapable</key><true/>\n")
	plist.WriteString("</dict>\n</plist>\n")
	if err := os.WriteFile(filepath.Join(bundle, "Contents", "Info.plist"), []byte(plist.String()), 0o644); err != nil {
		return err
	}
	execPath := filepath.Join(bundle, "Contents", "MacOS", "barecraft-launch")
	if err := os.WriteFile(execPath, script, 0o755); err != nil {
		return err
	}
	return os.Chmod(execPath, 0o755)
}

// ---------- Windows (.lnk via WScript.Shell) ----------

func windowsShortcuts(instance, slug string, cfg *Config, spec *launchSpec, icoPath string) ([]string, error) {
	ps, err := findPowerShell()
	if err != nil {
		return nil, err
	}
	target := filepath.Join(filepath.Dir(spec.Java.Path), "javaw.exe")
	if _, err := os.Stat(target); err != nil {
		target = spec.Java.Path // console java; keep it working even without javaw
	}
	args := specCommandLine(spec, false)
	name := sanitizeWinName(cfg.Name) + ".lnk"
	desc := "Minecraft " + cfg.MC + launcherSuffix(cfg) + " — set up by Barecraft"

	script := fmt.Sprintf(
		"$w=New-Object -ComObject WScript.Shell;"+
			"foreach($d in @([Environment]::GetFolderPath('Desktop'),[Environment]::GetFolderPath('Programs'))){"+
			"$s=$w.CreateShortcut((Join-Path $d %s));"+
			"$s.TargetPath=%s;$s.Arguments=%s;$s.WorkingDirectory=%s;$s.IconLocation=%s;$s.Description=%s;$s.Save()}",
		psQuote(name), psQuote(target), psQuote(args), psQuote(instance),
		psQuote(icoPath+",0"), psQuote(desc))

	enc := utf16Base64(script)
	out, err := run(ps, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", enc)
	if err != nil {
		return nil, fmt.Errorf("create shortcuts: %w", err)
	}
	_ = out
	desktop, _ := os.UserHomeDir()
	return []string{
		filepath.Join(desktop, "Desktop", name),
		filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", name),
	}, nil
}

func findPowerShell() (string, error) {
	for _, c := range []string{"powershell.exe", "powershell", "pwsh.exe", "pwsh"} {
		if p, err := exec.LookPath(c); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("PowerShell not found (needed to create Windows shortcuts)")
}

func utf16Base64(s string) string {
	u := utf16.Encode([]rune(s))
	buf := make([]byte, len(u)*2)
	for i, c := range u {
		binary.LittleEndian.PutUint16(buf[i*2:], c)
	}
	return base64.StdEncoding.EncodeToString(buf)
}

// ---------- shared name cleanup ----------

func sanitizeOneLine(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

// sanitizeAppName makes a safe macOS .app bundle directory name.
func sanitizeAppName(s string) string {
	s = sanitizeOneLine(s)
	return strings.NewReplacer("/", "-", ":", "-", "\x00", "").Replace(s)
}

// sanitizeWinName makes a safe Windows shortcut file name.
func sanitizeWinName(s string) string {
	s = sanitizeOneLine(s)
	return strings.NewReplacer("<", "-", ">", "-", ":", "-", "/", "-", "\\", "-", "|", "-", "?", "-", "*", "-", `"`, "'", "\x00", "").Replace(s)
}

func launcherSuffix(cfg *Config) string {
	if cfg.Loader == "vanilla" {
		return ""
	}
	return " (" + cfg.Loader + ")"
}
