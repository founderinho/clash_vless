//go:build !android && cgo

package gui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"clashvless/internal/control"
	"clashvless/internal/engine"
	"clashvless/internal/loghub"
	"clashvless/internal/store"
)

// connectionSummary never claims "Connected" unless system traffic is really captured:
// a working SOCKS route says nothing about the VPN.
func connectionSummary(vpnWanted bool, s engine.Status) (headline string, good bool, vpnLabel string) {
	online := s.Tier > 0
	switch {
	case vpnWanted && s.TunErr != "":
		return "Not protected", false, "VPN failed"
	case vpnWanted && !s.TunUp:
		return "Connecting", false, "VPN on · Connecting"
	case vpnWanted && online:
		return "Connected", true, "VPN on · Connected"
	case vpnWanted:
		return "No route", false, "VPN on · No route"
	case online:
		return "Proxy ready", true, "VPN off"
	}
	return "Disconnected", false, "VPN off"
}

func (d *dashboard) overviewView() fyne.CanvasObject {
	status := d.status
	online := status.Tier > 0
	connection, connected, vpnLabel := connectionSummary(d.state.TunEnabled, status)
	vpnSwitch := newConnectionSwitch(d.state.TunEnabled, func(on bool) {
		d.patch(map[string]any{"tun_enabled": on})
	})
	vpnControl := container.NewVBox(headingText(vpnLabel, 12), container.NewCenter(vpnSwitch))

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
		container.NewBorder(nil, nil, statusText(connection, connected), vpnControl),
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
		items = append(items, panel("VPN isn't running", "Your traffic is NOT going through the tunnel. Retrying automatically.", wrappingMuted(status.TunErr)))
	}

	endpointItems := []fyne.CanvasObject{}
	tunSlot := ""
	if d.state != nil {
		tunSlot = d.state.TunSlotName()
	}
	for _, slot := range d.state.Slots {
		if !slot.Enabled {
			continue
		}
		address := fmt.Sprintf("socks5://%s:%d", d.state.ListenHost(), slot.Port)
		details := address
		if tunSlot != "" && slot.Name == tunSlot {
			details += " · VPN slot"
		}
		endpointItems = append(endpointItems, flexRow(
			container.NewVBox(strong(slot.Name), ellipsized(details)),
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
		if sub.FetchErr != "" {
			providerItems = append(providerItems, wrappingMuted("Last fetch failed: "+sub.FetchErr))
		}
	}

	query := d.textInput("nodes", "Filter nodes", d.nodeQuery, func(value string) { d.nodeQuery = value })
	query.OnSubmitted = func(string) { d.render() }
	filter := widget.NewButton("Filter", d.render)
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
	tunSlot := ""
	if d.state != nil {
		tunSlot = d.state.TunSlotName()
	}
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
		slotTitle := ellipsized(slot.Name)
		slotTitle.TextStyle = fyne.TextStyle{Bold: true}
		isTun := tunSlot != "" && slot.Name == tunSlot
		slotDetails := fmt.Sprintf("Port %d · %s", slot.Port, live)
		if isTun {
			slotDetails += " · VPN slot"
		}
		var vpnBtn *widget.Button
		if isTun {
			vpnBtn = widget.NewButtonWithIcon("Selected for VPN", theme.ConfirmIcon(), nil)
			vpnBtn.Disable()
		} else {
			vpnBtn = widget.NewButton("Use for VPN", func() { d.patch(map[string]any{"tun_slot": slot.Name}) })
		}
		rows := []fyne.CanvasObject{
			flexRow(
				container.NewVBox(slotTitle, mutedLine(slotDetails)),
				enabled,
			),
			container.NewHBox(
				widget.NewButton("Edit port", func() { d.editSlotPort(slotIndex) }),
				vpnBtn,
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
				container.NewVBox(ellipsized(main.Name), mutedLine(mainHost(main.URL)+" · "+store.ProtoTag(main.URL))),
				widget.NewButtonWithIcon("", theme.DeleteIcon(), func() { d.removeMain(slotIndex, mainIndex) }),
			), container.NewHBox(exitEnabled, direct))
		}
		rows = append(rows, widget.NewButtonWithIcon("Add final exit", theme.ContentAddIcon(), func() { d.addMain(slotIndex) }))
		items = append(items, panel("", "", rows...))
	}
	return container.NewVBox(items...)
}

