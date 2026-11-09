//go:build !android && cgo

package gui

import (
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"clashvless/internal/store"
)

func (d *dashboard) settingsView() fyne.CanvasObject {
	sections := []string{"Network & failover", "VPN", "Subscriptions", "Device"}
	selector := widget.NewSelect(sections, nil)
	selector.SetSelected(d.settingsSection)
	selector.OnChanged = func(section string) {
		d.settingsSection = section
		d.render()
	}

	var content fyne.CanvasObject
	switch d.settingsSection {
	case "VPN":
		content = container.NewVBox(
			panel("System-wide VPN", "Capture device traffic through the selected exit slot",
				d.boolSetting("Enable VPN", "tun_enabled", d.state.TunEnabled, true, false),
				d.selectSetting("DNS mode", "tun_dns_direct", strconv.FormatBool(d.state.TunDNSDirect), []string{"false", "true"}, map[string]string{"false": "Static routed", "true": "Real network"}),
				d.textSetting("Static DNS resolver", "tun_static_dns", d.state.TunStaticDNS, "8.8.8.8"),
				d.boolSetting("Tunnel private/LAN ranges", "tun_tunnel_lan", d.state.TunTunnelLAN, true, false),
				d.boolSetting("Allow IPv6 outside VPN", "tun_allow_ipv6", d.state.TunAllowIPv6, true, false),
				d.boolSetting("Answer ping locally", "tun_no_icmp", d.state.TunICMP(), false, true),
			),
			panel("Virtual interface", "Changes apply when the VPN reconnects",
				d.textSetting("Device name", "tun_name", d.state.TunName, "Platform default"),
				d.textSetting("Interface address", "tun_addr", d.state.TunAddr, "198.18.0.1/30"),
				d.numberSetting("MTU · 0 = default", "tun_mtu", d.state.TunMTU, 0, 9000),
			),
		)
	case "Subscriptions":
		content = panel("Subscription updates", "Background refresh and optional fetch proxy",
			d.boolSetting("Automatic refresh", "no_auto_fetch", d.state.AutoFetch(), false, true),
			d.numberSetting("Refresh interval · hours", "fetch_interval_h", effective(d.state.FetchIntervalH, 24), 1, 168),
			d.boolSetting("Use fetch proxy", "use_fetch_proxy", d.state.UseFetchProxy, true, false),
			d.textSetting("Fetch proxy", "fetch_proxy", d.state.FetchProxy, "socks5://127.0.0.1:2084"),
		)
	case "Device":
		content = panel("Device identity", "Stable Happ-compatible identity shared by every subscription",
			d.textSetting("Hardware ID", "device_hwid", d.state.Device.HWID, ""),
			widget.NewSeparator(),
			infoRow("Panel OS", d.state.Device.OS+" "+d.state.Device.OSVer),
			infoRow("Model", d.state.Device.Model),
			infoRow("User agent", d.state.Device.UA),
			infoRow("Locale", d.state.Device.Locale),
		)
	default:
		content = container.NewVBox(
			panel("Automatic failover", "Route selection, health checks, and recovery",
				d.selectSetting("Pin tier", "pin_tier", strconv.Itoa(d.state.PinTier), []string{"0", "1", "2", "3"}, map[string]string{"0": "Automatic", "1": "T1 · Direct", "2": "T2 · Country", "3": "T3 · Bypass"}),
				d.boolSetting("Force a hop · skip T1", "force_hop", d.state.ForceHop, true, false),
				d.numberSetting("Probe interval · seconds", "interval_s", effective(d.state.Interval, 12), 2, 600),
				d.numberSetting("Egress timeout · seconds", "timeout_s", effective(d.state.Timeout, 6), 1, 60),
				d.numberSetting("Recovery threshold", "up_threshold", effective(d.state.UpThreshold, 3), 1, 10),
				d.numberSetting("Failure threshold", "down_threshold", effective(d.state.DownThreshold, 2), 1, 10),
			),
			panel("Local listeners", "Listener and log changes apply after a daemon restart",
				d.numberSetting("Listen port", "listen_port", d.state.ListenPort, 1024, 65534),
				d.numberSetting("First-hop port · 0 = auto", "entry_port", d.state.EntryPort, 0, 65535),
				d.boolSetting("Allow LAN connections", "listen_addr", d.state.ListenHost() == "0.0.0.0", "0.0.0.0", "127.0.0.1"),
				d.selectSetting("Xray log level", "log_level", d.state.Loglevel(), []string{"none", "error", "warning", "info", "debug"}, nil),
			),
		)
	}
	return container.NewVBox(
		flexRow(widget.NewLabel("Section"), container.New(&widthLayout{width: 200}, selector)),
		content,
	)
}

