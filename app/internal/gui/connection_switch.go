//go:build !android && cgo

package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

type connectionSwitch struct {
	widget.BaseWidget
	Checked   bool
	OnChanged func(bool)
}

func newConnectionSwitch(checked bool, changed func(bool)) *connectionSwitch {
	control := &connectionSwitch{Checked: checked, OnChanged: changed}
	control.ExtendBaseWidget(control)
	return control
}

func (c *connectionSwitch) Tapped(*fyne.PointEvent) {
	c.Checked = !c.Checked
	c.Refresh()
	if c.OnChanged != nil {
		c.OnChanged(c.Checked)
	}
}

func (c *connectionSwitch) Cursor() desktop.Cursor {
	return desktop.PointerCursor
}

func (c *connectionSwitch) CreateRenderer() fyne.WidgetRenderer {
	track := canvas.NewRectangle(colorControl)
	thumb := canvas.NewRectangle(colorMuted)
	renderer := &connectionSwitchRenderer{control: c, track: track, thumb: thumb}
	renderer.Refresh()
	return renderer
}

type connectionSwitchRenderer struct {
	control *connectionSwitch
	track   *canvas.Rectangle
	thumb   *canvas.Rectangle
}

func (r *connectionSwitchRenderer) Layout(size fyne.Size) {
	r.track.Resize(size)
	r.track.CornerRadius = size.Height / 2
	thumbSize := fyne.NewSize(size.Height-8, size.Height-8)
	r.thumb.Resize(thumbSize)
	r.thumb.CornerRadius = thumbSize.Height / 2
	x := float32(4)
	if r.control.Checked {
		x = size.Width - thumbSize.Width - 4
	}
	r.thumb.Move(fyne.NewPos(x, 4))
}

func (r *connectionSwitchRenderer) MinSize() fyne.Size {
	return fyne.NewSize(56, 32)
}

func (r *connectionSwitchRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.track, r.thumb}
}

func (r *connectionSwitchRenderer) Refresh() {
	if r.control.Checked {
		r.track.FillColor = colorAccent
		r.thumb.FillColor = colorBackground
	} else {
		r.track.FillColor = colorBorder
		r.thumb.FillColor = colorMuted
	}
	if size := r.control.Size(); size.Width > 0 && size.Height > 0 {
		r.Layout(size)
	}
	r.track.Refresh()
	r.thumb.Refresh()
}

func (r *connectionSwitchRenderer) Destroy() {}
