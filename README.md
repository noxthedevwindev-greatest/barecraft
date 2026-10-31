# Barecraft

**No launcher. Just play.**

Barecraft is a tiny static website plus a self-contained installer. You pick a
Minecraft version, a loader (vanilla / Fabric / Forge / NeoForge) and some
Modrinth mods, and it hands you a single file. Double-click it and it:

- installs a matching Java runtime if you don't have one (Temurin JRE, cached),
- downloads the game, libraries and assets into its own isolated folder,
- runs the loader's installer headlessly (Fabric / Forge / NeoForge),
- pulls your mods from Modrinth,
- bakes a `launch.sh` / `launch.bat` that runs the game directly with `java`,
- drops a shortcut on your desktop and in your start menu / Launchpad.

Your existing `.minecraft` is never touched.

## How it works

The site is plain HTML/CSS/JS — no backend. The config you choose is compressed
into the download itself: the site fetches the platform binary, appends a small
config record, and saves it as one file. Running that file does everything; it
needs no launcher, no account, no internet after the first run (mods/libs are
cached).

A terminal one-liner is also offered for people who prefer `curl`.

## Platforms

- Windows — `.exe` (double-click)
- macOS — executable (Terminal one-liner shown for Gatekeeper)
- Linux — executable (or `curl … | bash`)

Java Edition only. No Android, no Bedrock.

## Build the binaries

You need Go (1.24+). On Arch: `sudo pacman -S go`.

```sh
./build.sh
```

This cross-compiles `barecraft` for linux (amd64/arm64), darwin (amd64/arm64) and
windows (amd64) into `site/bin/`.

## Run locally (dev)

```sh
python3 -m http.server 8000
```

Open http://localhost:8000. The site fetches its version list and support matrix
from the baked `versions.json` / `support.json` plus live Modrinth/Mojang APIs.

## Publish (GitHub Pages)

```sh
gh repo create <you>/barecraft --public --source=. --push
gh api -X POST repos/<you>/barecraft/pages -f source[branch]=main -f source[path]=/
```

The site then lives at `https://<you>.github.io/barecraft/`.

## Environment overrides (testing / portable)

- `BARECRAFT_HOME` — redirect the whole install root (libraries, assets, java,
  instances, shortcuts) somewhere else.
- `BARECRAFT_ROOT` — override only the shared root.
- `BARECRAFT_DRY_RUN=1` — skip asset downloads (useful for testing resolution).

## License

MIT.
