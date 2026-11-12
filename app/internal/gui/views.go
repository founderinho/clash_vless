//go:build !android && cgo

package gui

import (
	"encoding/json"
	"fmt"
	"net"
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
	if status.Entry == "" {
		route = "Direct  →  " + status.Main
	}
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
	for index, slot := range d.state.Slots {
		if !slot.Enabled {
			continue
		}
		address := fmt.Sprintf("socks5://%s", net.JoinHostPort(connectHost(d.state.ListenHost()), strconv.Itoa(slot.Port)))
		details := address
		if hop := d.state.HopPort(index); hop > 0 {
			details += fmt.Sprintf(" · first hop :%d", hop)
		}
		if tunSlot != "" && slot.Name == tunSlot {
			details += " · VPN slot"
		}
		endpointItems = append(endpointItems, flexRow(
			container.NewVBox(strong(slot.Name), ellipsized(details)),
			widget.NewButtonWithIcon("Copy", theme.ContentCopyIcon(), func(value string) func() {
				return func() { d.app.Clipboard().SetContent(value); d.notify("Address copied") }
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
		actions = append(actions,
			d.iconButton(qrIcon(), "Export nodes as VLESS links", func() {
				links, skipped := nodeShareLinks(sub.Nodes)
				d.showShare("Export · "+sub.Name, links, skipped)
			}),
			d.iconButton(theme.DeleteIcon(), "Remove subscription", func() {
				d.confirm("Remove subscription", "Remove "+sub.Name+" and its cached nodes?", func() {
					// Remove by URL: another client may have removed an earlier subscription meanwhile.
					for current, candidate := range d.state.Subs {
						if candidate.URL == sub.URL {
							d.command(control.Command{Cmd: "rmsub", Index: current}, "Subscription removed")
							return
						}
					}
					d.notify("This subscription was already removed")
				})
			}),
		)
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
			share := d.iconButton(qrIcon(), "Share as VLESS link", func() {
				node := d.state.FindNodeByName(probe.Name)
				if node == nil {
					d.notify("This node is no longer in your subscriptions")
					return
				}
				link, err := nodeShareLink(*node)
				if err != nil {
					d.notify("Cannot export: " + err.Error())
					return
				}
				d.showShare("Share entry node", []shareLink{link}, 0)
			})
			nodeItems = append(nodeItems, flexRow(
				container.NewVBox(ellipsized(probe.Name), muted(probe.Server+" · "+stats)),
				container.NewHBox(share, widget.NewButton(pin, func() {
					if d.state.PinEntry == probe.Name {
						d.patch(map[string]any{"pin_entry": ""})
					} else {
						d.patch(map[string]any{"pin_entry": probe.Name})
					}
				})),
			))
		}
	}
	return container.NewVBox(
		panel("Providers", "Choose one provider or combine all entry pools", providerItems...),
		panel("Entry nodes", "Pin a node to override automatic selection", nodeItems...),
	)
}

func tabLabel(name string, isTun bool) string {
	clean := strings.Join(strings.Fields(name), " ")
	if clean == "" {
		if isTun {
			return "VPN"
		}
		return "Slot"
	}
	runes := []rune(clean)
	maxLen := 18
	if isTun {
		maxLen = 14
	}
	if len(runes) > maxLen {
		clean = string(runes[:maxLen-1]) + "…"
	}
	if isTun {
		return clean + " · VPN"
	}
	return clean
}

func (d *dashboard) routesView() fyne.CanvasObject {
	tunSlot := ""
	if d.state != nil {
		tunSlot = d.state.TunSlotName()
	}

	slots := []store.Slot{}
	if d.state != nil {
		slots = d.state.Slots
	}

	selectedIdx := -1
	if len(slots) > 0 {
		selectedIdx = d.state.SlotIndexByName(d.selectedSlot)
		if selectedIdx < 0 {
			if d.selectedSlot == "" {
				selectedIdx = 0
				d.selectedSlot = slots[0].Name
			} else {
				selectedIdx = 0
			}
		}
	} else {
		d.selectedSlot = ""
	}

	addSlotBtn := widget.NewButtonWithIcon("Add slot", theme.ContentAddIcon(), d.addSlot)

	tabItems := make([]fyne.CanvasObject, 0, len(slots))
	for i, slot := range slots {
		slotName := slot.Name
		isTun := tunSlot != "" && slotName == tunSlot
		isSelected := i == selectedIdx

		label := tabLabel(slotName, isTun)
		var icon fyne.Resource
		if isTun {
			icon = theme.ConfirmIcon()
		}
		btn := widget.NewButtonWithIcon(label, icon, func() {
			d.selectedSlot = slotName
			d.render()
		})
		var tabItem fyne.CanvasObject = btn
		if isSelected {
			btn.Importance = widget.HighImportance
		} else if isTun {
			btn.Importance = widget.MediumImportance
		} else {
			btn.Importance = widget.LowImportance
			bg := canvas.NewRectangle(colorSurface)
			bg.CornerRadius = 4
			bg.StrokeColor = colorBorder
			bg.StrokeWidth = 1
			tabItem = container.NewStack(bg, btn)
		}
		tabItems = append(tabItems, tabItem)
	}

	tabsScroll := container.NewHScroll(container.NewHBox(tabItems...))
	d.tabsScroll = tabsScroll
	topBar := container.NewBorder(nil, nil, addSlotBtn, nil, inset(tabsScroll, 0, 0, 8, 0))

	items := []fyne.CanvasObject{topBar}
	if selectedIdx >= 0 && selectedIdx < len(slots) {
		items = append(items, d.slotPanel(selectedIdx, slots[selectedIdx], tunSlot))
	} else {
		items = append(items, panel("", "", muted("No failover slots configured. Click \"Add slot\" to create one.")))
	}
	return container.NewVBox(items...)
}

func slotTitleLabel(name string) *widget.Label {
	clean := strings.Join(strings.Fields(name), " ")
	runes := []rune(clean)
	const maxLen = 32
	if len(runes) > maxLen {
		clean = string(runes[:maxLen-1]) + "…"
	}
	label := widget.NewLabel(clean)
	label.TextStyle = fyne.TextStyle{Bold: true}
	return label
}

func (d *dashboard) slotPanel(slotIndex int, slot store.Slot, tunSlot string) fyne.CanvasObject {
	var live engine.SlotStatus
	for _, slotStatus := range d.status.Slots {
		if slotStatus.Name == slot.Name {
			live = slotStatus
		}
	}
	statusLabel := canvas.NewText("Disabled", colorMuted)
	if slot.Enabled {
		statusLabel = canvas.NewText("Enabled", colorText)
	}
	statusLabel.TextSize = 13
	enableSlider := newCompactSwitch(slot.Enabled, func(on bool) {
		if on {
			statusLabel.Text, statusLabel.Color = "Enabled", colorText
		} else {
			statusLabel.Text, statusLabel.Color = "Disabled", colorMuted
		}
		statusLabel.Refresh()
		name := slot.Name
		d.updateSlots(func(slots []store.Slot) {
			if index := d.state.SlotIndexByName(name); index >= 0 {
				slots[index].Enabled = on
			}
		})
	})
	sep := canvas.NewText("|", colorMuted)
	sep.TextSize = 13
	titleLeft := container.NewHBox(slotTitleLabel(slot.Name),
		d.iconButton(theme.DocumentCreateIcon(), "Rename slot", func() { d.renameSlot(slotIndex) }),
		sep, statusLabel, container.NewCenter(enableSlider))
	titleRow := flexRow(titleLeft, d.iconButton(theme.DeleteIcon(), "Delete slot", func() { d.removeSlot(slotIndex) }))

	isTun := tunSlot != "" && slot.Name == tunSlot
	ports := []fyne.CanvasObject{neutralChip(fmt.Sprintf("Port %d", slot.Port))}
	if hop := d.state.HopPort(slotIndex); hop > 0 {
		ports = append(ports, neutralChip(fmt.Sprintf("Hop %d", hop)))
	}
	var vpnBtn *widget.Button
	if isTun {
		vpnBtn = widget.NewButtonWithIcon("Selected for VPN", theme.ConfirmIcon(), nil)
		vpnBtn.Disable()
	} else {
		vpnBtn = widget.NewButton("Use for VPN", func() { d.patch(map[string]any{"tun_slot": slot.Name}) })
	}
	exportBtn := widget.NewButtonWithIcon("Export", qrIcon(), func() {
		links := make([]shareLink, len(slot.Mains))
		for i, main := range slot.Mains {
			links[i] = mainShareLink(main)
		}
		d.showShare("Export · "+slot.Name, links, 0)
	})
	if len(slot.Mains) == 0 {
		exportBtn.Disable()
	}
	rows := []fyne.CanvasObject{
		titleRow,
		flexRow(slotLiveLine(slot.Enabled, live), container.NewHBox(ports...)),
		container.NewHBox(widget.NewButton("Edit port", func() { d.editSlotPort(slotIndex) }), vpnBtn, exportBtn),
		widget.NewSeparator(),
	}
	if len(slot.Mains) == 0 {
		rows = append(rows, muted("No final exits in this slot yet."))
	} else {
		heading := canvas.NewText("Final exits", colorText)
		heading.TextSize = 12
		heading.TextStyle = fyne.TextStyle{Bold: true}
		rows = append(rows, container.NewHBox(heading, muted("in priority order: the top working one is used")))
	}
	for mainIndex, main := range slot.Mains {
		rows = append(rows, widget.NewSeparator(), d.mainRow(slotIndex, mainIndex, len(slot.Mains), main, live))
	}
	rows = append(rows, widget.NewButtonWithIcon("Add final exits", theme.ContentAddIcon(), func() { d.addMain(slotIndex) }))
	return panel("", "", rows...)
}

// slotLiveLine is the slot's state in one line: a colored tag, then the route.
func slotLiveLine(enabled bool, live engine.SlotStatus) fyne.CanvasObject {
	tag, detail, detailColor := neutralChip("Checking"), "looking for a working exit…", colorMuted
	switch {
	case !enabled:
		tag, detail = neutralChip("Off"), "this slot is not serving"
	case live.HopOnly: // Main names the entry node the hop port serves
		tag, detail, detailColor = tintChip("First hop only", colorWarning), live.Main+" · no final exit reachable", colorText
	case live.Tier > 0:
		label := fmt.Sprintf("T%d %s", live.Tier, tierName(live.Tier))
		if live.Egress > 0 {
			label += " · " + milliseconds(live.Egress)
		}
		tag, detail, detailColor = tintChip(label, colorAccent), live.Main, colorText
		if live.Entry != "" {
			detail = live.Entry + " → " + live.Main
		}
		if live.Note != "" {
			detail += " · " + live.Note
		}
	case live.Err != "":
		tag, detail, detailColor = tintChip("Down", colorDanger), live.Err, colorText
	}
	return container.NewBorder(nil, nil, inset(tag, 0, 0, 0, 6), nil, newFitText(detail, detailColor, 13, false))
}

// mainUsage says, in a couple of words, which tiers can use an exit.
func mainUsage(main store.Main) (text string, usable bool) {
	switch {
	case !main.Enabled:
		return "Disabled", false
	case store.IsVision(main.URL) && !main.AllowNoHop:
		return "Unused: Vision needs direct", false
	case store.IsVision(main.URL):
		return "Direct only", true
	case main.AllowNoHop:
		return "Direct or hop", true
	}
	return "Hop only", true
}

func (d *dashboard) mainRow(slotIndex, mainIndex, count int, main store.Main, live engine.SlotStatus) fyne.CanvasObject {
	active := live.Tier > 0 && !live.HopOnly && live.Main == main.Name
	usage, usable := mainUsage(main)

	badgeColor := colorControl
	if active {
		badgeColor = colorAccent
	}
	badgeBg := canvas.NewRectangle(badgeColor)
	badgeBg.CornerRadius = 11
	number := canvas.NewText(strconv.Itoa(mainIndex+1), colorText)
	if active {
		number.Color = colorBackground
	}
	number.TextSize = 12
	number.TextStyle = fyne.TextStyle{Bold: true}
	badge := container.NewCenter(container.NewGridWrap(fyne.NewSize(22, 22), container.NewStack(badgeBg, container.NewCenter(number))))

	nameColor := colorText
	if !main.Enabled {
		nameColor = colorMuted
	}
	title := newFitText(main.Name, nameColor, 14, active)
	var tags []fyne.CanvasObject
	if active {
		label := "In use · T" + strconv.Itoa(live.Tier)
		if live.Egress > 0 {
			label += " · " + milliseconds(live.Egress)
		}
		tags = append(tags, tintChip(label, colorAccent))
	}
	if proto := store.ProtoTag(main.URL); proto != "" {
		tags = append(tags, neutralChip(strings.ReplaceAll(proto, "·", " · ")))
	}
	if !usable && main.Enabled {
		tags = append(tags, tintChip(usage, colorDanger))
	} else {
		tags = append(tags, neutralChip(usage))
	}
	meta := container.NewBorder(nil, nil, container.NewHBox(tags...), nil, newFitText(mainHost(main.URL), colorMuted, 12, false))
	info := container.New(layout.NewCustomPaddedVBoxLayout(4), title, meta)

	up := d.iconButton(theme.MoveUpIcon(), "Raise priority", func() { d.moveMain(slotIndex, mainIndex, -1) })
	up.Disabled = mainIndex == 0
	down := d.iconButton(theme.MoveDownIcon(), "Lower priority", func() { d.moveMain(slotIndex, mainIndex, 1) })
	down.Disabled = mainIndex == count-1
	actions := container.NewHBox(up, down,
		d.iconButton(qrIcon(), "Share link / QR code", func() {
			d.showShare("Share exit", []shareLink{mainShareLink(main)}, 0)
		}),
		d.iconButton(theme.DocumentCreateIcon(), "Rename exit", func() { d.renameMain(slotIndex, mainIndex) }),
		d.iconButton(theme.DeleteIcon(), "Delete exit", func() { d.removeMain(slotIndex, mainIndex) }),
	)

	exitEnabled := widget.NewCheck("Enabled", func(on bool) {
		d.setMainFlag(slotIndex, mainIndex, func(m *store.Main) { m.Enabled = on })
	})
	exitEnabled.Checked = main.Enabled
	direct := widget.NewCheck("Allow direct (T1)", func(on bool) {
		d.setMainFlag(slotIndex, mainIndex, func(m *store.Main) { m.AllowNoHop = on })
	})
	direct.Checked = main.AllowNoHop
	toggles := inset(container.NewHBox(exitEnabled, direct), 0, 0, 22, 0)

	head := container.NewBorder(nil, nil, inset(badge, 0, 0, 0, 6), nil, info)
	return container.NewVBox(flexRow(head, actions), toggles)
}

func (d *dashboard) editSlotPort(slotIndex int) {
	if d.state == nil || slotIndex < 0 || slotIndex >= len(d.state.Slots) {
		return
	}
	name, current := d.state.Slots[slotIndex].Name, d.state.Slots[slotIndex].Port
	d.ask("Edit slot port", "SOCKS5 port (next port serves the first hop)", strconv.Itoa(current), "Save", func(value string) {
		port, err := strconv.Atoi(value)
		if err != nil || port < 1024 || port > 65534 {
			d.notify("Port must be between 1024 and 65534 (the next port serves the first hop)")
			return
		}
		index := d.state.SlotIndexByName(name)
		if index < 0 {
			d.notify("This slot was removed while editing")
			return
		}
		if d.state.SlotPortConflict(index, port) {
			d.notify("This port or its first-hop port is already used by another slot")
			return
		}
		d.updateSlots(func(slots []store.Slot) { slots[index].Port = port })
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
		index := d.state.SlotIndexByName(name)
		if index < 0 {
			return // another client removed it while this dialog was open
		}
		if len(slots) <= 1 {
			d.notify("The last slot cannot be removed")
			return
		}
		slotIndex := index
		slots = append(slots[:slotIndex], slots[slotIndex+1:]...)
		if d.selectedSlot == name {
			if slotIndex > 0 && slotIndex-1 < len(slots) {
				d.selectedSlot = slots[slotIndex-1].Name
			} else if len(slots) > 0 {
				d.selectedSlot = slots[0].Name
			} else {
				d.selectedSlot = ""
			}
		}
		d.state.Slots = slots
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
		d.app.Clipboard().SetContent(visible[id].String())
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
	follow := widget.NewCheck("Follow new records", func(on bool) {
		d.pauseLog = !on
		if on {
			list.ScrollToBottom()
		}
	})
	follow.Checked = !d.pauseLog
	refresh()
	return container.NewVBox(
		panel("Filters", "Severity, module, and text filters update instantly",
			presets, levels, query, module,
		),
		panel("Activity", "Newest records are at the bottom", summary, container.New(&activityListLayout{}, list), container.NewHBox(clear, follow)),
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

// connectHost is the address a local app dials for a listener bound to host:
// a wildcard bind is reachable on loopback, and dialing 0.0.0.0 fails on Windows.
func connectHost(host string) string {
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		return "127.0.0.1"
	}
	return host
}
