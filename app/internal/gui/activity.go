//go:build !android && cgo

package gui

import (
	"fmt"
	"image/color"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"clashvless/internal/loghub"
)

var levelNames = map[loghub.Level]string{loghub.Info: "Info", loghub.Warning: "Warnings", loghub.Error: "Errors", loghub.Debug: "Debug"}

func levelTone(level loghub.Level) (badge, message color.Color, bg color.Color) {
	tint := func(c color.NRGBA) color.Color { c.A = 40; return c }
	switch level {
	case loghub.Error:
		return colorDanger, colorDanger, tint(colorDanger)
	case loghub.Warning:
		return colorWarning, colorText, tint(colorWarning)
	case loghub.Debug:
		return colorMuted, colorMuted, colorControl
	}
	return colorAccent, colorText, tint(colorAccent)
}

// fillPanel is a panel whose content takes all the space it is given.
func fillPanel(content fyne.CanvasObject) fyne.CanvasObject {
	background := canvas.NewRectangle(colorSurface)
	background.CornerRadius = 8
	background.StrokeColor = colorBorder
	background.StrokeWidth = 1
	return container.NewStack(background, inset(content, 10, 10, 12, 12))
}

func (d *dashboard) activityView() fyne.CanvasObject {
	visible := []loghub.Record{}
	summary := muted("")
	levelChecks := map[loghub.Level]*widget.Check{}
	var list *widget.List
	refresh := func() {
		visible = visible[:0]
		counts := map[loghub.Level]int{}
		for _, record := range d.logs {
			counts[record.Level]++
			if d.logFilter.Allows(record) {
				visible = append(visible, record)
			}
		}
		for level, check := range levelChecks {
			check.Text = fmt.Sprintf("%s %d", levelNames[level], counts[level])
			check.Refresh()
		}
		summary.Text = fmt.Sprintf("Showing %d of %d records · click one for details", len(visible), len(d.logs))
		summary.Refresh()
		if list != nil {
			list.Refresh()
		}
	}

	detail := container.NewVBox()
	showDetail := func(record *loghub.Record) {
		d.logDetail = record
		detail.Objects = nil
		if record == nil {
			detail.Refresh()
			return
		}
		badge, _, _ := levelTone(record.Level)
		head := newFitText(record.Time.Local().Format("15:04:05.000")+"  ·  "+record.LevelLabel()+"  ·  "+record.Origin(), badge, 12, true)
		message := widget.NewLabel(record.Message)
		message.Wrapping = fyne.TextWrapBreak
		message.Selectable = true
		line := record.String()
		actions := container.NewHBox(
			widget.NewButtonWithIcon("Copy", theme.ContentCopyIcon(), func() {
				d.app.Clipboard().SetContent(line)
				d.notify("Log record copied")
			}),
			d.iconButton(theme.CancelIcon(), "Close details", func() {
				d.logDetail = nil
				detail.Objects = nil
				detail.Refresh()
			}),
		)
		box := canvas.NewRectangle(colorBackground)
		box.CornerRadius = 6
		detail.Objects = []fyne.CanvasObject{inset(container.NewStack(box, inset(container.NewVBox(flexRow(head, actions), message), 6, 6, 10, 6)), 8, 0, 0, 0)}
		detail.Refresh()
	}

	list = widget.NewList(
		func() int { return len(visible) },
		func() fyne.CanvasObject { return newLogRow() },
		func(id widget.ListItemID, object fyne.CanvasObject) {
			if id < len(visible) {
				object.(*logRow).set(visible[id])
			}
		},
	)
	list.OnSelected = func(id widget.ListItemID) {
		if id < len(visible) {
			record := visible[id]
			showDetail(&record)
		}
		list.Unselect(id) // rows shift as records arrive; the details strip holds the choice
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
	module.Selected = "All modules"
	if d.logFilter.Module != "" {
		module.Selected = d.logFilter.Module
	}

	levels := container.NewHBox()
	for _, level := range []loghub.Level{loghub.Error, loghub.Warning, loghub.Info, loghub.Debug} {
		level := level
		check := widget.NewCheck(levelNames[level], func(on bool) {
			if d.logFilter.Levels == nil {
				d.logFilter.Levels = map[loghub.Level]bool{loghub.Info: true, loghub.Warning: true, loghub.Error: true, loghub.Debug: true}
			}
			d.logFilter.Levels[level] = on
			refresh()
		})
		check.Checked = d.logFilter.Shows(level)
		levelChecks[level] = check
		levels.Add(check)
	}
	follow := widget.NewCheck("Follow new", func(on bool) {
		d.pauseLog = !on
		if on {
			list.ScrollToBottom()
		}
	})
	follow.Checked = !d.pauseLog
	toolbar := panel("", "",
		container.NewBorder(nil, nil, nil, container.New(&widthLayout{width: 180}, module), query),
		flexRow(levels, follow),
	)

	copyVisible := widget.NewButtonWithIcon("Copy shown", theme.ContentCopyIcon(), func() {
		lines := make([]string, len(visible))
		for i, record := range visible {
			lines[i] = record.String()
		}
		d.app.Clipboard().SetContent(strings.Join(lines, "\n"))
		d.notify(fmt.Sprintf("%d records copied", len(lines)))
	})
	clear := widget.NewButtonWithIcon("Clear", theme.DeleteIcon(), func() {
		d.logs = nil
		showDetail(nil)
		refresh()
	})
	top := inset(flexRow(summary, container.NewHBox(copyVisible, clear)), 0, 6, 0, 0)
	records := fillPanel(container.NewBorder(top, detail, nil, nil, list))

	refresh()
	showDetail(d.logDetail)
	if !d.pauseLog {
		list.ScrollToBottom()
	}
	return container.NewBorder(toolbar, nil, nil, nil, inset(records, 8, 0, 0, 0))
}

// logRow draws one record in columns: time, level badge, origin, message.
type logRow struct {
	widget.BaseWidget
	record loghub.Record
}

func newLogRow() *logRow {
	row := &logRow{}
	row.ExtendBaseWidget(row)
	return row
}

func (r *logRow) set(record loghub.Record) {
	r.record = record
	r.Refresh()
}

func (r *logRow) CreateRenderer() fyne.WidgetRenderer {
	text := func(size float32, style fyne.TextStyle) *canvas.Text {
		t := canvas.NewText("", colorText)
		t.TextSize, t.TextStyle = size, style
		return t
	}
	badge := canvas.NewRectangle(colorControl)
	badge.CornerRadius = 4
	return &logRowRenderer{row: r, when: text(12, fyne.TextStyle{Monospace: true}), badge: badge,
		level: text(10, fyne.TextStyle{Bold: true}), origin: text(12, fyne.TextStyle{}), message: text(12, fyne.TextStyle{})}
}

type logRowRenderer struct {
	row                          *logRow
	when, level, origin, message *canvas.Text
	badge                        *canvas.Rectangle
	full                         [2]string // untruncated origin and message
}

const (
	logBadgeWidth = 48
	logGap        = 8
)

func (r *logRowRenderer) MinSize() fyne.Size { return fyne.NewSize(240, 22) }

func (r *logRowRenderer) Layout(size fyne.Size) {
	place := func(t *canvas.Text, x float32) {
		h := t.MinSize().Height
		t.Move(fyne.NewPos(x, (size.Height-h)/2))
		t.Resize(fyne.NewSize(t.MinSize().Width, h))
	}
	fit := func(t *canvas.Text, value string, width float32) {
		t.Text = fitRunes(value, max(0, width), func(s string) float32 {
			return fyne.MeasureText(s, t.TextSize, t.TextStyle).Width
		})
	}
	x := float32(0)
	place(r.when, x)
	x += fyne.MeasureText("00:00:00.000", 12, fyne.TextStyle{Monospace: true}).Width + logGap
	r.badge.Resize(fyne.NewSize(logBadgeWidth, 16))
	r.badge.Move(fyne.NewPos(x, (size.Height-16)/2))
	levelWidth := r.level.MinSize().Width
	place(r.level, x+(logBadgeWidth-levelWidth)/2)
	x += logBadgeWidth + logGap
	originWidth := float32(150)
	if size.Width < 640 {
		originWidth = 90
	}
	fit(r.origin, r.full[0], originWidth-logGap)
	place(r.origin, x)
	x += originWidth
	fit(r.message, r.full[1], size.Width-x)
	place(r.message, x)
}

func (r *logRowRenderer) Refresh() {
	record := r.row.record
	badge, message, bg := levelTone(record.Level)
	r.when.Text, r.when.Color = record.Time.Local().Format("15:04:05.000"), colorMuted
	r.level.Text, r.level.Color = record.LevelLabel(), badge
	r.badge.FillColor = bg
	r.origin.Color = colorMuted
	r.message.Color = message
	r.full = [2]string{record.Origin(), strings.Join(strings.Fields(record.Message), " ")}
	r.Layout(r.row.Size())
	for _, object := range r.Objects() {
		object.Refresh()
	}
}

func (r *logRowRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.when, r.badge, r.level, r.origin, r.message}
}

func (r *logRowRenderer) Destroy() {}
