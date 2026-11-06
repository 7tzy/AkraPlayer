//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")

	pRegisterClassExW              = user32.NewProc("RegisterClassExW")
	pCreateWindowExW               = user32.NewProc("CreateWindowExW")
	pDefWindowProcW                = user32.NewProc("DefWindowProcW")
	pGetMessageW                   = user32.NewProc("GetMessageW")
	pTranslateMessage              = user32.NewProc("TranslateMessage")
	pDispatchMessageW              = user32.NewProc("DispatchMessageW")
	pPostQuitMessage               = user32.NewProc("PostQuitMessage")
	pShowWindow                    = user32.NewProc("ShowWindow")
	pSetForegroundWindow           = user32.NewProc("SetForegroundWindow")
	pUpdateLayeredWindow           = user32.NewProc("UpdateLayeredWindow")
	pSetWindowPos                  = user32.NewProc("SetWindowPos")
	pGetWindowRect                 = user32.NewProc("GetWindowRect")
	pSetTimer                      = user32.NewProc("SetTimer")
	pKillTimer                     = user32.NewProc("KillTimer")
	pRegisterHotKey                = user32.NewProc("RegisterHotKey")
	pUnregisterHotKey              = user32.NewProc("UnregisterHotKey")
	pGetKeyState                   = user32.NewProc("GetKeyState")
	pSetCapture                    = user32.NewProc("SetCapture")
	pReleaseCapture                = user32.NewProc("ReleaseCapture")
	pTrackMouseEvent               = user32.NewProc("TrackMouseEvent")
	pLoadCursorW                   = user32.NewProc("LoadCursorW")
	pSetCursor                     = user32.NewProc("SetCursor")
	pEnumWindows                   = user32.NewProc("EnumWindows")
	pGetWindowThreadProcessId      = user32.NewProc("GetWindowThreadProcessId")
	pGetClassNameW                 = user32.NewProc("GetClassNameW")
	pGetWindowTextW                = user32.NewProc("GetWindowTextW")
	pPostMessageW                  = user32.NewProc("PostMessageW")
	pIsIconic                      = user32.NewProc("IsIconic")
	pIsWindowVisible               = user32.NewProc("IsWindowVisible")
	pFindWindowW                   = user32.NewProc("FindWindowW")
	pSystemParametersInfoW         = user32.NewProc("SystemParametersInfoW")
	pGetDpiForWindow               = user32.NewProc("GetDpiForWindow")
	pSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	pSetProcessDPIAware            = user32.NewProc("SetProcessDPIAware")
	pCreatePopupMenu               = user32.NewProc("CreatePopupMenu")
	pAppendMenuW                   = user32.NewProc("AppendMenuW")
	pTrackPopupMenu                = user32.NewProc("TrackPopupMenu")
	pDestroyMenu                   = user32.NewProc("DestroyMenu")
	pGetCursorPos                  = user32.NewProc("GetCursorPos")
	pCreateIconIndirect            = user32.NewProc("CreateIconIndirect")
	pGetSystemMetrics              = user32.NewProc("GetSystemMetrics")
	pMapVirtualKeyW                = user32.NewProc("MapVirtualKeyW")
	pGetKeyNameTextW               = user32.NewProc("GetKeyNameTextW")
	pRegisterWindowMessageW        = user32.NewProc("RegisterWindowMessageW")
	pSendMessageW                  = user32.NewProc("SendMessageW")

	pCreateCompatibleDC    = gdi32.NewProc("CreateCompatibleDC")
	pCreateDIBSection      = gdi32.NewProc("CreateDIBSection")
	pSelectObject          = gdi32.NewProc("SelectObject")
	pDeleteObject          = gdi32.NewProc("DeleteObject")
	pCreateFontW           = gdi32.NewProc("CreateFontW")
	pSetBkMode             = gdi32.NewProc("SetBkMode")
	pSetTextColor          = gdi32.NewProc("SetTextColor")
	pTextOutW              = gdi32.NewProc("TextOutW")
	pGetTextExtentPoint32W = gdi32.NewProc("GetTextExtentPoint32W")
	pGdiFlush              = gdi32.NewProc("GdiFlush")
	pCreateBitmap          = gdi32.NewProc("CreateBitmap")

	pGetModuleHandleW           = kernel32.NewProc("GetModuleHandleW")
	pCreateMutexW               = kernel32.NewProc("CreateMutexW")
	pOpenProcess                = kernel32.NewProc("OpenProcess")
	pQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
	pCloseHandle                = kernel32.NewProc("CloseHandle")

	pShell_NotifyIconW = shell32.NewProc("Shell_NotifyIconW")
	pShellExecuteW     = shell32.NewProc("ShellExecuteW")

	pRegSetKeyValueW    = advapi32.NewProc("RegSetKeyValueW")
	pRegDeleteKeyValueW = advapi32.NewProc("RegDeleteKeyValueW")
	pRegGetValueW       = advapi32.NewProc("RegGetValueW")
)

