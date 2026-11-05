package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/richtext"
)

// monoTheme is GoFolio's look: warm neutrals in both appearances, no hue.
func monoTheme(base *ui.Theme) *ui.Theme {
	t := *base
	if t.Dark {
		t.Background = ui.Hex("#1a1a19")
		t.Surface = ui.Hex("#141413")
		t.SurfaceHover = ui.Hex("#232321")
		t.SurfacePressed = ui.Hex("#2c2c29")
		t.Border = ui.Hex("#2e2e2b")
		t.Text = ui.Hex("#ecece7")
		t.TextMuted = ui.Hex("#8d8d85")
		t.Accent = ui.Hex("#deded8")
		t.AccentHover = ui.Hex("#f1f1ec")
		t.AccentPressed = ui.Hex("#ffffff")
		t.AccentText = ui.Hex("#1a1a19")
		t.Selection = ui.RGBA(236, 236, 231, 0.16)
		t.Focus = ui.Hex("#cfcfc8")
		t.Scrollbar = ui.RGBA(236, 236, 231, 0.20)
		t.Inverse = ui.Hex("#ecece7")
		t.InverseText = ui.Hex("#1a1a19")
	} else {
		t.Background = ui.Hex("#fcfcfb")
		t.Surface = ui.Hex("#f2f2ef")
		t.SurfaceHover = ui.Hex("#e8e8e4")
		t.SurfacePressed = ui.Hex("#dededa")
		t.Border = ui.Hex("#e2e2dd")
		t.Text = ui.Hex("#1b1b19")
		t.TextMuted = ui.Hex("#71716a")
		t.Accent = ui.Hex("#2b2b27")
		t.AccentHover = ui.Hex("#1b1b19")
		t.AccentPressed = ui.Hex("#0f0f0e")
		t.AccentText = ui.Hex("#fcfcfb")
		t.Selection = ui.RGBA(27, 27, 25, 0.15)
		t.Focus = ui.Hex("#1b1b19")
		t.Scrollbar = ui.RGBA(27, 27, 25, 0.18)
		t.Inverse = ui.Hex("#1b1b19")
		t.InverseText = ui.Hex("#fcfcfb")
	}
	t.Danger = t.TextMuted
	t.Warning = t.TextMuted
	t.Success = t.Text
	t.Radius = 7
	t.Spacing = 4
	t.FontSize = 13
	t.ScrollbarWidth = 10
	return &t
}

// faint is a third gray, quieter than TextMuted: hairlines, disabled hints,
// and the graph's dim link edges. Derived, so both appearances stay in step.
func faint(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.RGBA(236, 236, 231, 0.34)
	}
	return ui.RGBA(27, 27, 25, 0.34)
}

// monoEditorTheme colors Markdown source without the GitHub palette.
func monoEditorTheme(t *ui.Theme) *editor.Theme {
	et := &editor.Theme{
		Background:        t.Background,
		Text:              t.Text,
		LineNumber:        t.TextMuted.Alpha(0.55),
		ActiveLineNumber:  t.TextMuted,
		ActiveLine:        t.Text.Alpha(0.030),
		Selection:         t.Selection,
		InactiveSelection: t.Selection.Alpha(0.6),
		Cursor:            t.Accent,
		MatchingBracket:   t.TextMuted,
		Match:             t.Text.Alpha(0.10),
		CurrentMatch:      t.Text.Alpha(0.18),
		Occurrence:        t.Text.Alpha(0.07),
		IndentGuide:       t.Border,
		ActiveIndentGuide: t.TextMuted.Alpha(0.5),
		WrapGuide:         t.Border,
		Whitespace:        t.TextMuted.Alpha(0.4),
		Error:             t.Text,
		Warning:           t.TextMuted,
		Info:              t.TextMuted,
		Hint:              t.TextMuted,
		Added:             t.Text,
		Deleted:           t.TextMuted,
		Modified:          t.TextMuted,
	}
	bold := func(k editor.TokenKind) { et.Tokens[k] = editor.TokenStyle{Color: t.Text, Bold: true} }
	muted := func(k editor.TokenKind) { et.Tokens[k] = editor.TokenStyle{Color: t.TextMuted} }
	italic := func(k editor.TokenKind) { et.Tokens[k] = editor.TokenStyle{Color: t.TextMuted, Italic: true} }
	bold(editor.TokenKeyword)
	bold(editor.TokenHeading)
	bold(editor.TokenStrong)
	bold(editor.TokenTag)
	italic(editor.TokenEmphasis)
	italic(editor.TokenComment)
	muted(editor.TokenString)
	muted(editor.TokenEscape)
	muted(editor.TokenRegexp)
	muted(editor.TokenOperator)
	muted(editor.TokenPunctuation)
	muted(editor.TokenMeta)
	muted(editor.TokenLink)
	muted(editor.TokenCode)
	muted(editor.TokenAttribute)
	muted(editor.TokenVariable)
	muted(editor.TokenProperty)
	muted(editor.TokenInvalid)
	return et
}

// monoMarkdownTheme styles the rendered preview.
func monoMarkdownTheme(t *ui.Theme) *markdown.Theme {
	m := markdown.DefaultTheme(t)
	m.FontSize = 16
	m.LineHeight = 1.7
	m.CodeLineHeight = 1.55
	m.Link = t.TextMuted
	m.LinkWeight = 600
	m.Muted = t.TextMuted
	m.Rule = t.Border
	m.BlockSpacing = 0.9
	m.ItemSpacing = 0.3
	m.Headings[0].Size = 2.0
	m.Headings[0].Weight = 700
	m.Headings[0].Top = 1.0
	m.Headings[0].Bottom = 0.35
	m.Headings[1].Top = 1.0
	m.Headings[1].Bottom = 0.3
	m.QuoteBar = t.Border
	m.QuoteText = t.TextMuted
	m.CodeBackground = t.Text.Alpha(0.06)
	m.CodeBlockBackground = t.Text.Alpha(0.035)
	m.CodeBlockBorder = t.Border
	m.CodeColors = monoEditorTheme(t)
	m.TableBorder = t.Border
	m.TableHeader = t.Text.Alpha(0.035)
	return m
}

// monoRichtextTheme styles the live Markdown editor.
func monoRichtextTheme(t *ui.Theme) *richtext.Theme {
	r := richtext.DefaultTheme(t)
	r.Text = t.Text
	r.Muted = t.TextMuted
	r.Link = t.TextMuted
	r.Caret = t.Accent
	r.Selection = t.Selection
	r.Code = t.Text
	r.CodeBackground = t.Text.Alpha(0.06)
	r.CodeBlockBackground = t.Text.Alpha(0.035)
	r.Marker = t.TextMuted
	r.Checkbox = t.Text
	r.Checkmark = t.Background
	r.QuoteBar = t.Border
	r.QuoteText = t.TextMuted
	r.Padding = [2]float32{0, 0}
	r.Headings[0].Size = 1.9
	r.Headings[1].Size = 1.5
	r.Headings[2].Size = 1.25
	r.Headings[0].Top = 0.7
	r.Headings[1].Top = 0.9
	r.Headings[1].Bottom = 0.3
	r.ParagraphSpacing = 0.55
	return r
}
