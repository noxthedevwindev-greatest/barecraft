package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ---------- Mojang version manifest ----------

type manifestVersion struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	URL  string `json:"url"`
}

type manifest struct {
	Latest struct {
		Release  string `json:"release"`
		Snapshot string `json:"snapshot"`
	} `json:"latest"`
	Versions []manifestVersion `json:"versions"`
}

func loadManifest(root string) (*manifest, error) {
	const url = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"
	cache := filepath.Join(root, "cache", "version_manifest.json")
	var m manifest
	if b, err := getBytes(url); err == nil && json.Unmarshal(b, &m) == nil && len(m.Versions) > 0 {
		os.MkdirAll(filepath.Dir(cache), 0o755)
		os.WriteFile(cache, b, 0o644)
		return &m, nil
	}
	if b, err := os.ReadFile(cache); err == nil && json.Unmarshal(b, &m) == nil && len(m.Versions) > 0 {
		info("version manifest: using cache (%d versions)", len(m.Versions))
		return &m, nil
	}
	return nil, fmt.Errorf("cannot fetch the Minecraft version manifest and no cache exists")
}

func (m *manifest) find(id string) (manifestVersion, bool) {
	for _, v := range m.Versions {
		if v.ID == id {
			return v, true
		}
	}
	return manifestVersion{}, false
}

// ---------- version json ----------

type Artifact struct {
	ID   string `json:"id,omitempty"`
	Path string `json:"path,omitempty"`
	SHA1 string `json:"sha1,omitempty"`
	Size int64  `json:"size,omitempty"`
	URL  string `json:"url,omitempty"`
}

type LibDownloads struct {
	Artifact    *Artifact            `json:"artifact,omitempty"`
	Classifiers map[string]*Artifact `json:"classifiers,omitempty"`
}

type Library struct {
	Name      string            `json:"name"`
	Downloads *LibDownloads     `json:"downloads,omitempty"`
	Rules     []Rule            `json:"rules,omitempty"`
	Natives   map[string]string `json:"natives,omitempty"`
	Extract   *struct {
		Exclude []string `json:"exclude,omitempty"`
	} `json:"extract,omitempty"`
	URL string `json:"url,omitempty"`
}

type Rule struct {
	Action   string          `json:"action"`
	OS       *ruleOS         `json:"os,omitempty"`
	Features map[string]bool `json:"features,omitempty"`
}

type ruleOS struct {
	Name string `json:"name,omitempty"`
	Arch string `json:"arch,omitempty"`
}

// argVal is a version-json argument entry: either "string" or {rules, value}.
type argVal struct {
	Rules []Rule   `json:"rules,omitempty"`
	Vals  []string `json:"-"`
}

