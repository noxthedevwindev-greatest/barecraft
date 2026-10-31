package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"strings"
)

// Config is the compact JSON shape the website generates.
// Short keys keep the terminal link small.
type Config struct {
	Name    string   `json:"n,omitempty"`    // shortcut display name
	Player  string   `json:"p,omitempty"`    // in-game player name
	MC      string   `json:"mc"`             // minecraft version id
	Loader  string   `json:"ld"`             // vanilla | fabric | forge | neoforge
	LoaderV string   `json:"lv,omitempty"`   // loader version, empty = latest/recommended
	Mods    []string `json:"mods,omitempty"` // modrinth project slugs
	RAM     int      `json:"ram,omitempty"`  // max heap in MB
}

const overlayMagic = "BRCFG1\x00"

var (
	cfgFromOverlay bool
	errNoOverlay   = errors.New("no config overlay")
)

func (c *Config) applyDefaults() {
	if c.Player == "" {
		c.Player = "Player"
	}
	if c.RAM <= 0 {
		c.RAM = 4096
	}
	if c.Loader == "" {
		c.Loader = "vanilla"
	}
	if c.Name == "" {
		if c.Loader == "vanilla" {
			c.Name = "Minecraft " + c.MC
		} else {
			c.Name = fmt.Sprintf("Minecraft %s (%s)", c.MC, c.Loader)
		}
	}
}

func (c *Config) Slug() string {
	b, _ := json.Marshal(c)
	h := fnv.New32a()
	h.Write(b)
	return slugify(c.Name) + "-" + fmt.Sprintf("%06x", h.Sum32()&0xffffff)
}

func decodeCfg(s string) (*Config, error) {
	s = strings.TrimSpace(s)
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
	if err != nil {
		raw, err = base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("config is not valid base64: %w", err)
		}
	}
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("config parse failed: %w", err)
	}
	if c.MC == "" {
		return nil, errors.New("config is missing the minecraft version")
	}
	switch c.Loader {
	case "", "vanilla", "fabric", "forge", "neoforge":
	default:
		return nil, fmt.Errorf("unknown loader %q", c.Loader)
	}
	c.applyDefaults()
	return &c, nil
}

// cfgFromOwnBinary reads a config appended to our own executable
// (the website's "download setup.exe" flow).
func cfgFromOwnBinary() (*Config, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		return nil, err
	}
	need := len(overlayMagic) + 8
	if len(data) < need {
		return nil, errNoOverlay
	}
	if string(data[len(data)-len(overlayMagic):]) != overlayMagic {
		return nil, errNoOverlay
	}
	lenOff := len(data) - need
	var l uint64
	for i := 0; i < 8; i++ {
		l |= uint64(data[lenOff+i]) << (8 * i)
	}
	if l == 0 || uint64(lenOff) < l {
		return nil, errors.New("config overlay is corrupt")
	}
	start := lenOff - int(l)
	return decodeCfg(string(data[start:lenOff]))
}
