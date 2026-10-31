package main

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// launchSpec is a fully resolved, ready-to-run game launch command.
type launchSpec struct {
	Java      javaInfo
	JVMArgs   []string
	MainClass string
	GameArgs  []string
	Natives   string
	Classpath []string
}

var placeholderRe = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)\}`)

func buildLaunchSpec(root, instance string, cfg *Config, v *VersionJSON, jarID string, java javaInfo) (*launchSpec, error) {
	if v.MainClass == "" {
		return nil, fmt.Errorf("version %s has no mainClass after merging", v.ID)
	}
	cp, err := classpath(root, v, jarID)
	if err != nil {
		return nil, err
	}
	nativesDir := filepath.Join(root, "natives", slugify(v.ID))
	if err := os.MkdirAll(nativesDir, 0o755); err != nil {
		return nil, err
	}

	indexID := ""
	if v.AssetIndex != nil && v.AssetIndex.ID != "" {
		indexID = v.AssetIndex.ID
	} else {
		indexID = v.Assets
	}

	subs := map[string]string{
		"auth_player_name":      cfg.Player,
		"version_name":          v.ID,
		"game_directory":        instance,
		"assets_root":           filepath.Join(root, "assets"),
		"assets_index_name":     indexID,
		"asset_index":           indexID,
		"auth_uuid":             offlineUUID(cfg.Player),
		"auth_access_token":     "0",
		"clientid":              "",
		"auth_xuid":             "",
		"user_type":             "msa",
		"version_type":          v.Type,
		"natives_directory":     nativesDir,
		"launcher_name":         "barecraft",
		"launcher_version":      "1.0.0",
		"classpath":             strings.Join(cp, string(os.PathListSeparator)),
		"classpath_separator":   string(os.PathListSeparator),
		"library_directory":     filepath.Join(root, "libraries"),
		"resolution_width":      "1280",
		"resolution_height":     "720",
		"quickPlayPath":         filepath.Join(instance, "quickPlay.json"),
		"quickPlaySingleplayer": "",
		"quickPlayMultiplayer":  "",
		"quickPlayRealms":       "",
	}
	unknown := map[string]bool{}
	sub := func(s string) string {
		return placeholderRe.ReplaceAllStringFunc(s, func(m string) string {
			name := m[2 : len(m)-1]
			val, ok := subs[name]
			if !ok {
				unknown[name] = true
				return ""
			}
			return val
		})
	}

	// JVM args: from the version json, or a sensible default for old versions.
	jvmTmpl := defaultJVMTmpl
	if v.Arguments != nil && len(v.Arguments.JVM) > 0 {
		jvmTmpl = v.Arguments.JVM
	}
	jvm := renderArgs(jvmTmpl, sub)
	jvm = append([]string{fmt.Sprintf("-Xmx%dM", cfg.RAM)}, jvm...)

	// Game args: modern arguments.game, or the legacy minecraftArguments string.
	var game []string
	switch {
	case v.Arguments != nil && len(v.Arguments.Game) > 0:
		game = renderArgs(v.Arguments.Game, sub)
	case v.MinecraftArguments != "":
		for _, t := range tokenize(v.MinecraftArguments) {
			game = append(game, sub(t))
		}
	}
	if len(game) == 0 {
		return nil, fmt.Errorf("version %s produced no game arguments", v.ID)
	}
	if len(unknown) > 0 {
		var names []string
		for n := range unknown {
			names = append(names, n)
		}
		info("note: no value for placeholder(s) %s — passed empty", strings.Join(names, ", "))
	}

	return &launchSpec{
		Java:      java,
		JVMArgs:   jvm,
		MainClass: v.MainClass,
		GameArgs:  game,
		Natives:   nativesDir,
		Classpath: cp,
	}, nil
}

var defaultJVMTmpl = []argVal{
	{Vals: []string{"-Djava.library.path=${natives_directory}"}},
	{Vals: []string{"-Dminecraft.launcher.brand=${launcher_name}"}},
	{Vals: []string{"-Dminecraft.launcher.version=${launcher_version}"}},
	{Vals: []string{"-cp", "${classpath}"}},
}

func renderArgs(args []argVal, sub func(string) string) []string {
	var out []string
	for _, a := range args {
		if !rulesAllow(a.Rules) {
			continue
		}
		for _, val := range a.Vals {
			out = append(out, sub(val))
		}
	}
	return out
}

// tokenize splits a legacy minecraftArguments string, honouring double quotes.
func tokenize(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case (r == ' ' || r == '\t') && !inQuote:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func offlineUUID(name string) string {
	sum := md5.Sum([]byte("OfflinePlayer:" + name))
	sum[6] = (sum[6] & 0x0f) | 0x30
	sum[8] = (sum[8] & 0x3f) | 0x80
	h := hex.EncodeToString(sum[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// ---------- bake launch scripts ----------

// writeLaunchSh writes a self-contained /bin/sh launcher (Linux + macOS).
func writeLaunchSh(path string, spec *launchSpec, cfg *Config, instance string) error {
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	b.WriteString("# Barecraft launch script — " + cfg.Name + "\n")
	b.WriteString("# Minecraft " + cfg.MC)
	if cfg.Loader != "vanilla" {
		b.WriteString(" + " + cfg.Loader)
	}
	b.WriteString(" — everything is resolved; this just runs the game.\n")
	b.WriteString("cd " + shquote(instance) + " || exit 1\n")
	b.WriteString("exec " + shquote(spec.Java.Path))
	for _, a := range spec.JVMArgs {
		b.WriteString(" " + shquote(a))
	}
	b.WriteString(" " + shquote(spec.MainClass))
	for _, a := range spec.GameArgs {
		b.WriteString(" " + shquote(a))
	}
	b.WriteString(` "$@"` + "\n")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o755)
}

// specCommandLine joins a command for Windows (lnk/bat).
// bat=true doubles % for cmd.exe expansion safety.
func specCommandLine(spec *launchSpec, bat bool) string {
	var parts []string
	q := winQuote
	if bat {
		q = batQuote
	}
	parts = append(parts, q(spec.Java.Path))
	for _, a := range spec.JVMArgs {
		parts = append(parts, q(a))
	}
	parts = append(parts, q(spec.MainClass))
	for _, a := range spec.GameArgs {
		parts = append(parts, q(a))
	}
	return strings.Join(parts, " ")
}

// winQuote is batQuote without % doubling (for .lnk Arguments, no cmd involved).
func winQuote(s string) string {
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
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// writeLaunchBat writes a console launcher for Windows (cmd).
// Long classpaths switch to a Java @argfile (Java 9+) to dodge cmd's 8191-char limit.
func writeLaunchBat(batPath string, spec *launchSpec, cfg *Config, instance string) error {
	line := specCommandLine(spec, true)
	if len(line) > 7000 && spec.Java.Major >= 9 {
		argPath := filepath.Join(instance, "launch.args")
		if err := os.WriteFile(argPath, []byte(javaArgFile(spec)+"\n"), 0o644); err != nil {
			return err
		}
		script := "@echo off\r\n" +
			"rem Barecraft launch script (console) - " + sanitizeOneLine(cfg.Name) + "\r\n" +
			"cd /d \"%~dp0\"\r\n" +
			"\"" + spec.Java.Path + "\" @launch.args %*\r\n"
		return os.WriteFile(batPath, []byte(script), 0o755)
	}

	var b strings.Builder
	b.WriteString("@echo off\r\n")
	b.WriteString("rem Barecraft launch script (console) - " + sanitizeOneLine(cfg.Name) + "\r\n")
	b.WriteString("setlocal\r\n")
	b.WriteString("cd /d \"%~dp0\"\r\n")
	b.WriteString(batBody(spec))
	return os.WriteFile(batPath, []byte(b.String()), 0o755)
}

func batBody(spec *launchSpec) string {
	var lines []string
	emit := func(t string) { lines = append(lines, "  "+t) }
	emit(batQuote(spec.Java.Path) + " ^")
	for _, a := range spec.JVMArgs {
		emit(batQuote(a) + " ^")
	}
	emit(batQuote(spec.MainClass) + " ^")
	for i, a := range spec.GameArgs {
		if i == len(spec.GameArgs)-1 {
			emit(batQuote(a))
		} else {
			emit(batQuote(a) + " ^")
		}
	}
	return strings.Join(lines, "^\r\n") + "\r\n"
}

// javaArgFile renders the Java 9+ @argument-file format (forward slashes,
// quoted where needed) so cmd never has to carry the whole line.
func javaArgFile(spec *launchSpec) string {
	var toks []string
	all := append([]string{}, spec.JVMArgs...)
	all = append(all, spec.MainClass)
	all = append(all, spec.GameArgs...)
	for _, t := range all {
		fwd := strings.ReplaceAll(t, `\`, "/")
		if strings.ContainsAny(fwd, " \t\"") {
			fwd = `"` + strings.ReplaceAll(fwd, `"`, `\"`) + `"`
		}
		toks = append(toks, fwd)
	}
	return strings.Join(toks, "\r\n")
}
