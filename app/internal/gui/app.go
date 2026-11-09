//go:build !android && cgo

package gui

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"clashvless/branding"
	"clashvless/internal/control"
	"clashvless/internal/engine"
	"clashvless/internal/loghub"
	"clashvless/internal/store"
)

type dashboard struct {
	logRefresh      func() // re-filters the Activity list; only valid while that page is shown
	scroll          *container.Scroll
	scrollPage      string
	inputs          map[string]*widget.Entry
	app             fyne.App
	window          fyne.Window
	client          *control.Client
	embedded        bool
	state           *store.State
	status          engine.Status
	logs            []loghub.Record
	logFilter       loghub.Filter
	logList         *widget.List
	page            string
	settingsSection string
	nodeQuery       string
	content         *fyne.Container
	notice          *widget.Label
	nav             map[string]*widget.Button
}

func RunClient(client *control.Client, embedded bool) error {
	a := app.NewWithID("com.clashvless")
	a.Settings().SetTheme(nativeTheme{})
	a.SetIcon(logoResource())
	w := a.NewWindow("Clashvless")
	w.SetIcon(logoResource())
	w.SetPadded(false)
	w.Resize(fitScreen(fyne.NewSize(900, 640)))
	w.CenterOnScreen()
	d := &dashboard{
		app: a, window: w, client: client, embedded: embedded,
		page: "overview", settingsSection: "Network & failover", nav: map[string]*widget.Button{},
		logFilter: loghub.DefaultFilter(), inputs: map[string]*widget.Entry{},
	}
	d.buildShell()
	go d.readEvents()
	w.ShowAndRun()
	return nil
}

func (d *dashboard) readEvents() {
	for event := range d.client.Events() {
		event := event
		fyne.Do(func() {
			switch event.Type {
			case "state":
				var state store.State
				if json.Unmarshal(event.State, &state) == nil {
					d.state = &state
					d.render()
				}
			case "status":
				if event.Status != nil {
					d.status = *event.Status
					if d.page != "settings" && d.page != "activity" {
						d.render()
					}
				}
			case "log":
				record := loghub.ParseLine(event.Line)
				if event.Record != nil {
					record = *event.Record
				}
				d.logs = append(d.logs, record)
				if len(d.logs) > 2000 {
					d.logs = d.logs[len(d.logs)-2000:]
				}
				if d.page == "activity" && d.logRefresh != nil {
					d.logRefresh()
					d.logList.ScrollToBottom()
				}
			}
		})
	}
	fyne.Do(func() { d.notice.SetText("Daemon disconnected") })
}

func (d *dashboard) buildShell() {
	logo := canvas.NewImageFromResource(logoResource())
	logo.FillMode = canvas.ImageFillContain
	logo.SetMinSize(fyne.NewSize(30, 30))
	brand := container.NewHBox(logo, headingText(branding.Name, 16))
	items := make([]fyne.CanvasObject, 0, 5)
	for _, item := range []struct {
		page string
		name string
		icon string
	}{
		{"overview", "Overview", "overview"},
		{"subscriptions", "Subscriptions", "subscriptions"},
		{"routes", "Exit routes", "routes"},
		{"activity", "Activity", "activity"},
		{"settings", "Settings", "settings"},
	} {
		page := item.page
		button := widget.NewButtonWithIcon(item.name, brandIcon(item.icon), func() { d.navigate(page) })
		button.Alignment = widget.ButtonAlignLeading
		button.Importance = widget.LowImportance
		d.nav[page] = button
		items = append(items, button)
	}
	mode := "Attached to daemon"
	if d.embedded {
		mode = "Local engine"
	}
	footer := container.NewVBox(
		widget.NewSeparator(),
		muted(mode),
		muted("Version "+store.Version),
	)
	sidebarContent := container.NewBorder(inset(brand, 18, 14, 12, 12), inset(footer, 10, 12, 12, 12), nil, nil,
		inset(container.NewVBox(items...), 8, 8, 8, 8))
	background := canvas.NewRectangle(colorSidebar)
	sidebar := container.New(&widthLayout{width: 154}, container.NewStack(background, sidebarContent))

	d.notice = widget.NewLabel("Ready")
	d.notice.Truncation = fyne.TextTruncateEllipsis
	d.notice.SizeName = theme.SizeNameCaptionText
	d.content = container.NewStack(container.NewCenter(widget.NewLabel("Starting local engine…")))
	statusBar := container.NewStack(
		canvas.NewRectangle(colorSidebar),
		inset(d.notice, 4, 12, 12, 12),
	)
	main := container.NewBorder(nil, statusBar, nil, nil, d.content)
	d.window.SetContent(container.NewBorder(nil, nil, sidebar, nil, main))
}

