The unmodified `wintun-0.14.1.zip` comes from:
https://www.wintun.net/builds/wintun-0.14.1.zip

SHA-256: `07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51`

It contains the official signed DLLs for amd64, x86, arm64 and arm, plus
upstream's README, headers and `wintun/LICENSE.txt`. Keep the archive and its
license intact when updating. Windows builds embed it with Go's `embed` package;
other platforms exclude it. No build-time or runtime download is required.

On first VPN startup, the appropriate DLL is extracted beside the executable
before xray loads it. An existing `wintun.dll` is left unchanged.
