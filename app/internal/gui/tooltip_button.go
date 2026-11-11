//go:build !android && cgo

package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

type tooltipIconButton struct {
	widget.BaseWidget
	Icon     fyne.Resource
	OnTapped func()
	OnHover  func(bool)
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
	if b.OnTapped != nil {
		b.OnTapped()
	}
}

func (b *tooltipIconButton) Cursor() desktop.Cursor {
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
	return &tooltipIconButtonRenderer{
		btn: b,
		bg:  bg,
		img: img,
	}
}

type tooltipIconButtonRenderer struct {
	btn *tooltipIconButton
	bg  *canvas.Rectangle
	img *canvas.Image
}

func (r *tooltipIconButtonRenderer) MinSize() fyne.Size {
	return fyne.NewSize(24, 24)
}

func (r *tooltipIconButtonRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)
	pad := float32(4)
	iconSize := fyne.NewSize(size.Width-pad*2, size.Height-pad*2)
	r.img.Resize(iconSize)
	r.img.Move(fyne.NewPos(pad, pad))
}

func (r *tooltipIconButtonRenderer) Refresh() {
	if r.btn.hovered {
		r.bg.FillColor = colorControl
	} else {
		r.bg.FillColor = color.Transparent
	}
	r.bg.Refresh()
	r.img.Resource = r.btn.Icon
	r.img.Refresh()
}

func (r *tooltipIconButtonRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.bg, r.img}
}

func (r *tooltipIconButtonRenderer) Destroy() {}
