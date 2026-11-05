//go:build !android

package gui

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"clashvless/internal/control"
	"clashvless/internal/engine"
	"clashvless/internal/store"
)

func (d *dashboard) overviewView() fyne.CanvasObject {
	status := d.status
	online := status.Tier > 0
	connection := "Disconnected"
	if online {
		connection = "Connected"
	}
	vpnLabel := "VPN off"
	if d.state.TunEnabled {
		vpnLabel = "VPN on · Connecting"
		if online {
			vpnLabel = "VPN on · Connected"
		}
	}
	vpnSlider := widget.NewSlider(0, 1)
	vpnSlider.SetValue(map[bool]float64{false: 0, true: 1}[d.state.TunEnabled])
	vpnSlider.OnChangeEnded = func(value float64) {
		on := value >= 0.5
		vpnSlider.SetValue(map[bool]float64{false: 0, true: 1}[on])
		if on != d.state.TunEnabled {
			d.patch(map[string]any{"tun_enabled": on})
		}
	}
	vpnControl := container.NewVBox(headingText(vpnLabel, 12), container.New(&widthLayout{width: 100}, vpnSlider))

	route := status.Entry + "  →  " + status.Main
	if !online {
		route = status.Err
		if route == "" {
			route = "No working route yet"
		}
	}
	healthy := 0
	for _, probe := range append(append([]engine.Probe{}, status.Country...), status.Bypass...) {
		if probe.OK {
			healthy++
		}
	}
	tier := "—"
	if online {
		tier = fmt.Sprintf("T%d", status.Tier)
	}
	connectionItems := []fyne.CanvasObject{
		container.NewBorder(nil, nil, statusText(connection, online), vpnControl),
		ellipsized(route),
		container.NewGridWithColumns(3,
			metric("Latency", milliseconds(status.Egress)),
			metric("Healthy entries", fmt.Sprintf("%d / %d", healthy, len(status.Country)+len(status.Bypass))),
			metric("Active tier", tier),
		),
	}
	if status.Note != "" {
		connectionItems = append(connectionItems, wrappingMuted(status.Note))
	}
	items := []fyne.CanvasObject{panel("Connection", "", connectionItems...)}
	if status.TunErr != "" {
		items = append(items, panel("VPN error", "", wrappingMuted(status.TunErr)))
	}

	endpointItems := []fyne.CanvasObject{}
	for _, slot := range d.state.Slots {
		if !slot.Enabled {
			continue
		}
		address := fmt.Sprintf("socks5://%s:%d", d.state.ListenHost(), slot.Port)
		endpointItems = append(endpointItems, flexRow(
			container.NewVBox(strong(slot.Name), ellipsized(address)),
			widget.NewButtonWithIcon("Copy", theme.ContentCopyIcon(), func(value string) func() {
				return func() { d.window.Clipboard().SetContent(value); d.notify("Address copied") }
			}(address)),
		))
	}
	if len(endpointItems) == 0 {
		endpointItems = append(endpointItems, muted("No enabled slots"))
	}
	items = append(items, panel("Local endpoints", "SOCKS5 addresses for your applications", endpointItems...))
	items = append(items, panel("Entry pools", "",
		d.poolSummary("Country exits · T2", status.Country),
		widget.NewSeparator(),
		d.poolSummary("Bypass nodes · T3", status.Bypass),
	))
	if !d.state.AnyUsableMain() {
		items = append(items, panel("Complete setup", "Add a final VLESS exit to carry your traffic.",
			widget.NewButtonWithIcon("Add final exit", theme.ContentAddIcon(), func() { d.addMain(0) })))
	}
	return container.NewVBox(items...)
}

func (d *dashboard) poolSummary(name string, probes []engine.Probe) fyne.CanvasObject {
	up := 0
	measured, flakiness := 0, 0.0
	for _, probe := range probes {
		if probe.OK {
			up++
		}
		if probe.Samples > 0 {
			measured++
			flakiness += probe.Flakiness
		}
	}
	stability := "Waiting for measurements"
	if measured > 0 {
		stability = fmt.Sprintf("%.0f%% flakiness · last hour", flakiness/float64(measured))
	}
	return flexRow(
		container.NewVBox(strong(name), muted(stability)),
		container.NewVBox(headingText(fmt.Sprintf("%d / %d", up, len(probes)), 16), muted("online")),
	)
}

