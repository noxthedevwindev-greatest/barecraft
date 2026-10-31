package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// installLoader runs the loader's official installer inside the shared root
// and returns the id of the version json it created.
func installLoader(root string, cfg *Config, java javaInfo) (string, error) {
	if cfg.Loader == "vanilla" {
		return "", nil
	}
	ensureProfiles(root)
	before := versionDirs(root)

	var err error
	switch cfg.Loader {
	case "fabric":
		err = installFabric(root, cfg, java)
	case "forge":
		err = installForge(root, cfg, java)
	case "neoforge":
		err = installNeoForge(root, cfg, java)
	default:
		err = fmt.Errorf("unknown loader %q", cfg.Loader)
	}
	if err != nil {
		return "", err
	}

	// find what the installer created
	after := versionDirs(root)
	var fresh []string
	for k := range after {
		if !before[k] {
			fresh = append(fresh, k)
		}
	}
	sort.Strings(fresh)
	if len(fresh) == 0 {
		// Re-run on an already-installed loader: nothing new appeared, so
		// reuse the existing version that inherits from our MC version.
		var reused []string
		for k := range after {
			v, err := loadVersionJSON(root, k)
			if err != nil || v.InheritsFrom != cfg.MC {
				continue
			}
			if !loaderMatches(cfg.Loader, k) {
				continue
			}
			reused = append(reused, k)
		}
		sort.Strings(reused)
		if len(reused) > 0 {
			pick := reused[len(reused)-1]
			info("reusing existing %s install: %s", cfg.Loader, pick)
			return pick, nil
		}
		return "", fmt.Errorf("the %s installer reported success but wrote no new version into %s/versions",
			cfg.Loader, root)
	}
	// prefer one that inherits from our minecraft version
	pick := fresh[0]
	for _, id := range fresh {
		if v, err := loadVersionJSON(root, id); err == nil && v.InheritsFrom == cfg.MC {
			pick = id
			break
		}
	}
	return pick, nil
}

func versionDirs(root string) map[string]bool {
	out := map[string]bool{}
	ents, err := os.ReadDir(filepath.Join(root, "versions"))
	if err != nil {
		return out
	}
	for _, e := range ents {
		if e.IsDir() {
			out[e.Name()] = true
		}
	}
	return out
}

// ensureProfiles gives installers a valid launcher_profiles.json to hook into.
func ensureProfiles(root string) {
	p := filepath.Join(root, "launcher_profiles.json")
	if _, err := os.Stat(p); err == nil {
		return
	}
	os.MkdirAll(root, 0o755)
	os.WriteFile(p, []byte(`{"profiles":{},"version":3,"settings":{}}`), 0o644)
}

// ---------- Fabric ----------

func installFabric(root string, cfg *Config, java javaInfo) error {
	loaderVer := strings.TrimSpace(cfg.LoaderV)
	if loaderVer == "" {
		var entries []struct {
			Version string `json:"version"`
			Stable  *bool  `json:"stable"`
		}
		if err := getJSON("https://meta.fabricmc.net/v2/versions/loader", &entries); err != nil {
			return fmt.Errorf("query Fabric loader versions: %w", err)
		}
		for _, e := range entries {
			if e.Stable != nil && *e.Stable {
				loaderVer = e.Version
				break
			}
		}
		if loaderVer == "" && len(entries) > 0 {
			loaderVer = entries[0].Version
		}
		if loaderVer == "" {
			return fmt.Errorf("no Fabric loader versions available")
		}
	}
	info("Fabric loader %s for Minecraft %s", loaderVer, cfg.MC)

	// installer version from maven metadata
	metaXML, err := getBytes("https://maven.fabricmc.net/net/fabricmc/fabric-installer/maven-metadata.xml")
	if err != nil {
		return fmt.Errorf("query Fabric installer: %w", err)
	}
	m := regexp.MustCompile(`<release>([^<]+)</release>`).FindSubmatch(metaXML)
	if m == nil {
		return fmt.Errorf("cannot read Fabric installer release from maven metadata")
	}
	instVer := string(m[1])
	jar := filepath.Join(root, "cache", fmt.Sprintf("fabric-installer-%s.jar", instVer))
	instURL := fmt.Sprintf("https://maven.fabricmc.net/net/fabricmc/fabric-installer/%s/fabric-installer-%s.jar", instVer, instVer)
	if err := downloadFile(instURL, jar, 0, "", ""); err != nil {
		return fmt.Errorf("fetch Fabric installer: %w", err)
	}
	out, err := run(java.Path, "-jar", jar, "client", "-dir", root, "-mcversion", cfg.MC, "-loader", loaderVer)
	if err != nil {
		return fmt.Errorf("Fabric installer failed: %w", err)
	}
	_ = out
	return nil
}

// ---------- Forge ----------

func installForge(root string, cfg *Config, java javaInfo) error {
	full, err := forgeVersionFor(cfg)
	if err != nil {
		return err
	}
	info("Forge %s for Minecraft %s", full, cfg.MC)
	url := fmt.Sprintf("https://maven.minecraftforge.net/net/minecraftforge/forge/%s/forge-%s-installer.jar", full, full)
	jar := filepath.Join(root, "cache", fmt.Sprintf("forge-%s-installer.jar", full))
	if err := downloadFile(url, jar, 0, "", ""); err != nil {
		return fmt.Errorf("fetch Forge installer (does Forge support %s yet?): %w", cfg.MC, err)
	}
	if _, err := run(java.Path, "-jar", jar, "--installClient", root); err != nil {
		return fmt.Errorf("Forge installer failed: %w", err)
	}
	return nil
}

