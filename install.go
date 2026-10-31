package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var mavenBases = []string{
	"https://libraries.minecraft.net/",
	"https://maven.fabricmc.net/",
	"https://maven.neoforged.net/",
	"https://maven.minecraftforge.net/",
	"https://repo1.maven.org/maven2/",
}

// libLocalPath is the path of a library jar relative to root/libraries.
func libLocalPath(lib Library) (string, error) {
	if lib.Downloads != nil && lib.Downloads.Artifact != nil && lib.Downloads.Artifact.Path != "" {
		return lib.Downloads.Artifact.Path, nil
	}
	return libPathFromName(lib.Name)
}

// libPathFromName derives group/artifact/version/artifact-version[-classifier].jar
// from a maven coordinate like "com.mojang:brigadier:1.0.18" or
// "org.lwjgl:lwjgl:3.3.1:natives-linux".
func libPathFromName(name string) (string, error) {
	parts := strings.Split(name, ":")
	if len(parts) < 3 {
		return "", fmt.Errorf("bad maven coordinate %q", name)
	}
	group, artifact, version := parts[0], parts[1], parts[2]
	classifier := ""
	switch len(parts) {
	case 4:
		classifier = parts[3]
	case 5:
		classifier = parts[4] // g:a:v:packaging:classifier
	}
	file := artifact + "-" + version
	if classifier != "" {
		file += "-" + classifier
	}
	file += ".jar"
	return strings.ReplaceAll(group, ".", "/") + "/" + artifact + "/" + version + "/" + file, nil
}

func libArtifact(lib Library) *Artifact {
	if lib.Downloads != nil && lib.Downloads.Artifact != nil {
		return lib.Downloads.Artifact
	}
	return nil
}

// libURLs returns candidate download URLs, best guess first.
func libURLs(lib Library, localPath string) []string {
	var out []string
	if a := libArtifact(lib); a != nil && a.URL != "" {
		out = append(out, a.URL)
	}
	if lib.URL != "" {
		base := lib.URL
		if !strings.HasSuffix(base, "/") {
			base += "/"
		}
		out = append(out, base+localPath)
	}
	for _, b := range mavenBases {
		out = append(out, b+localPath)
	}
	return out
}

// classpath returns absolute jar paths for allowed libraries, client jar last.
func classpath(root string, v *VersionJSON, jarID string) ([]string, error) {
	var out []string
	for _, lib := range v.Libraries {
		if !rulesAllow(lib.Rules) {
			continue
		}
		p, err := libLocalPath(lib)
		if err != nil {
			return nil, err
		}
		out = append(out, filepath.Join(root, "libraries", filepath.FromSlash(p)))
	}
	out = append(out, filepath.Join(root, "versions", jarID, jarID+".jar"))
	return out, nil
}

// downloadGameFiles fetches the client jar, every allowed library,
// old-style natives, and the asset objects.
func downloadGameFiles(root string, v *VersionJSON, jarID, nativesDir string, dryRun bool) error {
	// --- client jar ---
	if cd := v.Downloads["client"]; cd != nil && cd.URL != "" {
		dest := filepath.Join(root, "versions", jarID, jarID+".jar")
		if err := downloadFile(cd.URL, dest, cd.Size, cd.SHA1, ""); err != nil {
			return fmt.Errorf("client jar: %w", err)
		}
	} else if _, err := os.Stat(filepath.Join(root, "versions", jarID, jarID+".jar")); err != nil {
		return fmt.Errorf("version %s has no client jar download URL", jarID)
	}

	// --- libraries (parallel) ---
	var tasks []func() error
	libCount := 0
	for _, lib := range v.Libraries {
		if !rulesAllow(lib.Rules) {
			continue
		}
		libCount++
		lib := lib
		tasks = append(tasks, func() error {
			p, err := libLocalPath(lib)
			if err != nil {
				return err
			}
			dest := filepath.Join(root, "libraries", filepath.FromSlash(p))
			if _, err := os.Stat(dest); err == nil {
				return nil
			}
			var a *Artifact
			if lib.Downloads != nil {
				a = lib.Downloads.Artifact
			}
			var lastErr error
			for _, u := range libURLs(lib, p) {
				var size int64
				var sha1 string
				if a != nil {
					size, sha1 = a.Size, a.SHA1
				}
				if err := downloadFile(u, dest, size, sha1, ""); err != nil {
					lastErr = err
					continue
				}
				return nil
			}
			return fmt.Errorf("library %s: %w", lib.Name, lastErr)
		})
	}
	if err := runPool(fmt.Sprintf("%d libraries", libCount), tasks); err != nil {
		return err
	}

	// --- old-style natives (natives{linux:...} + downloads.classifiers) ---
	if err := extractNatives(root, v, nativesDir); err != nil {
		return err
	}
	os.MkdirAll(nativesDir, 0o755) // modern versions let LWJGL self-extract here

	// --- assets ---
	if dryRun {
		info("dry run: skipping asset download")
		return nil
	}
	if err := downloadAssets(root, v); err != nil {
		return err
	}
	return nil
}

