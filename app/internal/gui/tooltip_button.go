//go:build !android && cgo

package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type tooltipIconButton struct {
	widget.BaseWidget
	Icon     fyne.Resource
	Hint     string
	OnTapped func()
	OnHover  func(bool)
	Disabled bool
	hovered  bool
}

func newTooltipIconButton(icon fyne.Resource, tapped func(), hover func(bool)) *tooltipIconButton {
	b := &tooltipIconButton{
		Icon:     icon,
		OnTapped: tapped,
		OnHover:  hover,
	}
	b.ExtendBaseWidget(b)
	return b
}

func (b *tooltipIconButton) Tapped(*fyne.PointEvent) {
	if b.OnTapped != nil && !b.Disabled {
		b.OnTapped()
	}
}

func (b *tooltipIconButton) Cursor() desktop.Cursor {
	if b.Disabled {
		return desktop.DefaultCursor
	}
	return desktop.PointerCursor
}

func (b *tooltipIconButton) MouseIn(*desktop.MouseEvent) {
	b.hovered = true
	b.Refresh()
	if b.OnHover != nil {
		b.OnHover(true)
	}
}

func (b *tooltipIconButton) MouseMoved(*desktop.MouseEvent) {}

func (b *tooltipIconButton) MouseOut() {
	b.hovered = false
	b.Refresh()
	if b.OnHover != nil {
		b.OnHover(false)
	}
}

func (b *tooltipIconButton) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(color.Transparent)
	bg.CornerRadius = 4
	img := canvas.NewImageFromResource(b.Icon)
	img.FillMode = canvas.ImageFillContain
	r := &tooltipIconButtonRenderer{btn: b, bg: bg, img: img}
	r.Refresh()
	return r
}

type tooltipIconButtonRenderer struct {
	btn *tooltipIconButton
	bg  *canvas.Rectangle
	img *canvas.Image
}

func (r *tooltipIconButtonRenderer) MinSize() fyne.Size {
	return fyne.NewSize(26, 26)
}

func (r *tooltipIconButtonRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)
	pad := float32(4)
	iconSize := fyne.NewSize(size.Width-pad*2, size.Height-pad*2)
	r.img.Resize(iconSize)
	r.img.Move(fyne.NewPos(pad, pad))
}

func (r *tooltipIconButtonRenderer) Refresh() {
	if r.btn.hovered && !r.btn.Disabled {
		r.bg.FillColor = colorControl
	} else {
		r.bg.FillColor = color.Transparent
	}
	r.bg.Refresh()
	if r.btn.Disabled {
		r.img.Resource = theme.NewDisabledResource(r.btn.Icon)
	} else {
		r.img.Resource = r.btn.Icon
	}
	r.img.Refresh()
}

func (r *tooltipIconButtonRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.bg, r.img}
}

func (r *tooltipIconButtonRenderer) Destroy() {}

// tooltipLayer floats hints above the page. Plain canvas objects are never hit
// targets, so the layer doesn't steal clicks or hover from the widgets below.
type tooltipLayer struct {
	layer *fyne.Container
	pill  *fyne.Container
	text  *canvas.Text
	owner fyne.CanvasObject
}

func newTooltipLayer() *tooltipLayer {
	bg := canvas.NewRectangle(colorControl)
	bg.CornerRadius = 4
	bg.StrokeColor = colorBorder
	bg.StrokeWidth = 1
	text := canvas.NewText("", colorText)
	text.TextSize = 11
	pill := container.NewStack(bg, inset(text, 3, 3, 7, 7))
	pill.Hide()
	return &tooltipLayer{layer: container.NewWithoutLayout(pill), pill: pill, text: text}
}

func (t *tooltipLayer) show(owner fyne.CanvasObject, hint string) {
	app := fyne.CurrentApp()
	if app == nil || hint == "" {
		return
	}
	t.owner = owner
	t.text.Text = hint
	t.text.Refresh()
	size := t.pill.MinSize()
	t.pill.Resize(size)
	pos := app.Driver().AbsolutePositionForObject(owner)
	ownerSize := owner.Size()
	area := t.layer.Size()
	x := pos.X + ownerSize.Width/2 - size.Width/2
	y := pos.Y + ownerSize.Height + 4
	if area.Height > 0 && y+size.Height > area.Height {
		y = pos.Y - size.Height - 4
	}
	if area.Width > 0 {
		x = min(x, area.Width-size.Width-4)
	}
	t.pill.Move(fyne.NewPos(max(4, x), max(0, y)))
	t.pill.Show()
}

func (t *tooltipLayer) hide(owner fyne.CanvasObject) {
	if owner == nil || owner == t.owner {
		t.owner = nil
		t.pill.Hide()
	}
}