func (d *dashboard) subscriptionsView() fyne.CanvasObject {
	providerItems := []fyne.CanvasObject{container.NewHBox(
		widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), d.addSubscription),
		widget.NewButtonWithIcon("Refresh all", theme.ViewRefreshIcon(), func() {
			d.command(control.Command{Cmd: "refetch"}, "Refreshing subscriptions…")
		}),
	)}
	aggregate := widget.NewCheck("Combine all subscriptions", func(on bool) {
		d.patch(map[string]any{"auto_select": on})
	})
	aggregate.Checked = d.state.AutoSelect
	providerItems = append(providerItems, aggregate)
	if len(d.state.Subs) == 0 {
		providerItems = append(providerItems, widget.NewSeparator(), muted("No subscriptions yet. Add a Remnawave URL to get started."))
	}
	for index, sub := range d.state.Subs {
		index, sub := index, sub
		updated := "Not fetched"
		if !sub.LastFetch.IsZero() {
			updated = sub.LastFetch.Local().Format("02 Jan · 15:04")
		}
		active := ""
		if d.state.AutoSelect || d.state.ActiveSub == index {
			active = " · Active"
		}
		actions := []fyne.CanvasObject{}
		if !d.state.AutoSelect && d.state.ActiveSub != index {
			actions = append(actions, widget.NewButton("Select", func() {
				d.patch(map[string]any{"auto_select": false, "active_sub": index})
			}))
		}
		actions = append(actions, widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
			d.confirm("Remove subscription", "Remove "+sub.Name+" and its cached nodes?", func() {
				d.command(control.Command{Cmd: "rmsub", Index: index}, "Subscription removed")
			})
		}))
		providerItems = append(providerItems, widget.NewSeparator(), flexRow(
			container.NewVBox(ellipsized(sub.Name), muted(fmt.Sprintf("%d nodes · %s%s", len(sub.Nodes), updated, active))),
			container.NewHBox(actions...),
		))
	}

	query := widget.NewEntry()
	query.SetPlaceHolder("Filter nodes")
	query.SetText(d.nodeQuery)
	query.OnSubmitted = func(value string) { d.nodeQuery = value; d.render() }
	filter := widget.NewButton("Filter", func() { d.nodeQuery = query.Text; d.render() })
	nodeItems := []fyne.CanvasObject{container.NewBorder(nil, nil, nil, filter, query)}
	if d.state.PinEntry != "" {
		nodeItems = append(nodeItems, widget.NewButtonWithIcon("Unpin "+d.state.PinEntry, theme.CancelIcon(), func() {
			d.patch(map[string]any{"pin_entry": ""})
		}))
	}
	for _, pool := range []struct {
		name   string
		probes []engine.Probe
	}{
		{"Country exits · T2", d.status.Country},
		{"Bypass nodes · T3", d.status.Bypass},
	} {
		nodeItems = append(nodeItems, widget.NewSeparator(), strong(pool.name))
		if len(pool.probes) == 0 {
			nodeItems = append(nodeItems, muted("No nodes in this pool"))
		}
		for _, probe := range pool.probes {
			if d.nodeQuery != "" && !strings.Contains(strings.ToLower(probe.Name+" "+probe.Server), strings.ToLower(d.nodeQuery)) {
				continue
			}
			probe := probe
			health := "Offline"
			if probe.OK {
				health = milliseconds(probe.Latency)
			}
			stats := health
			if probe.Speed > 0 {
				stats += fmt.Sprintf(" · %.0f Mbps", probe.Speed)
			}
			if probe.Samples > 0 {
				stats += fmt.Sprintf(" · %.0f%% flakiness", probe.Flakiness)
			}
			pin := "Pin"
			if d.state.PinEntry == probe.Name {
				pin = "Unpin"
			}
			nodeItems = append(nodeItems, flexRow(
				container.NewVBox(ellipsized(probe.Name), muted(probe.Server+" · "+stats)),
				widget.NewButton(pin, func() {
					if d.state.PinEntry == probe.Name {
						d.patch(map[string]any{"pin_entry": ""})
					} else {
						d.patch(map[string]any{"pin_entry": probe.Name})
					}
				}),
			))
		}
	}
	return container.NewVBox(
		panel("Providers", "Choose one provider or combine all entry pools", providerItems...),
		panel("Entry nodes", "Pin a node to override automatic selection", nodeItems...),
	)
}