func settingRow(name string, control fyne.CanvasObject) fyne.CanvasObject {
	label := widget.NewLabel(name)
	label.Truncation = fyne.TextTruncateEllipsis
	return flexRow(label, control)
}

func infoRow(name, value string) fyne.CanvasObject {
	return container.NewVBox(muted(name), ellipsized(value))
}

func (d *dashboard) numberSetting(name, key string, current, min, max int) fyne.CanvasObject {
	entry := widget.NewEntry()
	entry.SetText(strconv.Itoa(current))
	save := widget.NewButtonWithIcon("", theme.ConfirmIcon(), func() {
		value, err := strconv.Atoi(entry.Text)
		if err != nil || value < min || value > max {
			d.notify(fmt.Sprintf("%s must be between %d and %d", name, min, max))
			return
		}
		d.patch(map[string]any{key: value})
	})
	entry.OnSubmitted = func(string) { save.OnTapped() }
	return settingRow(name, container.NewBorder(nil, nil, nil, save, container.New(&widthLayout{width: 180}, entry)))
}

func (d *dashboard) textSetting(name, key, current, placeholder string) fyne.CanvasObject {
	entry := widget.NewEntry()
	entry.SetText(current)
	entry.SetPlaceHolder(placeholder)
	save := widget.NewButtonWithIcon("", theme.ConfirmIcon(), func() {
		value := entry.Text
		if key == "device_hwid" {
			if !store.ValidHWID(value) {
				d.notify("HWID must be 10–64 letters, digits, = or -")
				return
			}
			device := d.state.Device
			device.HWID = value
			d.patch(map[string]any{"device": device})
			return
		}
		d.patch(map[string]any{key: value})
	})
	entry.OnSubmitted = func(string) { save.OnTapped() }
	return settingRow(name, container.NewBorder(nil, nil, nil, save, container.New(&widthLayout{width: 180}, entry)))
}

func (d *dashboard) selectSetting(name, key, current string, options []string, labels map[string]string) fyne.CanvasObject {
	valuesByLabel := map[string]string{}
	displayOptions := make([]string, len(options))
	currentLabel := current
	for i, value := range options {
		label := value
		if labels != nil {
			label = labels[value]
		}
		displayOptions[i] = label
		valuesByLabel[label] = value
		if value == current {
			currentLabel = label
		}
	}
	selectWidget := widget.NewSelect(displayOptions, nil)
	selectWidget.SetSelected(currentLabel)
	selectWidget.OnChanged = func(label string) {
		value := valuesByLabel[label]
		if key == "pin_tier" {
			valueNumber, _ := strconv.Atoi(value)
			d.patch(map[string]any{key: valueNumber})
			return
		}
		if key == "tun_dns_direct" {
			d.patch(map[string]any{key: value == "true"})
			return
		}
		d.patch(map[string]any{key: value})
	}
	return settingRow(name, selectWidget)
}

func (d *dashboard) boolSetting(name, key string, checked bool, onValue, offValue any) fyne.CanvasObject {
	check := widget.NewCheck("", nil)
	check.Checked = checked
	check.OnChanged = func(on bool) {
		value := offValue
		if on {
			value = onValue
		}
		d.patch(map[string]any{key: value})
	}
	return settingRow(name, check)
}

func effective(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}
