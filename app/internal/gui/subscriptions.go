//go:build !android && cgo

package gui

import (
	"fmt"
	"image/color"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"clashvless/branding"
	"clashvless/internal/control"
	"clashvless/internal/engine"
)

const (
	sortRecommended = "Recommended"
	sortLatency     = "Lowest latency"
	sortName        = "Name"
	sortProvider    = "Provider order"
)

func (d *dashboard) subscriptionsView() fyne.CanvasObject {
	return container.NewVBox(d.providersPanel(), d.nodesPanel())
}

func ago(t, now time.Time) string {
	if t.IsZero() {
		return "never fetched"
	}
	elapsed := now.Sub(t)
	switch {
	case elapsed < time.Minute:
		return "updated just now"
	case elapsed < time.Hour:
		return fmt.Sprintf("updated %d min ago", int(elapsed.Minutes()))
	case elapsed < 48*time.Hour:
		return fmt.Sprintf("updated %d h ago", int(elapsed.Hours()))
	}
	return "updated " + t.Local().Format("02 Jan 2006")
}

func (d *dashboard) providersPanel() fyne.CanvasObject {
	add := widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), d.addSubscription)
	refresh := widget.NewButtonWithIcon("Refresh all", theme.ViewRefreshIcon(), func() {
		d.command(control.Command{Cmd: "refetch"}, "Refreshing subscriptions…")
	})
	if len(d.state.Subs) == 0 {
		refresh.Disable()
	}
	actions := container.NewHBox(add, refresh)
	if len(d.state.Subs) == 0 {
		return section("Providers", "Remnawave subscriptions that supply entry nodes", actions,
			widget.NewSeparator(),
			wrappingMuted("No subscriptions yet. Add a subscription URL to get entry nodes for hopped routes (T2/T3)."))
	}
	subtitle := "Only the active provider's nodes are used"
	if d.state.AutoSelect {
		subtitle = "Nodes from all providers are combined"
	}
	var rows []fyne.CanvasObject
	if len(d.state.Subs) > 1 {
		combine := widget.NewCheck("Use nodes from all providers", func(on bool) {
			d.patch(map[string]any{"auto_select": on})
		})
		combine.Checked = d.state.AutoSelect
		rows = append(rows, combine)
	}
	online := map[string]bool{}
	for _, probe := range append(append([]engine.Probe{}, d.status.Country...), d.status.Bypass...) {
		online[probe.Name] = online[probe.Name] || probe.OK
	}
	now := time.Now()
	for index, sub := range d.state.Subs {
		index, sub := index, sub
		active := d.state.AutoSelect || d.state.ActiveSub == index
		country, bypass, up := 0, 0, 0
		for _, node := range sub.Nodes {
			if node.Whitelist {
				bypass++
			} else {
				country++
			}
			if online[node.Name] {
				up++
			}
		}
		var tag fyne.CanvasObject
		switch {
		case sub.FetchErr != "":
			tag = tintChip("Fetch failed", colorDanger)
		case active:
			tag = tintChip("Active", colorAccent)
		default:
			tag = neutralChip("Not used")
		}
		facts := fmt.Sprintf("%d nodes", len(sub.Nodes))
		if bypass > 0 {
			facts = fmt.Sprintf("%d nodes (%d country, %d bypass)", len(sub.Nodes), country, bypass)
		}
		if active && len(sub.Nodes) > 0 {
			facts += fmt.Sprintf(" · %d online", up)
		}
		facts += " · " + ago(sub.LastFetch, now)
		info := []fyne.CanvasObject{titleWithTags(newFitText(sub.Name, colorText, 14, true), tag), newFitText(facts, colorMuted, 12, false)}
		if sub.FetchErr != "" {
			reason := wrappingMuted("Last fetch failed: " + sub.FetchErr)
			reason.Importance = widget.DangerImportance
			info = append(info, inset(reason, -4, -4, -8, 0)) // align with the unpadded lines above
		}
		buttons := []fyne.CanvasObject{}
		if !active {
			buttons = append(buttons, widget.NewButton("Use this", func() {
				d.patch(map[string]any{"auto_select": false, "active_sub": index})
			}))
		}
		buttons = append(buttons,
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
		rows = append(rows, widget.NewSeparator(), flexRow(
			container.New(layout.NewCustomPaddedVBoxLayout(2), info...),
			container.NewHBox(buttons...),
		))
	}
	return section("Providers", subtitle, actions, rows...)
}