func (d *dashboard) editSlotPort(slotIndex int) {
	current := d.state.Slots[slotIndex].Port
	d.ask("Edit slot port", "SOCKS5 port (next port serves the first hop)", strconv.Itoa(current), "Save", func(value string) {
		port, err := strconv.Atoi(value)
		if err != nil || port < 1024 || port > 65534 {
			d.notify("Port must be between 1024 and 65534 (the next port serves the first hop)")
			return
		}
		for index, slot := range d.state.Slots {
			if index != slotIndex && (slot.Port == port || slot.Port == port+1 || slot.Port+1 == port) {
				d.notify("This port or its first-hop port is already used")
				return
			}
		}
		d.updateSlots(func(slots []store.Slot) {
			if slotIndex < len(slots) {
				slots[slotIndex].Port = port
			}
		})
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
		if slotIndex >= len(slots) || slots[slotIndex].Name != name {
			return // another client changed the slots while this dialog was open
		}
		slots = append(slots[:slotIndex], slots[slotIndex+1:]...)
		d.patch(map[string]any{"slots": slots})
	})
}

func (d *dashboard) activityView() fyne.CanvasObject {
	visible := []loghub.Record{}
	summary := muted("")
	var list *widget.List
	refresh := func() {
		visible = visible[:0]
		for _, record := range d.logs {
			if d.logFilter.Allows(record) {
				visible = append(visible, record)
			}
		}
		summary.Text = fmt.Sprintf("%d of %d records · click a row to copy", len(visible), len(d.logs))
		summary.Refresh()
		if list != nil {
			list.Refresh()
		}
	}
	list = widget.NewList(
		func() int { return len(visible) },
		func() fyne.CanvasObject { return &canvas.Text{TextSize: 12} },
		func(id widget.ListItemID, object fyne.CanvasObject) {
			text := object.(*canvas.Text)
			record := visible[id]
			text.Text = record.String()
			text.Color = colorMuted
			switch record.Level {
			case loghub.Info:
				text.Color = colorText
			case loghub.Warning:
				text.Color = colorAccent
			case loghub.Error:
				text.Color = colorDanger
			}
			text.Refresh()
		},
	)
	list.OnSelected = func(id widget.ListItemID) {
		d.window.Clipboard().SetContent(visible[id].String())
		d.notify("Log record copied")
		list.Unselect(id)
	}
	d.logList, d.logRefresh = list, refresh

	query := d.textInput("log", "Search: route & failed | timeout", d.logFilter.Query, func(value string) { d.logFilter.Query = value; refresh() })

	modules := []string{"All modules"}
	seen := map[string]bool{}
	for _, record := range d.logs {
		if !seen[record.Module] {
			seen[record.Module] = true
			modules = append(modules, record.Module)
		}
	}
	sort.Strings(modules[1:])
	module := widget.NewSelect(modules, func(value string) {
		d.logFilter.Module = ""
		if value != "All modules" {
			d.logFilter.Module = value
		}
		refresh()
	})
	if d.logFilter.Module != "" {
		module.SetSelected(d.logFilter.Module)
	} else {
		module.SetSelected("All modules")
	}

	levels := container.NewHBox()
	setLevels := func(shown map[loghub.Level]bool) {
		d.logFilter.Levels = shown
		d.render()
	}
	presets := container.NewHBox(
		widget.NewButton("Basic", func() { setLevels(loghub.DefaultFilter().Levels) }),
		widget.NewButton("All", func() {
			setLevels(map[loghub.Level]bool{loghub.Info: true, loghub.Warning: true, loghub.Error: true, loghub.Debug: true})
		}),
		widget.NewButton("Errors", func() { setLevels(map[loghub.Level]bool{loghub.Error: true}) }),
	)
	for _, level := range loghub.Levels {
		level := level
		check := widget.NewCheck(strings.ToUpper(string(level)), func(on bool) {
			if d.logFilter.Levels == nil {
				d.logFilter.Levels = map[loghub.Level]bool{loghub.Info: true, loghub.Warning: true, loghub.Error: true, loghub.Debug: true}
			}
			d.logFilter.Levels[level] = on
			refresh()
		})
		check.Checked = d.logFilter.Shows(level)
		levels.Add(check)
	}
	clear := widget.NewButton("Clear", func() { d.logs = nil; refresh() })
	refresh()
	return container.NewVBox(
		panel("Filters", "Severity, module, and text filters update instantly",
			presets, levels, query, module,
		),
		panel("Activity", "Newest records are at the bottom", summary, container.New(&activityListLayout{}, list), clear),
	)
}

type activityListLayout struct{}

func (*activityListLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, object := range objects {
		object.Resize(size)
	}
}

func (*activityListLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(320, 300)
}