func (a *argVal) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		a.Vals = []string{s}
		return nil
	}
	var o struct {
		Rules []Rule          `json:"rules"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(b, &o); err != nil {
		return err
	}
	a.Rules = o.Rules
	if len(o.Value) > 0 {
		if o.Value[0] == '"' {
			var s string
			if err := json.Unmarshal(o.Value, &s); err != nil {
				return err
			}
			a.Vals = []string{s}
			return nil
		}
		var ss []string
		if err := json.Unmarshal(o.Value, &ss); err != nil {
			return err
		}
		a.Vals = ss
	}
	return nil
}

func (a argVal) MarshalJSON() ([]byte, error) {
	o := map[string]any{}
	if len(a.Rules) > 0 {
		o["rules"] = a.Rules
	}
	if len(a.Vals) == 1 {
		o["value"] = a.Vals[0]
	} else {
		o["value"] = a.Vals
	}
	return json.Marshal(o)
}

type VersionJSON struct {
	ID                 string               `json:"id"`
	InheritsFrom       string               `json:"inheritsFrom,omitempty"`
	Jar                string               `json:"jar,omitempty"`
	Type               string               `json:"type,omitempty"`
	MainClass          string               `json:"mainClass,omitempty"`
	MinecraftArguments string               `json:"minecraftArguments,omitempty"`
	Arguments          *versionArgs         `json:"arguments,omitempty"`
	Assets             string               `json:"assets,omitempty"`
	AssetIndex         *Artifact            `json:"assetIndex,omitempty"`
	Downloads          map[string]*Artifact `json:"downloads,omitempty"`
	JavaVersion        *struct {
		Component    string `json:"component,omitempty"`
		MajorVersion int    `json:"majorVersion"`
	} `json:"javaVersion,omitempty"`
	Libraries []Library `json:"libraries,omitempty"`
}

type versionArgs struct {
	Game []argVal `json:"game,omitempty"`
	JVM  []argVal `json:"jvm,omitempty"`
}

func loadVersionJSON(root, id string) (*VersionJSON, error) {
	p := filepath.Join(root, "versions", id, id+".json")
	if b, err := os.ReadFile(p); err == nil {
		var v VersionJSON
		if err := json.Unmarshal(b, &v); err != nil {
			return nil, fmt.Errorf("parse %s: %w", p, err)
		}
		return &v, nil
	}
	// not on disk: fetch vanilla from the manifest
	m, err := loadManifest(root)
	if err != nil {
		return nil, err
	}
	mv, ok := m.find(id)
	if !ok {
		return nil, fmt.Errorf("version %q not found in the Minecraft manifest", id)
	}
	b, err := getBytes(mv.URL)
	if err != nil {
		return nil, fmt.Errorf("fetch version %s: %w", id, err)
	}
	var v VersionJSON
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("parse version %s: %w", id, err)
	}
	if err := os.MkdirAll(filepath.Join(root, "versions", id), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		return nil, err
	}
	return &v, nil
}

// resolveVersion walks the inheritsFrom chain and merges child over parent.
// Returns the merged json plus the id of the vanilla jar to use.
func resolveVersion(root, id string) (*VersionJSON, string, error) {
	chain := []*VersionJSON{}
	seen := map[string]bool{}
	cur := id
	for i := 0; ; i++ {
		if seen[cur] {
			return nil, "", fmt.Errorf("version inheritance loop at %q", cur)
		}
		if i > 10 {
			return nil, "", fmt.Errorf("version inheritance chain too deep at %q", cur)
		}
		seen[cur] = true
		v, err := loadVersionJSON(root, cur)
		if err != nil {
			return nil, "", err
		}
		chain = append(chain, v)
		if v.InheritsFrom == "" {
			break
		}
		cur = v.InheritsFrom
	}

	// jar id: nearest explicit .jar, else the root of the chain
	jarID := chain[len(chain)-1].ID
	for _, v := range chain {
		if v.Jar != "" {
			jarID = v.Jar
			break
		}
	}

	merged := *chain[len(chain)-1] // start from the vanilla root
	for i := len(chain) - 2; i >= 0; i-- {
		overlayVersion(&merged, chain[i])
	}
	return &merged, jarID, nil
}

func overlayVersion(base, child *VersionJSON) {
	if child.ID != "" {
		base.ID = child.ID
	}
	if child.Type != "" {
		base.Type = child.Type
	}
	if child.MainClass != "" {
		base.MainClass = child.MainClass
	}
	if child.MinecraftArguments != "" {
		base.MinecraftArguments = child.MinecraftArguments
	}
	if child.Jar != "" {
		base.Jar = child.Jar
	}
	if child.Assets != "" {
		base.Assets = child.Assets
	}
	if child.AssetIndex != nil {
		base.AssetIndex = child.AssetIndex
	}
	if child.JavaVersion != nil {
		base.JavaVersion = child.JavaVersion
	}
	if child.Downloads != nil {
		if base.Downloads == nil {
			base.Downloads = map[string]*Artifact{}
		}
		for k, v := range child.Downloads {
			base.Downloads[k] = v
		}
	}
	if child.Arguments != nil {
		if base.Arguments == nil {
			base.Arguments = &versionArgs{}
		}
		base.Arguments.Game = concatArgs(base.Arguments.Game, child.Arguments.Game)
		base.Arguments.JVM = concatArgs(base.Arguments.JVM, child.Arguments.JVM)
	}
	base.Libraries = mergeLibs(base.Libraries, child.Libraries)
}

func concatArgs(parent, child []argVal) []argVal {
	if len(child) == 0 {
		return parent
	}
	seen := map[string]bool{}
	out := append([]argVal{}, parent...)
	for _, a := range parent {
		if k, err := json.Marshal(a); err == nil {
			seen[string(k)] = true
		}
	}
	for _, a := range child {
		k, err := json.Marshal(a)
		if err == nil && seen[string(k)] {
			continue
		}
		if err == nil {
			seen[string(k)] = true
		}
		out = append(out, a)
	}
	return out
}

func mergeLibs(parent, child []Library) []Library {
	if len(child) == 0 {
		return parent
	}
	out := append([]Library{}, parent...)
	idx := map[string]int{}
	for i, l := range out {
		idx[l.Name] = i
	}
	for _, l := range child {
		if i, ok := idx[l.Name]; ok {
			out[i] = l // child overrides same coordinate
		} else {
			idx[l.Name] = len(out)
			out = append(out, l)
		}
	}
	return out
}

// ---------- rules ----------

var launcherFeatures = map[string]bool{
	"is_demo_user":               false,
	"has_custom_resolution":      false,
	"has_quick_plays_support":    false,
	"is_quick_play_singleplayer": false,
	"is_quick_play_multiplayer":  false,
}

func ruleMatches(r Rule) bool {
	if r.OS != nil {
		if r.OS.Name != "" && r.OS.Name != osName() {
			return false
		}
		if r.OS.Arch != "" && r.OS.Arch != mojangArch() {
			return false
		}
	}
	for k, want := range r.Features {
		if launcherFeatures[k] != want {
			return false
		}
	}
	return true
}

// rulesAllow: rules present => default deny, last matching rule wins.
func rulesAllow(rules []Rule) bool {
	if len(rules) == 0 {
		return true
	}
	allowed := false
	for _, r := range rules {
		if ruleMatches(r) {
			allowed = r.Action == "allow"
		}
	}
	return allowed
}
