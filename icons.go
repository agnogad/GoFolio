package main

import "github.com/egoist/mygo/ui"

const iconSVG = `xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"`

func icon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg ` + iconSVG + `>` + shapes + `</svg>`))
}

var (
	iconPlus               = icon(`<path d="M12 5v14M5 12h14"/>`)
	iconFolderPlus         = icon(`<path d="M3 7.5A1.5 1.5 0 0 1 4.5 6h4l2 2h7A1.5 1.5 0 0 1 19 9.5v8A1.5 1.5 0 0 1 17.5 19h-13A1.5 1.5 0 0 1 3 17.5z"/><path d="M11 11.5v5M8.5 14h5"/>`)
	iconSearch             = icon(`<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>`)
	iconSidebar            = icon(`<rect x="3" y="4.5" width="18" height="15" rx="2.5"/><path d="M9.5 4.5v15"/>`)
	iconRefresh            = icon(`<path d="M20 12a8 8 0 1 1-2.34-5.66"/><path d="M20 4v5h-5"/>`)
	iconX                  = icon(`<path d="M6 6l12 12M18 6 6 18"/>`)
	iconFile               = icon(`<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/>`)
	iconChevron            = icon(`<path d="m9 6 6 6-6 6"/>`)
	iconLink               = icon(`<path d="M10 13a5 5 0 0 0 7 0l2-2a5 5 0 0 0-7-7l-1 1"/><path d="M14 11a5 5 0 0 0-7 0l-2 2a5 5 0 0 0 7 7l1-1"/>`)
	iconFolder             = icon(`<path d="M3 7.5A1.5 1.5 0 0 1 4.5 6h4l2 2h7A1.5 1.5 0 0 1 19 9.5v8A1.5 1.5 0 0 1 17.5 19h-13A1.5 1.5 0 0 1 3 17.5z"/>`)
	iconMore               = icon(`<circle cx="5" cy="12" r="1.4" fill="currentColor" stroke="none"/><circle cx="12" cy="12" r="1.4" fill="currentColor" stroke="none"/><circle cx="19" cy="12" r="1.4" fill="currentColor" stroke="none"/>`)
	iconTrash              = icon(`<path d="M4 7h16M9 7V4.5h6V7M6.5 7l1 12.5h9L18 7"/>`)
	iconMinus              = icon(`<path d="M5 12h14"/>`)
	iconMax                = icon(`<rect x="5" y="5" width="14" height="14" rx="1"/>`)
	iconRestore            = icon(`<rect x="4.5" y="7.5" width="12" height="12" rx="1"/><path d="M8 7.5V4.5h11v11h-3"/>`)
	iconGraph              = icon(`<circle cx="6" cy="7" r="2.2"/><circle cx="18" cy="6" r="2.2"/><circle cx="17" cy="17.5" r="2.2"/><circle cx="7" cy="16.5" r="2.2"/><path d="M8 7.6 15.8 6.3M17.7 8 17.2 15.4M15 18 9 16.9M6.7 14.4 6.2 9.2"/>`)
	iconSettings           = icon(`<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .34 1.87l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.7 1.7 0 0 0-1.87-.34 1.7 1.7 0 0 0-1 1.55V21a2 2 0 1 1-4 0v-.09a1.7 1.7 0 0 0-1.11-1.55 1.7 1.7 0 0 0-1.87.34l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.7 1.7 0 0 0 .34-1.87 1.7 1.7 0 0 0-1.55-1H3a2 2 0 1 1 0-4h.09A1.7 1.7 0 0 0 4.64 8a1.7 1.7 0 0 0-.34-1.87l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.7 1.7 0 0 0 1.87.34H9a1.7 1.7 0 0 0 1-1.55V3a2 2 0 1 1 4 0v.09a1.7 1.7 0 0 0 1 1.55 1.7 1.7 0 0 0 1.87-.34l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.7 1.7 0 0 0-.34 1.87V9a1.7 1.7 0 0 0 1.55 1H21a2 2 0 1 1 0 4h-.09a1.7 1.7 0 0 0-1.55 1z"/>`)
	iconVaults             = icon(`<path d="M3 7.5A1.5 1.5 0 0 1 4.5 6h4l2 2h7A1.5 1.5 0 0 1 19 9.5v8A1.5 1.5 0 0 1 17.5 19h-13A1.5 1.5 0 0 1 3 17.5z"/><path d="M3 11h16"/>`)
	iconChevronDouble      = icon(`<path d="m11 7-5 5 5 5M18 7l-5 5 5 5"/>`)
	iconChevronDoubleRight = icon(`<path d="m13 7 5 5-5 5M6 7l5 5-5 5"/>`)
	iconSun                = icon(`<circle cx="12" cy="12" r="4.5"/><path d="M12 2v2.5M12 19.5V22M4.2 4.2l1.8 1.8M18 18l1.8 1.8M2 12h2.5M19.5 12H22M4.2 19.8 6 18M18 6l1.8-1.8"/>`)
	iconMoon               = icon(`<path d="M20 14.2A8 8 0 1 1 9.8 4a6.5 6.5 0 0 0 10.2 10.2z"/>`)
	iconCopy               = icon(`<rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>`)
	iconFit                = icon(`<path d="M4 9V5.5A1.5 1.5 0 0 1 5.5 4H9M15 4h3.5A1.5 1.5 0 0 1 20 5.5V9M20 15v3.5a1.5 1.5 0 0 1-1.5 1.5H15M9 20H5.5A1.5 1.5 0 0 1 4 18.5V15"/>`)
)