func forgeVersionFor(cfg *Config) (string, error) {
	if v := strings.TrimSpace(cfg.LoaderV); v != "" {
		if strings.HasPrefix(v, cfg.MC+"-") {
			return v, nil
		}
		return cfg.MC + "-" + v, nil
	}
	// promotions first (recommended, then latest)
	var promo struct {
		Promos map[string]string `json:"promos"`
	}
	if err := getJSON("https://files.minecraftforge.net/net/minecraftforge/forge/promotions_slim.json", &promo); err == nil {
		for _, key := range []string{cfg.MC + "-recommended", cfg.MC + "-latest"} {
			if v := promo.Promos[key]; v != "" {
				return cfg.MC + "-" + v, nil
			}
		}
	}
	// fall back to maven metadata
	x, err := getBytes("https://maven.minecraftforge.net/net/minecraftforge/forge/maven-metadata.xml")
	if err != nil {
		return "", fmt.Errorf("query Forge versions: %w", err)
	}
	return pickMavenVersion(x, cfg.MC+"-")
}

// pickMavenVersion picks the highest <prefix>-X.Y.Z version from maven metadata.
func pickMavenVersion(metaXML []byte, prefix string) (string, error) {
	re := regexp.MustCompile(`<version>([^<]+)</version>`)
	var best string
	for _, m := range re.FindAllSubmatch(metaXML, -1) {
		v := string(m[1])
		if !strings.HasPrefix(v, prefix) {
			continue
		}
		if best == "" || cmpVer(strings.TrimPrefix(v, prefix), strings.TrimPrefix(best, prefix)) > 0 {
			best = v
		}
	}
	if best == "" {
		return "", fmt.Errorf("no version starting with %q in maven metadata", prefix)
	}
	return best, nil
}

// ---------- NeoForge ----------

func installNeoForge(root string, cfg *Config, java javaInfo) error {
	nv, err := neoForgeVersionFor(cfg)
	if err != nil {
		return err
	}
	info("NeoForge %s for Minecraft %s", nv, cfg.MC)
	url := fmt.Sprintf("https://maven.neoforged.net/releases/net/neoforged/neoforge/%s/neoforge-%s-installer.jar", nv, nv)
	jar := filepath.Join(root, "cache", fmt.Sprintf("neoforge-%s-installer.jar", nv))
	if err := downloadFile(url, jar, 0, "", ""); err != nil {
		return fmt.Errorf("fetch NeoForge installer: %w", err)
	}
	if _, err := run(java.Path, "-jar", jar, "--installClient", root); err != nil {
		return fmt.Errorf("NeoForge installer failed: %w", err)
	}
	return nil
}

// neoforgeMCs maps a NeoForge version to the Minecraft version(s) it targets.
// Old scheme: 21.1.177 -> 1.21.1 (major 19..21 means 1.<major>.<minor>).
// New scheme: 26.3.0.97 -> 26.3   (major 22+ is the year-based version).
func neoforgeMCs(v string) []string {
	segs := strings.Split(v, ".")
	if len(segs) < 2 {
		return nil
	}
	a, err1 := strconv.Atoi(segs[0])
	b, err2 := strconv.Atoi(segs[1])
	if err1 != nil || err2 != nil {
		return nil
	}
	switch {
	case a >= 19 && a <= 21:
		out := []string{fmt.Sprintf("1.%d.%d", a, b)}
		if b == 0 {
			out = append(out, fmt.Sprintf("1.%d", a))
		}
		return out
	case a >= 22:
		out := []string{fmt.Sprintf("%d.%d", a, b)}
		if b == 0 {
			out = append(out, fmt.Sprintf("%d", a))
		}
		return out
	}
	return nil
}

func neoForgeVersionFor(cfg *Config) (string, error) {
	x, err := getBytes("https://maven.neoforged.net/releases/net/neoforged/neoforge/maven-metadata.xml")
	if err != nil {
		return "", fmt.Errorf("query NeoForge versions: %w", err)
	}
	re := regexp.MustCompile(`<version>([^<]+)</version>`)
	var matches []string
	for _, m := range re.FindAllSubmatch(x, -1) {
		v := string(m[1])
		for _, mc := range neoforgeMCs(v) {
			if mc == cfg.MC {
				matches = append(matches, v)
				break
			}
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("NeoForge has no build for Minecraft %s (it supports 1.20.2 and newer, and 22+)", cfg.MC)
	}
	if want := strings.TrimSpace(cfg.LoaderV); want != "" {
		var hit string
		for _, v := range matches {
			if v == want || strings.HasPrefix(v, want+".") || strings.HasPrefix(v, want+"-") {
				hit = v
				break
			}
		}
		if hit == "" {
			return "", fmt.Errorf("NeoForge version %q does not exist for Minecraft %s", want, cfg.MC)
		}
		return hit, nil
	}
	sort.Slice(matches, func(i, j int) bool { return cmpVer(matches[i], matches[j]) > 0 })
	return matches[0], nil
}

// cmpVer compares dotted version strings numerically per segment.
func cmpVer(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var av, bv string
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		an, aerr := strconv.Atoi(av)
		bn, berr := strconv.Atoi(bv)
		switch {
		case aerr == nil && berr == nil:
			if an != bn {
				if an < bn {
					return -1
				}
				return 1
			}
		case aerr == nil:
			return 1 // numeric sorts above non-numeric
		case berr == nil:
			return -1
		default:
			if av != bv {
				if av < bv {
					return -1
				}
				return 1
			}
		}
	}
	return 0
}

func loaderMatches(loader, id string) bool {
	l := strings.ToLower(id)
	switch loader {
	case "fabric":
		return strings.Contains(l, "fabric")
	case "forge":
		return strings.Contains(l, "forge") && !strings.Contains(l, "neoforge")
	case "neoforge":
		return strings.Contains(l, "neoforge")
	}
	return false
}
