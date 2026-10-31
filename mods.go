package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type mrProject struct {
	ID      string `json:"id"`
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	IconURL string `json:"icon_url"`
}

type mrVersion struct {
	ID    string `json:"id"`
	Num   string `json:"version_number"`
	Files []struct {
		URL      string `json:"url"`
		Filename string `json:"filename"`
		Primary  bool   `json:"primary"`
		Size     int64  `json:"size"`
		Hashes   struct {
			SHA1 string `json:"sha1"`
		} `json:"hashes"`
	} `json:"files"`
}

// installMods resolves every configured Modrinth project for this MC+loader
// and drops the jars into the instance's mods folder.
// Returns the icon URL of the first project (used for the shortcut icon).
func installMods(root string, cfg *Config, instance string) (string, error) {
	if len(cfg.Mods) == 0 {
		return "", nil
	}
	modsDir := filepath.Join(instance, "mods")
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		return "", err
	}

	q := url.Values{}
	q.Set("game_versions", jsonArray([]string{cfg.MC}))
	if cfg.Loader != "vanilla" {
		q.Set("loaders", jsonArray([]string{cfg.Loader}))
	}

	firstIcon := ""
	var failed []string
	for _, raw := range cfg.Mods {
		slug := strings.TrimSpace(raw)
		if slug == "" {
			continue
		}
		var proj mrProject
		if err := getJSON("https://api.modrinth.com/v2/project/"+url.PathEscape(slug), &proj); err != nil {
			return firstIcon, fmt.Errorf("mod %q: %w", slug, err)
		}
		if firstIcon == "" && proj.IconURL != "" {
			firstIcon = proj.IconURL
		}
		var versions []mrVersion
		vurl := fmt.Sprintf("https://api.modrinth.com/v2/project/%s/version?%s", url.PathEscape(slug), q.Encode())
		if err := getJSON(vurl, &versions); err != nil {
			return firstIcon, fmt.Errorf("mod %q: %w", slug, err)
		}
		if len(versions) == 0 {
			failed = append(failed, fmt.Sprintf("%s (no build for %s + %s)", slug, cfg.MC, cfg.Loader))
			continue
		}
		v := versions[0]
		file := v.Files[0]
		for _, f := range v.Files {
			if f.Primary {
				file = f
				break
			}
		}
		if file.Filename == "" {
			failed = append(failed, slug+" (release has no file)")
			continue
		}
		dest := filepath.Join(modsDir, file.Filename)
		if err := downloadFile(file.URL, dest, file.Size, file.Hashes.SHA1, ""); err != nil {
			return firstIcon, fmt.Errorf("mod %q download: %w", slug, err)
		}
		info("+ %s", file.Filename)
	}
	if len(failed) > 0 {
		return firstIcon, fmt.Errorf("could not resolve mods:\n         - %s", strings.Join(failed, "\n         - "))
	}
	return firstIcon, nil
}

func jsonArray(vals []string) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = `"` + v + `"`
	}
	return "[" + strings.Join(parts, ",") + "]"
}
