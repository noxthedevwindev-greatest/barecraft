package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func main() {
	cfg, err := parseConfig(os.Args[1:])
	if err != nil {
		if err == errNoOverlay && len(os.Args) == 1 {
			usage()
			os.Exit(2)
		}
		fatal(err)
	}
	if cfgFromOverlay {
		fmt.Println("Barecraft — setting up: " + cfg.Name)
	}
	if err := setup(cfg); err != nil {
		fatal(err)
	}
	promptExit()
}

func parseConfig(args []string) (*Config, error) {
	if len(args) > 0 {
		a := args[0]
		if (a == "setup" || a == "install") && len(args) > 1 {
			a = args[1]
		}
		switch a {
		case "-h", "--help", "help":
			usage()
			os.Exit(0)
		}
		return decodeCfg(a)
	}
	cfg, err := cfgFromOwnBinary()
	if err == nil {
		cfgFromOverlay = true
		return cfg, nil
	}
	return nil, errNoOverlay
}

func usage() {
	fmt.Println(`Barecraft — no launcher, just play.

usage:
  barecraft <config>       run setup with a base64url config
  barecraft setup <config> same thing, explicit
  <generated setup file>   double-click the file you downloaded

make a config on the Barecraft website, then either paste the
terminal line or run the downloaded file.`)
}

func setup(cfg *Config) error {
	root := rootDir()
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	dry := os.Getenv("BARECRAFT_DRY_RUN") != ""
	const total = 7
	slug := cfg.Slug()

	// 1. version metadata
	step(1, total, "Reading Minecraft %s", cfg.MC)
	m, err := loadManifest(root)
	if err != nil {
		return err
	}
	mv, ok := m.find(cfg.MC)
	if !ok {
		return fmt.Errorf("Minecraft %s does not exist — pick a version from the list", cfg.MC)
	}
	info("version type: %s", mv.Type)
	v, jarID, err := resolveVersion(root, cfg.MC)
	if err != nil {
		return err
	}
	info("main class: %s", v.MainClass)

	// 2. java
	step(2, total, "Resolving Java")
	req := 8
	if v.JavaVersion != nil && v.JavaVersion.MajorVersion > 0 {
		req = v.JavaVersion.MajorVersion
	}
	info("needs Java %d", req)
	java, err := pickJava(root, req)
	if err != nil {
		return err
	}
	info("using %s (Java %d)", java.Path, java.Major)

	// 3. loader
	step(3, total, "Loader: %s", cfg.Loader)
	launchID := cfg.MC
	if cfg.Loader != "vanilla" {
		launchID, err = installLoader(root, cfg, java)
		if err != nil {
			return err
		}
		info("installed version id: %s", launchID)
	} else {
		info("vanilla — nothing to install")
	}
	v, jarID, err = resolveVersion(root, launchID)
	if err != nil {
		return err
	}

	// 4. game files
	step(4, total, "Downloading game files")
	nativesDir := filepath.Join(root, "natives", slugify(v.ID))
	if err := downloadGameFiles(root, v, jarID, nativesDir, dry); err != nil {
		return err
	}

	// 5. instance + mods
	step(5, total, "Instance: %s", slug)
	instance := filepath.Join(root, "instances", slug)
	if err := os.MkdirAll(instance, 0o755); err != nil {
		return err
	}
	if wipe, err := configChanged(instance, cfg); err == nil && wipe {
		// stale jars from a previous config would silently keep loading
		modsDir := filepath.Join(instance, "mods")
		if ents, err := os.ReadDir(modsDir); err == nil {
			for _, e := range ents {
				if strings.HasSuffix(strings.ToLower(e.Name()), ".jar") {
					os.Remove(filepath.Join(modsDir, e.Name()))
				}
			}
		}
		info("config changed since last run: removed old mod jars")
	}
	var iconURL string
	if len(cfg.Mods) > 0 {
		info("installing %d mod(s) from Modrinth", len(cfg.Mods))
		iconURL, err = installMods(root, cfg, instance)
		if err != nil {
			return err
		}
	} else {
		info("no mods")
	}

	// 6. launch script
	step(6, total, "Writing launch script")
	spec, err := buildLaunchSpec(root, instance, cfg, v, jarID, java)
	if err != nil {
		return err
	}
	if err := writeLaunchSh(filepath.Join(instance, "launch.sh"), spec, cfg, instance); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		if err := writeLaunchBat(filepath.Join(instance, "launch.bat"), spec, cfg, instance); err != nil {
			return err
		}
	}
	metaOut, _ := json.MarshalIndent(map[string]any{
		"generator": "barecraft",
		"created":   time.Now().UTC().Format(time.RFC3339),
		"config":    cfg,
		"version":   v.ID,
		"jar":       jarID,
		"java":      java.Path,
		"mainClass": spec.MainClass,
	}, "", "  ")
	os.WriteFile(filepath.Join(instance, "instance.json"), metaOut, 0o644)

	// 7. icon + shortcuts
	step(7, total, "Creating shortcuts")
	pngPath, icoPath, err := ensureIcon(root, slug, iconURL)
	if err != nil {
		return err
	}
	made, err := createShortcuts(instance, slug, cfg, spec, pngPath, icoPath)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("Done — " + cfg.Name + " is ready.")
	fmt.Println("  game:     Minecraft " + cfg.MC + launcherSuffix(cfg) +
		fmt.Sprintf(", %d mod(s), %d MB RAM", len(cfg.Mods), cfg.RAM))
	fmt.Println("  files:    " + instance)
	for _, p := range made {
		fmt.Println("  shortcut: " + p)
	}
	fmt.Println("Double-click the shortcut to play.")
	return nil
}

// configChanged reports whether this instance was built from a different config.
func configChanged(instance string, cfg *Config) (bool, error) {
	prev, err := os.ReadFile(filepath.Join(instance, "instance.json"))
	if err != nil {
		return false, err
	}
	var old struct {
		Config *Config `json:"config"`
	}
	if json.Unmarshal(prev, &old) != nil || old.Config == nil {
		return false, nil
	}
	b1, _ := json.Marshal(old.Config)
	b2, _ := json.Marshal(cfg)
	return string(b1) != string(b2), nil
}