// sortProbes orders a copy of a pool for display. Recommended follows the
// engine's preference: online, steadier, faster throughput, then lower ping.
func sortProbes(probes []engine.Probe, mode string) []engine.Probe {
	sorted := append([]engine.Probe(nil), probes...)
	type key struct {
		offline, band, unmeasured int
		speed                     float64
		latency                   time.Duration
		name                      string
	}
	keyOf := func(p engine.Probe) key {
		k := key{band: 3, unmeasured: 1, latency: p.Latency, name: strings.ToLower(p.Name)}
		if !p.OK {
			k.offline = 1
		}
		if p.Samples > 0 {
			k.band = int(p.Flakiness / 10)
		}
		if p.SpeedN > 0 {
			k.unmeasured, k.speed = 0, p.Speed
		}
		return k
	}
	var less func(a, b key) bool
	switch mode {
	case sortProvider:
		return sorted
	case sortName:
		less = func(a, b key) bool { return a.name < b.name }
	case sortLatency:
		less = func(a, b key) bool {
			if a.offline != b.offline {
				return a.offline < b.offline
			}
			return a.latency < b.latency
		}
	default:
		less = func(a, b key) bool {
			switch {
			case a.offline != b.offline:
				return a.offline < b.offline
			case a.band != b.band:
				return a.band < b.band
			case a.unmeasured != b.unmeasured:
				return a.unmeasured < b.unmeasured
			case a.speed != b.speed:
				return a.speed > b.speed
			}
			return a.latency < b.latency
		}
	}
	sort.SliceStable(sorted, func(i, j int) bool { return less(keyOf(sorted[i]), keyOf(sorted[j])) })
	return sorted
}

func flakeChip(pct float64) fyne.CanvasObject {
	label := fmt.Sprintf("%.0f%% fails", pct)
	switch {
	case pct > 30:
		return tintChip(label, colorDanger)
	case pct > 10:
		return tintChip(label, colorWarning)
	}
	return neutralChip(label)
}

type nodeEntry struct {
	row    *fyne.Container
	search string
	ok     bool
}

func (d *dashboard) nodesPanel() fyne.CanvasObject {
	if d.nodeSort == "" {
		d.nodeSort = sortRecommended
	}
	inUse := entriesInUse(d.status)
	var entries []nodeEntry
	var groups []fyne.CanvasObject
	var refreshers []func() int
	for _, pool := range []struct {
		name   string
		tier   int
		probes []engine.Probe
	}{
		{"Country exits", 2, d.status.Country},
		{"Bypass nodes", 3, d.status.Bypass},
	} {
		up := 0
		for _, probe := range pool.probes {
			if probe.OK {
				up++
			}
		}
		head := flexRow(container.NewHBox(strong(pool.name), neutralChip(fmt.Sprintf("T%d", pool.tier))),
			muted(fmt.Sprintf("%d of %d online", up, len(pool.probes))))
		empty := muted("No nodes match")
		if len(pool.probes) == 0 {
			empty = muted("No nodes in this pool")
		}
		list := container.NewVBox()
		first := len(entries)
		for _, probe := range sortProbes(pool.probes, d.nodeSort) {
			row := d.nodeRow(probe, inUse[probe.Name])
			entries = append(entries, nodeEntry{row: container.NewVBox(widget.NewSeparator(), row), search: strings.ToLower(probe.Name + " " + probe.Server), ok: probe.OK})
			list.Add(entries[len(entries)-1].row)
		}
		poolEntries := entries[first:]
		refreshers = append(refreshers, func() int {
			shown := 0
			for _, entry := range poolEntries {
				if !entry.row.Visible() {
					continue
				}
				shown++
			}
			empty.Hidden = shown > 0
			list.Refresh()
			empty.Refresh()
			return shown
		})
		groups = append(groups, inset(container.NewVBox(head, list, empty), 6, 0, 0, 0))
	}

	summary := muted("")
	apply := func() {
		query := strings.ToLower(strings.TrimSpace(d.nodeQuery))
		for _, entry := range entries {
			entry.row.Hidden = (query != "" && !strings.Contains(entry.search, query)) || (d.nodesOnline && !entry.ok)
		}
		shown := 0
		for _, refresh := range refreshers {
			shown += refresh()
		}
		summary.Text = fmt.Sprintf("Showing %d of %d", shown, len(entries))
		summary.Refresh()
	}
	query := d.textInput("nodes", "Search by name or server", d.nodeQuery, func(value string) {
		d.nodeQuery = value
		apply()
	})
	onlineOnly := widget.NewCheck("Online only", func(on bool) {
		d.nodesOnline = on
		apply()
	})
	onlineOnly.Checked = d.nodesOnline
	sorter := widget.NewSelect([]string{sortRecommended, sortLatency, sortName, sortProvider}, func(mode string) {
		if mode != d.nodeSort {
			d.nodeSort = mode
			d.render()
		}
	})
	sorter.Selected = d.nodeSort
	controls := container.NewBorder(nil, nil, nil, container.NewHBox(onlineOnly, container.New(&widthLayout{width: 170}, sorter)), query)
	apply()

	items := []fyne.CanvasObject{controls}
	if pin := d.state.PinEntry; pin != "" {
		items = append(items, d.pinBanner(pin))
	}
	if sweep := d.status.Sweep; sweep.Node != "" {
		items = append(items, newFitText(fmt.Sprintf("Speed test %d / %d · testing %s", sweep.Done+1, sweep.Total, sweep.Node), colorMuted, 12, false))
	}
	items = append(items, groups...)
	items = append(items, summary)
	return section("Entry nodes", "Hops for T2/T3 routes. Pin a node to stop automatic selection.", nil, items...)
}