func (d *dashboard) navigate(page string) {
	d.page = page
	d.render()
}

func (d *dashboard) render() {
	if d.state == nil {
		return
	}
	prevScroll, prevPage := d.scroll, d.scrollPage
	focused := ""
	for key, entry := range d.inputs {
		if d.window.Canvas().Focused() == entry {
			focused = key
		}
	}
	d.inputs = map[string]*widget.Entry{}
	for name, button := range d.nav {
		button.Importance = widget.LowImportance
		if name == d.page {
			button.Importance = widget.HighImportance
		}
		button.Refresh()
	}
	titles := map[string][2]string{
		"overview":      {"Overview", "Connection and local proxy"},
		"subscriptions": {"Subscriptions", "Providers and entry nodes"},
		"routes":        {"Exit routes", "Failover slots and VLESS exits"},
		"activity":      {"Activity", "Live engine events"},
		"settings":      {"Settings", "Application preferences"},
	}
	var body fyne.CanvasObject
	switch d.page {
	case "subscriptions":
		body = d.subscriptionsView()
	case "routes":
		body = d.routesView()
	case "activity":
		body = d.activityView()
	case "settings":
		body = d.settingsView()
	default:
		body = d.overviewView()
	}
	recheck := widget.NewButtonWithIcon("Recheck", theme.ViewRefreshIcon(), func() {
		d.command(control.Command{Cmd: "kick"}, "Route check started")
	})
	title := container.NewVBox(headingText(titles[d.page][0], 20), muted(titles[d.page][1]))
	headerContent := container.NewVBox(container.NewBorder(nil, nil, title, recheck))
	headerBottomPadding := float32(16)
	if d.page == "routes" {
		headerContent.Add(inset(routingCompatibilityPanel(), 6, 0, 0, 0))
		headerBottomPadding = 8
	}
	header := container.NewStack(
		canvas.NewRectangle(colorSidebar),
		inset(headerContent, 10, headerBottomPadding, 16, 16),
	)
	scroll := container.NewVScroll(inset(body, 12, 16, 16, 16))
	d.content.Objects = []fyne.CanvasObject{container.NewBorder(header, nil, nil, nil, scroll)}
	d.content.Refresh()
	if d.page == prevPage && prevScroll != nil {
		scroll.ScrollToOffset(prevScroll.Offset)
	}
	d.scroll, d.scrollPage = scroll, d.page
	if entry := d.inputs[focused]; entry != nil {
		entry.CursorColumn = len([]rune(entry.Text))
		d.window.Canvas().Focus(entry)
	}
}

// textInput is an entry that survives render(), which rebuilds the page on every
// status event: changed must store the text so the next build can restore it.
func (d *dashboard) textInput(key, placeholder, text string, changed func(string)) *widget.Entry {
	entry := widget.NewEntry()
	entry.SetPlaceHolder(placeholder)
	entry.SetText(text)
	entry.OnChanged = changed
	d.inputs[key] = entry
	return entry
}

func (d *dashboard) command(cmd control.Command, message string) {
	if err := d.client.Send(cmd); err != nil {
		d.notify("Command failed: " + err.Error())
		return
	}
	d.notify(message)
}

func (d *dashboard) patch(values map[string]any) {
	data, err := json.Marshal(values)
	if err != nil {
		d.notify("Could not save: " + err.Error())
		return
	}
	if err := d.client.SendPatch(data); err != nil {
		d.notify("Could not save: " + err.Error())
		return
	}
	d.notify("Saved")
}