const (
	WM_DESTROY      = 0x0002
	WM_ACTIVATE     = 0x0006
	WM_CLOSE        = 0x0010
	WM_NULL         = 0x0000
	WM_SETCURSOR    = 0x0020
	WM_NCHITTEST    = 0x0084
	WM_NCMOUSEMOVE  = 0x00A0
	WM_KEYDOWN      = 0x0100
	WM_SYSKEYDOWN   = 0x0104
	WM_SYSCHAR      = 0x0106
	WM_TIMER        = 0x0113
	WM_MOUSEMOVE    = 0x0200
	WM_LBUTTONDOWN  = 0x0201
	WM_LBUTTONUP    = 0x0202
	WM_RBUTTONUP    = 0x0205
	WM_CONTEXTMENU  = 0x007B
	WM_EXITSIZEMOVE = 0x0232
	WM_DPICHANGED   = 0x02E0
	WM_MOUSELEAVE   = 0x02A3
	WM_HOTKEY       = 0x0312
	WM_APPCOMMAND   = 0x0319
	WM_SETICON      = 0x0080
	WM_APP          = 0x8000
	WM_TRAY         = WM_APP + 1
	WM_SHOWME       = WM_APP + 2

	HTNOWHERE = 0
	HTCLIENT  = 1
	HTCAPTION = 2

	WS_POPUP         = 0x80000000
	WS_EX_LAYERED    = 0x00080000
	WS_EX_TOOLWINDOW = 0x00000080
	WS_EX_TOPMOST    = 0x00000008

	SW_HIDE    = 0
	SW_SHOW    = 5
	SW_RESTORE = 9

	SWP_NOSIZE     = 0x0001
	SWP_NOMOVE     = 0x0002
	SWP_NOACTIVATE = 0x0010

	MOD_ALT      = 0x0001
	MOD_CONTROL  = 0x0002
	MOD_SHIFT    = 0x0004
	MOD_WIN      = 0x0008
	MOD_NOREPEAT = 0x4000

	APPCOMMAND_MEDIA_NEXTTRACK     = 11
	APPCOMMAND_MEDIA_PREVIOUSTRACK = 12
	APPCOMMAND_MEDIA_PLAY_PAUSE    = 14

	NIM_ADD     = 0
	NIM_MODIFY  = 1
	NIM_DELETE  = 2
	NIF_MESSAGE = 1
	NIF_ICON    = 2
	NIF_TIP     = 4

	MF_STRING    = 0x0
	MF_CHECKED   = 0x8
	MF_SEPARATOR = 0x800

	TPM_RIGHTBUTTON = 0x0002
	TPM_RETURNCMD   = 0x0100

	HKEY_CURRENT_USER = 0x80000001
	REG_SZ            = 1
	RRF_RT_REG_SZ     = 0x2

	VK_BACK    = 0x08
	VK_SHIFT   = 0x10
	VK_CONTROL = 0x11
	VK_MENU    = 0x12
	VK_ESCAPE  = 0x1B
	VK_SPACE   = 0x20
	VK_LEFT    = 0x25
	VK_UP      = 0x26
	VK_DOWN    = 0x28
	VK_RIGHT   = 0x27
	VK_LWIN    = 0x5B
	VK_RWIN    = 0x5C
)

type POINT struct{ X, Y int32 }
type SIZE struct{ CX, CY int32 }
type RECT struct{ L, T, R, B int32 }

type WNDCLASSEX struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type MSG struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
	Private uint32
}

type BITMAPINFOHEADER struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type BITMAPINFO struct {
	H      BITMAPINFOHEADER
	Colors [1]uint32
}

type BLENDFUNCTION struct{ Op, Flags, Alpha, Format byte }

type TRACKMOUSEEVENT struct {
	CbSize    uint32
	DwFlags   uint32
	Hwnd      uintptr
	HoverTime uint32
}

type ICONINFO struct {
	FIcon    int32
	XHotspot uint32
	YHotspot uint32
	HbmMask  uintptr
	HbmColor uintptr
}

type NOTIFYICONDATA struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     uintptr
}

func u16(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

// 32bpp top-down DIB section; returns bitmap handle and pixel pointer.
func newDIB(w, h int) (uintptr, unsafe.Pointer) {
	bi := BITMAPINFO{H: BITMAPINFOHEADER{BiSize: 40, BiWidth: int32(w), BiHeight: -int32(h), BiPlanes: 1, BiBitCount: 32}}
	var bits unsafe.Pointer
	hbm, _, _ := pCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	return hbm, bits
}

var (
	comdlg32 = syscall.NewLazyDLL("comdlg32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")

	pGetForegroundWindow           = user32.NewProc("GetForegroundWindow")
	pSetWindowRgn                  = user32.NewProc("SetWindowRgn")
	pSetWindowCompositionAttribute = user32.NewProc("SetWindowCompositionAttribute")
	pCreateRoundRectRgn            = gdi32.NewProc("CreateRoundRectRgn")
	pAddFontMemResourceEx          = gdi32.NewProc("AddFontMemResourceEx")
	pGetGlyphIndicesW              = gdi32.NewProc("GetGlyphIndicesW")
	pChooseColorW                  = comdlg32.NewProc("ChooseColorW")
	pCoInitializeEx                = ole32.NewProc("CoInitializeEx")
	pCoCreateInstance              = ole32.NewProc("CoCreateInstance")
)

type ACCENT_POLICY struct {
	AccentState   uint32
	AccentFlags   uint32
	GradientColor uint32
	AnimationId   uint32
}

type WINCOMPATTRDATA struct {
	Attrib uint32
	Data   uintptr
	Size   uintptr
}

type CHOOSECOLOR struct {
	LStructSize    uint32
	HwndOwner      uintptr
	HInstance      uintptr
	RgbResult      uint32
	LpCustColors   *[16]uint32
	Flags          uint32
	LCustData      uintptr
	LpfnHook       uintptr
	LpTemplateName *uint16
}
