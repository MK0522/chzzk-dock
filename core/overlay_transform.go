package core

import (
	"unsafe"

	"github.com/wailsapp/go-webview2/pkg/edge"
)

const (
	minOverlayWidth  = 100
	minOverlayHeight = 80
	maxOverlayCrop   = 8192
)

var (
	getOverlayClientRect = user32.NewProc("GetClientRect")
	getOverlayKeyState   = user32.NewProc("GetKeyState")
)

type overlayCrop struct {
	Left, Top, Right, Bottom int32
}

func (c *overlayCrop) normalized() overlayCrop {
	if c == nil {
		return overlayCrop{}
	}
	return overlayCrop{
		Left: min(max(c.Left, 0), maxOverlayCrop), Top: min(max(c.Top, 0), maxOverlayCrop),
		Right: min(max(c.Right, 0), maxOverlayCrop), Bottom: min(max(c.Bottom, 0), maxOverlayCrop),
	}
}

type overlayCropDrag struct {
	bounds RECT
	crop   overlayCrop
}

type overlayResizeDrag struct {
	hit      uintptr
	origin   POINT
	start    RECT
	last     RECT
	crop     *overlayCropDrag
	origCrop *overlayCrop
}

func overlayResizeEdge(hit uintptr) uintptr {
	if hit >= 10 && hit <= 17 {
		return hit - 9
	}
	return 0
}

func overlayResizeCursor(hit uintptr) uintptr {
	switch hit {
	case 10, 11: // HTLEFT, HTRIGHT
		return 32644 // IDC_SIZEWE
	case 12, 15: // HTTOP, HTBOTTOM
		return 32645 // IDC_SIZENS
	case 13, 17: // HTTOPLEFT, HTBOTTOMRIGHT
		return 32642 // IDC_SIZENWSE
	case 14, 16: // HTTOPRIGHT, HTBOTTOMLEFT
		return 32643 // IDC_SIZENESW
	default:
		return 32512 // IDC_ARROW
	}
}

func overlayResizeRect(start RECT, origin, cursor POINT, hit uintptr) RECT {
	dx := cursor.X - origin.X
	dy := cursor.Y - origin.Y
	r := start
	switch hit {
	case 10, 13, 16: // HTLEFT, HTTOPLEFT, HTBOTTOMLEFT
		r.Left = min(start.Left+dx, start.Right-minOverlayWidth)
	case 11, 14, 17: // HTRIGHT, HTTOPRIGHT, HTBOTTOMRIGHT
		r.Right = max(start.Right+dx, start.Left+minOverlayWidth)
	}
	switch hit {
	case 12, 13, 14: // HTTOP, HTTOPLEFT, HTTOPRIGHT
		r.Top = min(start.Top+dy, start.Bottom-minOverlayHeight)
	case 15, 16, 17: // HTBOTTOM, HTBOTTOMLEFT, HTBOTTOMRIGHT
		r.Bottom = max(start.Bottom+dy, start.Top+minOverlayHeight)
	}
	return r
}

func (d overlayCropDrag) sizing(edge uintptr, proposed *RECT) overlayCrop {
	crop, start := d.crop, d.bounds
	// Moving a visible edge changes its crop by the same amount. The
	// WebView viewport retains its original dimensions for the whole drag.
	switch edge {
	case 1, 4, 7:
		crop.Left = min(max(crop.Left+proposed.Left-start.Left, 0), maxOverlayCrop)
		crop.Left = min(crop.Left, d.crop.Left+max(0, start.Right-start.Left-minOverlayWidth))
		proposed.Left = start.Left + crop.Left - d.crop.Left
	case 2, 5, 8:
		crop.Right = min(max(crop.Right+start.Right-proposed.Right, 0), maxOverlayCrop)
		crop.Right = min(crop.Right, d.crop.Right+max(0, start.Right-start.Left-minOverlayWidth))
		proposed.Right = start.Right - crop.Right + d.crop.Right
	}
	switch edge {
	case 3, 4, 5:
		crop.Top = min(max(crop.Top+proposed.Top-start.Top, 0), maxOverlayCrop)
		crop.Top = min(crop.Top, d.crop.Top+max(0, start.Bottom-start.Top-minOverlayHeight))
		proposed.Top = start.Top + crop.Top - d.crop.Top
	case 6, 7, 8:
		crop.Bottom = min(max(crop.Bottom+start.Bottom-proposed.Bottom, 0), maxOverlayCrop)
		crop.Bottom = min(crop.Bottom, d.crop.Bottom+max(0, start.Bottom-start.Top-minOverlayHeight))
		proposed.Bottom = start.Bottom - crop.Bottom + d.crop.Bottom
	}
	return crop
}

func (o *chatOverlayWindow) resizeWebView() {
	var r RECT
	if ok, _, _ := getOverlayClientRect.Call(o.hwnd, uintptr(unsafe.Pointer(&r))); ok == 0 {
		return
	}
	if r.Right <= 0 || r.Bottom <= 0 {
		return
	}
	crop := o.state.OverlayCrop.normalized()
	bounds := edge.Rect{
		Left: -crop.Left, Top: -crop.Top,
		Right: r.Right + crop.Right, Bottom: r.Bottom + crop.Bottom,
	}
	if bounds != o.lastViewport {
		o.lastViewport = bounds
		o.chromium.ResizeWithBounds(&bounds)
	}
	if crop != o.lastCrop {
		o.lastCrop = crop
		o.updatePage()
	}
}