func (d *dashboard) pinBanner(pin string) fyne.CanvasObject {
	bg := colorWarning
	bg.A = 28
	box := canvas.NewRectangle(bg)
	box.CornerRadius = 6
	box.StrokeColor = colorWarning
	box.StrokeWidth = 1
	text := container.New(layout.NewCustomPaddedVBoxLayout(2),
		newFitText("Pinned: "+pin, colorText, 13, true),
		newFitText("Automatic entry selection is off for every slot", colorMuted, 12, false))
	unpin := widget.NewButtonWithIcon("Unpin", theme.CancelIcon(), func() { d.patch(map[string]any{"pin_entry": ""}) })
	return container.NewStack(box, inset(flexRow(text, unpin), 6, 6, 10, 6))
}

func (d *dashboard) nodeRow(probe engine.Probe, inUse bool) fyne.CanvasObject {
	pinned := d.state.PinEntry == probe.Name
	dotColor := color.Color(colorDanger)
	if probe.OK {
		dotColor = colorAccent
	}
	var tags []fyne.CanvasObject
	if inUse {
		tags = append(tags, tintChip("In use", colorAccent))
	}
	if pinned {
		tags = append(tags, tintChip("Pinned", colorWarning))
	}
	if d.status.Sweep.Node == probe.Name {
		tags = append(tags, tintChip("Testing speed…", branding.AccentBlue))
	}
	if probe.OK {
		tags = append(tags, neutralChip(milliseconds(probe.Latency)))
	} else {
		tags = append(tags, tintChip("Offline", colorDanger))
	}
	if probe.SpeedN > 0 {
		if probe.Speed < 1 {
			tags = append(tags, tintChip("<1 Mbps", colorDanger))
		} else {
			tags = append(tags, neutralChip(fmt.Sprintf("%.0f Mbps", probe.Speed)))
		}
	}
	if probe.Samples > 0 {
		tags = append(tags, flakeChip(probe.Flakiness))
	}
	nameColor := colorText
	if !probe.OK {
		nameColor = colorMuted
	}
	info := container.New(layout.NewCustomPaddedVBoxLayout(3),
		newFitText(probe.Name, nameColor, 13, inUse || pinned),
		container.NewBorder(nil, nil, container.NewHBox(tags...), nil, newFitText(probe.Server, colorMuted, 12, false)),
	)
	name := probe.Name
	share := d.iconButton(qrIcon(), "Share as VLESS link", func() {
		node := d.state.FindNodeByName(name)
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
	pin := widget.NewButton("Pin", func() { d.patch(map[string]any{"pin_entry": name}) })
	pin.Importance = widget.LowImportance
	if pinned {
		pin = widget.NewButton("Unpin", func() { d.patch(map[string]any{"pin_entry": ""}) })
	}
	return container.NewBorder(nil, nil, inset(statusDot(dotColor), 0, 0, 2, 10), container.NewHBox(share, pin), info)
}
