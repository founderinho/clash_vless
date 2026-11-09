//go:build !android && cgo

package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"

	"clashvless/branding"
)

var (
	colorBackground = branding.Background
	colorSidebar    = branding.Sidebar
	colorSurface    = branding.Surface
	colorControl    = branding.Control
	colorBorder     = branding.Border
	colorText       = branding.Text
	colorMuted      = branding.Muted
	colorAccent     = branding.Accent
	colorDanger     = branding.Danger
)

type nativeTheme struct{}

func (nativeTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return colorBackground
	case theme.ColorNameButton, theme.ColorNameInputBackground:
		return colorControl
	case theme.ColorNamePrimary, theme.ColorNameFocus, theme.ColorNameSelection, theme.ColorNameSuccess:
		return colorAccent
	case theme.ColorNameForeground:
		return colorText
	case theme.ColorNameForegroundOnPrimary:
		return colorBackground
	case theme.ColorNamePlaceHolder, theme.ColorNameDisabled:
		return colorMuted
	case theme.ColorNameHover:
		return colorBorder
	case theme.ColorNamePressed:
		return colorSurface
	case theme.ColorNameSeparator, theme.ColorNameInputBorder:
		return colorBorder
	case theme.ColorNameError:
		return colorDanger
	default:
		return theme.DefaultTheme().Color(name, theme.VariantDark)
	}
}

func (nativeTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (nativeTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (nativeTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 6
	case theme.SizeNameInnerPadding:
		return 6
	case theme.SizeNameText:
		return 13
	case theme.SizeNameHeadingText:
		return 20
	case theme.SizeNameSubHeadingText:
		return 15
	case theme.SizeNameCaptionText:
		return 11
	case theme.SizeNameInputRadius, theme.SizeNameButtonRadius, theme.SizeNameSelectionRadius:
		return 6
	case theme.SizeNameScrollBar:
		return 8
	default:
		return theme.DefaultTheme().Size(name)
	}
}