func (d *dashboard) notify(message string) {
	d.notice.SetText(message)
}

func (d *dashboard) updateSlots(mutate func([]store.Slot)) {
	if d.state == nil {
		return
	}
	data, _ := json.Marshal(d.state.Slots)
	var slots []store.Slot
	_ = json.Unmarshal(data, &slots)
	mutate(slots)
	d.patch(map[string]any{"slots": slots})
}

func (d *dashboard) ask(title, label, initial, confirm string, apply func(string)) {
	entry := widget.NewEntry()
	entry.SetText(initial)
	form := dialog.NewForm(title, confirm, "Cancel", []*widget.FormItem{widget.NewFormItem(label, entry)}, func(ok bool) {
		if ok {
			apply(strings.TrimSpace(entry.Text))
		}
	}, d.window)
	form.Resize(fyne.NewSize(520, 150))
	form.Show()
}

func (d *dashboard) confirm(title, message string, apply func()) {
	dialog.ShowConfirm(title, message, func(ok bool) {
		if ok {
			apply()
		}
	}, d.window)
}

func (d *dashboard) addSubscription() {
	d.ask("Add subscription", "Subscription URL", "", "Add", func(value string) {
		if !strings.HasPrefix(strings.ToLower(value), "http://") && !strings.HasPrefix(strings.ToLower(value), "https://") {
			d.notify("Enter a valid http(s) subscription URL")
			return
		}
		d.command(control.Command{Cmd: "addsub", URL: value}, "Subscription added; fetching nodes…")
	})
}

func (d *dashboard) addMain(slotIndex int) {
	d.ask("Add final exit", "VLESS URL", "", "Add", func(value string) {
		parsed, err := url.Parse(value)
		if err != nil || parsed.Scheme != "vless" || parsed.Hostname() == "" {
			d.notify("Enter a valid vless:// URL")
			return
		}
		d.updateSlots(func(slots []store.Slot) {
			if slotIndex >= 0 && slotIndex < len(slots) {
				slots[slotIndex].AddMain(value)
			}
		})
	})
}

func (d *dashboard) addSlot() {
	d.ask("Add slot", "Slot name", fmt.Sprintf("slot-%d", len(d.state.Slots)+1), "Add", func(name string) {
		if name == "" || d.state.SlotIndexByName(name) >= 0 {
			d.notify("Enter a unique slot name")
			return
		}
		data, _ := json.Marshal(d.state.Slots)
		var slots []store.Slot
		_ = json.Unmarshal(data, &slots)
		slots = append(slots, store.Slot{Name: name, Port: d.state.FreeSlotPort(), Enabled: true})
		d.patch(map[string]any{"slots": slots})
	})
}

func (d *dashboard) removeMain(slotIndex, mainIndex int) {
	slotName, name := d.state.Slots[slotIndex].Name, d.state.Slots[slotIndex].Mains[mainIndex].Name
	d.confirm("Delete exit", "Remove "+name+" from this failover slot?", func() {
		d.updateSlots(func(slots []store.Slot) {
			// Another client may have changed the slots while this dialog was open.
			if slotIndex < len(slots) && slots[slotIndex].Name == slotName && mainIndex < len(slots[slotIndex].Mains) && slots[slotIndex].Mains[mainIndex].Name == name {
				slots[slotIndex].RemoveMain(mainIndex)
			}
		})
	})
}

func logoResource() fyne.Resource {
	data, _ := branding.Logo()
	return fyne.NewStaticResource("clashvless-logo.svg", data)
}

func brandIcon(name string) fyne.Resource {
	data, _ := branding.Icon(name)
	return theme.NewThemedResource(fyne.NewStaticResource("clashvless-"+name+".svg", data))
}

func headingText(value string, size float32) *canvas.Text {
	text := canvas.NewText(value, colorText)
	text.TextSize = size
	text.TextStyle = fyne.TextStyle{Bold: true}
	return text
}

func strong(value string) *canvas.Text {
	return headingText(value, 14)
}