func (d *dashboard) routesView() fyne.CanvasObject {
	items := []fyne.CanvasObject{container.NewHBox(
		widget.NewButtonWithIcon("Add slot", theme.ContentAddIcon(), d.addSlot),
		layout.NewSpacer(),
	)}
	for slotIndex, slot := range d.state.Slots {
		slotIndex, slot := slotIndex, slot
		live := "Idle"
		for _, slotStatus := range d.status.Slots {
			if slotStatus.Name == slot.Name && slotStatus.Tier > 0 {
				live = fmt.Sprintf("T%d · %s · %s", slotStatus.Tier, slotStatus.Main, milliseconds(slotStatus.Egress))
			}
		}
		enabled := widget.NewCheck("Enabled", func(on bool) {
			d.updateSlots(func(slots []store.Slot) { slots[slotIndex].Enabled = on })
		})
		enabled.Checked = slot.Enabled
		rows := []fyne.CanvasObject{
			flexRow(
				container.NewVBox(strong(slot.Name), muted(fmt.Sprintf("Port %d · %s", slot.Port, live))),
				enabled,
			),
			container.NewHBox(
				widget.NewButton("Edit port", func() { d.editSlotPort(slotIndex) }),
				widget.NewButton("Use for VPN", func() { d.patch(map[string]any{"tun_slot": slot.Name}) }),
				layout.NewSpacer(),
				widget.NewButtonWithIcon("Delete slot", theme.DeleteIcon(), func() { d.removeSlot(slotIndex) }),
			),
		}
		if len(slot.Mains) == 0 {
			rows = append(rows, widget.NewSeparator(), muted("No final exits in this slot."))
		}
		for mainIndex, main := range slot.Mains {
			slotIndex, mainIndex, main := slotIndex, mainIndex, main
			exitEnabled := widget.NewCheck("Enabled", func(on bool) {
				d.updateSlots(func(slots []store.Slot) { slots[slotIndex].Mains[mainIndex].Enabled = on })
			})
			exitEnabled.Checked = main.Enabled
			direct := widget.NewCheck("Allow direct · T1", func(on bool) {
				d.updateSlots(func(slots []store.Slot) { slots[slotIndex].Mains[mainIndex].AllowNoHop = on })
			})
			direct.Checked = main.AllowNoHop
			rows = append(rows, widget.NewSeparator(), flexRow(
				container.NewVBox(ellipsized(main.Name), muted(mainHost(main.URL)+" · "+store.ProtoTag(main.URL))),
				widget.NewButtonWithIcon("", theme.DeleteIcon(), func() { d.removeMain(slotIndex, mainIndex) }),
			), container.NewHBox(exitEnabled, direct))
		}
		rows = append(rows, widget.NewButtonWithIcon("Add final exit", theme.ContentAddIcon(), func() { d.addMain(slotIndex) }))
		items = append(items, panel("", "", rows...))
	}
	items = append(items, panel("Routing compatibility", "Vision exits are direct-only. REALITY exits require a non-Vision hop; plain exits support any hop."))
	return container.NewVBox(items...)
}

func (d *dashboard) editSlotPort(slotIndex int) {
	current := d.state.Slots[slotIndex].Port
	d.ask("Edit slot port", "SOCKS5 port (next port serves the first hop)", strconv.Itoa(current), "Save", func(value string) {
		port, err := strconv.Atoi(value)
		if err != nil || port < 1024 || port > 65535 {
			d.notify("Port must be between 1024 and 65535")
			return
		}
		for index, slot := range d.state.Slots {
			if index != slotIndex && (slot.Port == port || slot.Port == port+1 || slot.Port+1 == port) {
				d.notify("This port or its first-hop port is already used")
				return
			}
		}
		d.updateSlots(func(slots []store.Slot) { slots[slotIndex].Port = port })
	})
}

func (d *dashboard) removeSlot(slotIndex int) {
	if len(d.state.Slots) <= 1 {
		d.notify("The last slot cannot be removed")
		return
	}
	name := d.state.Slots[slotIndex].Name
	d.confirm("Delete slot", "Delete "+name+" and all of its final exits?", func() {
		data, _ := json.Marshal(d.state.Slots)
		var slots []store.Slot
		_ = json.Unmarshal(data, &slots)
		slots = append(slots[:slotIndex], slots[slotIndex+1:]...)
		d.patch(map[string]any{"slots": slots})
	})
}

func (d *dashboard) activityView() fyne.CanvasObject {
	logs := widget.NewMultiLineEntry()
	logs.SetText(strings.Join(d.logs, "\n"))
	logs.SetMinRowsVisible(18)
	logs.Disable()
	clear := widget.NewButton("Clear", func() { d.logs = nil; d.render() })
	return panel("Engine log", fmt.Sprintf("%d lines · newest at the bottom", len(d.logs)), clear, logs)
}