func runPool(label string, tasks []func() error) error {
	if len(tasks) == 0 {
		return nil
	}
	p := newPool(12)
	for _, t := range tasks {
		p.add(t)
	}
	if err := p.wait(); err != nil {
		return err
	}
	return nil
}

// extractNatives downloads and unpacks jars declared via the old
// "natives": {"linux": "natives-linux"} style (Minecraft <= 1.18 era).
func extractNatives(root string, v *VersionJSON, nativesDir string) error {
	extracted := 0
	for _, lib := range v.Libraries {
		if lib.Natives == nil || !rulesAllow(lib.Rules) {
			continue
		}
		cls, ok := lib.Natives[osName()]
		if !ok {
			continue
		}
		cls = strings.ReplaceAll(cls, "${arch}", archBit())
		var art *Artifact
		if lib.Downloads != nil && lib.Downloads.Classifiers != nil {
			art = lib.Downloads.Classifiers[cls]
		}
		var localPath string
		var err error
		if art != nil && art.Path != "" {
			localPath = art.Path
		} else {
			localPath, err = libPathFromName(lib.Name + ":" + cls)
			if err != nil {
				return err
			}
		}
		jarPath := filepath.Join(root, "libraries", filepath.FromSlash(localPath))
		if art != nil {
			url := art.URL
			if url == "" {
				url = "https://libraries.minecraft.net/" + localPath
			}
			if err := downloadFile(url, jarPath, art.Size, art.SHA1, ""); err != nil {
				return fmt.Errorf("natives %s: %w", lib.Name, err)
			}
		} else {
			var last error
			for _, u := range libURLs(lib, localPath) {
				if err := downloadFile(u, jarPath, 0, "", ""); err == nil {
					last = nil
					break
				} else {
					last = err
				}
			}
			if last != nil {
				return fmt.Errorf("natives %s: %w", lib.Name, last)
			}
		}
		if err := unzipNatives(jarPath, nativesDir, lib); err != nil {
			return fmt.Errorf("extract natives %s: %w", lib.Name, err)
		}
		extracted++
	}
	if extracted > 0 {
		info("extracted natives from %d jars into %s", extracted, nativesDir)
	}
	return nil
}

func unzipNatives(jarPath, dest string, lib Library) error {
	r, err := zip.OpenReader(jarPath)
	if err != nil {
		return err
	}
	defer r.Close()
	var excludes []string
	if lib.Extract != nil {
		excludes = lib.Extract.Exclude
	}
	for _, f := range r.File {
		name := f.Name
		if strings.HasPrefix(name, "META-INF/") {
			continue
		}
		skip := false
		for _, ex := range excludes {
			if ex != "" && strings.HasPrefix(name, ex) {
				skip = true
				break
			}
		}
		if skip || f.FileInfo().IsDir() || strings.Contains(name, "..") {
			continue
		}
		target := filepath.Join(dest, filepath.FromSlash(name))
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			rc.Close()
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func archBit() string {
	if runtime.GOARCH == "386" || runtime.GOARCH == "arm" {
		return "32"
	}
	return "64"
}

type assetObject struct {
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

func downloadAssets(root string, v *VersionJSON) error {
	if v.AssetIndex == nil || v.AssetIndex.URL == "" {
		return fmt.Errorf("version has no asset index URL")
	}
	id := v.AssetIndex.ID
	if id == "" {
		id = v.Assets
	}
	if id == "" {
		return fmt.Errorf("version has no asset index id")
	}
	idxPath := filepath.Join(root, "assets", "indexes", id+".json")
	if err := downloadFile(v.AssetIndex.URL, idxPath, v.AssetIndex.Size, v.AssetIndex.SHA1, ""); err != nil {
		return fmt.Errorf("asset index: %w", err)
	}
	b, err := os.ReadFile(idxPath)
	if err != nil {
		return err
	}
	var idx struct {
		Objects map[string]assetObject `json:"objects"`
	}
	if err := json.Unmarshal(b, &idx); err != nil {
		return fmt.Errorf("parse asset index: %w", err)
	}
	var tasks []func() error
	for _, o := range idx.Objects {
		o := o
		if len(o.Hash) != 40 {
			continue
		}
		dest := filepath.Join(root, "assets", "objects", o.Hash[:2], o.Hash)
		tasks = append(tasks, func() error {
			return downloadFile("https://resources.download.minecraft.net/"+o.Hash[:2]+"/"+o.Hash, dest, o.Size, "", "")
		})
	}
	return runPool(fmt.Sprintf("%d assets", len(tasks)), tasks)
}