func muted(value string) *canvas.Text {
	text := canvas.NewText(value, colorMuted)
	text.TextSize = 11
	return text
}

func wrappingMuted(value string) *widget.Label {
	label := widget.NewLabel(value)
	label.Wrapping = fyne.TextWrapWord
	label.SizeName = theme.SizeNameCaptionText
	return label
}

func ellipsized(value string) *widget.Label {
	label := widget.NewLabel(value)
	label.Truncation = fyne.TextTruncateEllipsis
	return label
}

func statusText(value string, ok bool) *canvas.Text {
	c := colorDanger
	if ok {
		c = colorAccent
	}
	text := canvas.NewText(value, c)
	text.TextSize = 20
	text.TextStyle = fyne.TextStyle{Bold: true}
	return text
}

func panel(title, subtitle string, objects ...fyne.CanvasObject) fyne.CanvasObject {
	items := make([]fyne.CanvasObject, 0, len(objects)+2)
	if title != "" {
		items = append(items, strong(title))
	}
	if subtitle != "" {
		items = append(items, wrappingMuted(subtitle))
	}
	items = append(items, objects...)
	content := inset(container.NewVBox(items...), 11, 11, 12, 12)
	background := canvas.NewRectangle(colorSurface)
	background.CornerRadius = 8
	background.StrokeColor = colorBorder
	background.StrokeWidth = 1
	return container.NewStack(background, content)
}

// routingCompatibilityPanel keeps the warning visible above the scrollable routes.
func routingCompatibilityPanel() fyne.CanvasObject {
	border := canvas.NewRectangle(colorSurface)
	border.StrokeColor = colorDanger
	border.StrokeWidth = 1
	border.CornerRadius = 3
	mark := canvas.NewText("!", colorDanger)
	mark.TextStyle = fyne.TextStyle{Bold: true}
	mark.TextSize = 13
	icon := container.NewGridWrap(fyne.NewSize(20, 20), container.NewStack(border, container.NewCenter(mark)))
	title := container.NewBorder(nil, nil, nil, icon, strong("Routing compatibility"))
	return panel("", "", title, wrappingMuted("Vision exits are direct-only. REALITY exits require a non-Vision hop; plain exits support any hop."))
}

func metric(name, value string) fyne.CanvasObject {
	return container.NewVBox(muted(name), headingText(value, 17))
}

func inset(object fyne.CanvasObject, top, bottom, left, right float32) *fyne.Container {
	return container.New(layout.NewCustomPaddedLayout(top, bottom, left, right), object)
}

func flexRow(primary, accessory fyne.CanvasObject) *fyne.Container {
	return container.New(&flexLayout{}, primary, accessory)
}

type flexLayout struct{}

func (l *flexLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) != 2 {
		return
	}
	accessorySize := objects[1].MinSize()
	primaryWidth := max(0, size.Width-accessorySize.Width-10)
	primarySize := fyne.NewSize(primaryWidth, objects[0].MinSize().Height)
	objects[0].Resize(primarySize)
	objects[0].Move(fyne.NewPos(0, (size.Height-primarySize.Height)/2))
	objects[1].Resize(accessorySize)
	objects[1].Move(fyne.NewPos(size.Width-accessorySize.Width, (size.Height-accessorySize.Height)/2))
}

func (l *flexLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) != 2 {
		return fyne.Size{}
	}
	first, second := objects[0].MinSize(), objects[1].MinSize()
	return fyne.NewSize(first.Width+second.Width+10, max(first.Height, second.Height))
}

type widthLayout struct {
	width float32
}

func (l *widthLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, object := range objects {
		object.Resize(size)
	}
}

func (l *widthLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	height := float32(0)
	for _, object := range objects {
		height = max(height, object.MinSize().Height)
	}
	return fyne.NewSize(l.width, height)
}

func milliseconds(duration time.Duration) string {
	if duration <= 0 {
		return "—"
	}
	return strconv.FormatInt(duration.Milliseconds(), 10) + " ms"
}

func mainHost(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "Invalid VLESS URL"
	}
	return parsed.Host
}
