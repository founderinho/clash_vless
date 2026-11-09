# Clashvless brand — Route Signal

This folder is the shared, toolkit-independent visual identity for desktop, Android, and other clients.

## Contents

- `logo.svg` — scalable application and launcher logo with a transparent background.
- `windows/clashvless.ico` — multi-resolution Windows icon, embedded by the native build.
- `icons/` — 24×24 SVG icons using `currentColor`, so host applications can tint them.
- `png/` — pre-rendered transparent PNGs for Android and other non-SVG consumers.
- `tokens.json` — portable colors, corner radii, and spacing values.
- `branding.go` — embedded assets and Go color values; importing this package adds no UI dependency.
- `app/icon_windows_amd64.syso` — generated from `windows/clashvless.rc` with `windres`; Go embeds it automatically in both Windows executables.

The connected-node shield represents tiered routes. Mint indicates an active/healthy route, blue indicates the exit, and the neutral graphite surfaces keep connection information prominent.

## Go / Fyne

```go
import (
    "clashvless/branding"
    "fyne.io/fyne/v2"
    "fyne.io/fyne/v2/theme"
)

logoData, _ := branding.Logo()
logo := fyne.NewStaticResource("clashvless-logo.svg", logoData)

iconData, _ := branding.Icon("overview")
icon := theme.NewThemedResource(fyne.NewStaticResource("overview.svg", iconData))

background := branding.Background
```

Available icons: `overview`, `subscriptions`, `routes`, `activity`, `settings`, `refresh`, `copy`, `add`, and `delete`.

Use `branding.Files` for direct embedded-filesystem access, `branding.Tokens()` for the JSON tokens, and `branding.PNG(name)` for raster assets.

## Android

`android/build.sh` automatically:

- installs `png/logo.png` as the transparent launcher icon;
- packages `tokens.json` and `png/` under APK assets;
- uses `png/notification.png` (white-on-transparent) for VPN notifications and the shared shield vector for the quick-settings tile;
- lets `MainActivity` load the shared colors and tintable navigation icons.

For another native toolkit, copy `tokens.json` and either `icons/` or `png/`; no generation step is required.
