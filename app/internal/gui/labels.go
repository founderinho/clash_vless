//go:build !android && cgo

package gui

import (
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

var colorWarning = color.NRGBA{R: 238, G: 190, B: 105, A: 255}

// fitText is a single line that ellipsizes to its width, so long names and hosts
// never widen the window. It cuts runes itself rather than using Label's truncator.
type fitText struct {
	widget.BaseWidget
	text  string
	color color.Color
	size  float32
	bold  bool
}

func newFitText(value string, c color.Color, size float32, bold bool) *fitText {
	t := &fitText{text: strings.Join(strings.Fields(value), " "), color: c, size: size, bold: bold}
	t.ExtendBaseWidget(t)
	return t
}

func (t *fitText) CreateRenderer() fyne.WidgetRenderer {
	text := canvas.NewText(t.text, t.color)
	text.TextSize = t.size
	text.TextStyle = fyne.TextStyle{Bold: t.bold}
	return &fitTextRenderer{owner: t, text: text}
}

type fitTextRenderer struct {
	owner *fitText
	text  *canvas.Text
}

func (r *fitTextRenderer) MinSize() fyne.Size {
	size := fyne.MeasureText("…", r.owner.size, r.text.TextStyle)
	return fyne.NewSize(min(size.Width*3, fyne.MeasureText(r.owner.text, r.owner.size, r.text.TextStyle).Width), size.Height)
}

func (r *fitTextRenderer) Layout(size fyne.Size) {
	r.text.Text = fitRunes(r.owner.text, size.Width, func(s string) float32 {
		return fyne.MeasureText(s, r.owner.size, r.text.TextStyle).Width
	})
	r.text.Resize(size)
	r.text.Refresh()
}

func (r *fitTextRenderer) Refresh() {
	r.text.Color = r.owner.color
	r.Layout(r.owner.Size())
}

func (r *fitTextRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.text} }
func (r *fitTextRenderer) Destroy()                     {}

// fitRunes returns value, or its longest prefix plus "…" that fits width.
func fitRunes(value string, width float32, measure func(string) float32) string {
	if value == "" || measure(value) <= width {
		return value
	}
	runes := []rune(value)
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if measure(strings.TrimRight(string(runes[:mid]), " ·")+"…") <= width {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return strings.TrimRight(string(runes[:lo]), " ·") + "…"
}

// chip is a small rounded tag for short facts: protocol, route, port.
func chip(value string, fg, bg color.Color) fyne.CanvasObject {
	text := canvas.NewText(value, fg)
	text.TextSize = 11
	text.TextStyle = fyne.TextStyle{Bold: true}
	box := canvas.NewRectangle(bg)
	box.CornerRadius = 4
	return container.NewCenter(container.NewStack(box, inset(text, 1, 1, 6, 6)))
}

func neutralChip(value string) fyne.CanvasObject {
	return chip(value, colorMuted, colorControl)
}

func tintChip(value string, c color.NRGBA) fyne.CanvasObject {
	bg := c
	bg.A = 40
	return chip(value, c, bg)
}

func statusDot(c color.Color) fyne.CanvasObject {
	dot := canvas.NewCircle(c)
	return container.NewCenter(container.NewGridWrap(fyne.NewSize(8, 8), dot))
}

func tierName(tier int) string {
	switch tier {
	case 1:
		return "Direct"
	case 2:
		return "Country hop"
	case 3:
		return "Bypass hop"
	}
	return ""
}

// titleWithTags puts tags right after a name, shrinking only the name when space runs out.
func titleWithTags(name *fitText, tags ...fyne.CanvasObject) fyne.CanvasObject {
	return container.New(&titleTagsLayout{}, append([]fyne.CanvasObject{name}, tags...)...)
}

type titleTagsLayout struct{}

const titleTagGap = 6

func (l *titleTagsLayout) tagsWidth(objects []fyne.CanvasObject) float32 {
	width := float32(0)
	for _, tag := range objects[1:] {
		width += tag.MinSize().Width + titleTagGap
	}
	return width
}

func (l *titleTagsLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	name := objects[0].(*fitText)
	natural := fyne.MeasureText(name.text, name.size, fyne.TextStyle{Bold: name.bold}).Width + 1
	nameWidth := max(name.MinSize().Width, min(natural, size.Width-l.tagsWidth(objects)))
	name.Resize(fyne.NewSize(nameWidth, size.Height))
	name.Move(fyne.NewPos(0, 0))
	x := nameWidth
	for _, tag := range objects[1:] {
		tagSize := tag.MinSize()
		x += titleTagGap
		tag.Resize(tagSize)
		tag.Move(fyne.NewPos(x, (size.Height-tagSize.Height)/2))
		x += tagSize.Width
	}
}

func (l *titleTagsLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	size := objects[0].MinSize()
	for _, tag := range objects[1:] {
		size.Height = max(size.Height, tag.MinSize().Height)
	}
	return fyne.NewSize(size.Width+l.tagsWidth(objects), size.Height)
}

// countBadge is a small red counter that hides itself at zero.
type countBadge struct {
	box  *fyne.Container
	text *canvas.Text
}

func newCountBadge() *countBadge {
	text := canvas.NewText("", colorBackground)
	text.TextSize = 10
	text.TextStyle = fyne.TextStyle{Bold: true}
	bg := canvas.NewRectangle(colorDanger)
	bg.CornerRadius = 8
	b := &countBadge{box: container.NewStack(bg, inset(container.NewCenter(text), 1, 1, 5, 5)), text: text}
	b.box.Hide()
	return b
}

func (b *countBadge) set(count int) {
	if b == nil {
		return
	}
	if count <= 0 {
		b.box.Hide()
		return
	}
	b.text.Text = fmt.Sprint(count)
	if count > 99 {
		b.text.Text = "99+"
	}
	b.text.Refresh()
	b.box.Show()
	b.box.Refresh()
}

// clickable makes any content a pointer-cursor tap target with a hover tint.
type clickable struct {
	widget.BaseWidget
	content fyne.CanvasObject
	onTap   func()
	bg      *canvas.Rectangle
}

func newClickable(content fyne.CanvasObject, tapped func()) *clickable {
	c := &clickable{content: content, onTap: tapped, bg: canvas.NewRectangle(color.Transparent)}
	c.bg.CornerRadius = 4
	c.ExtendBaseWidget(c)
	return c
}

func (c *clickable) Tapped(*fyne.PointEvent) {
	if c.onTap != nil {
		c.onTap()
	}
}

func (c *clickable) Cursor() desktop.Cursor { return desktop.PointerCursor }

func (c *clickable) MouseIn(*desktop.MouseEvent) {
	c.bg.FillColor = colorControl
	c.bg.Refresh()
}

func (c *clickable) MouseMoved(*desktop.MouseEvent) {}

func (c *clickable) MouseOut() {
	c.bg.FillColor = color.Transparent
	c.bg.Refresh()
}

func (c *clickable) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewStack(c.bg, inset(c.content, 4, 4, 4, 4)))
}
