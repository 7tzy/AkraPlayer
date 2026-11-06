package main

import (
	"math"
)

// ---------- layout ----------

const (
	W = 420 // normal panel size (logical); the window IS the glass panel
	H = 300
	R = 30 // corner radius

	ExpW = 600 // panel size while the Keybinds page is open
	ExpH = 480

	MiniW = 400 // mini mode pill
	MiniH = 76
)

// WinSize is the window size in pixels for the current (animated) panel size.
func (u *UI) WinSize(s float32) (int, int) {
	return int(math.Ceil(float64(u.PW * s))), int(math.Ceil(float64(u.PH * s)))
}

func WinSize(s float32) (int, int) {
	return int(math.Ceil(float64(W * s))), int(math.Ceil(float64(H * s)))
}

// Radius of the glass right now (eases to a pill in mini mode and to square
// corners in fullscreen).
func (u *UI) Radius() float32 {
	r := float32(R)
	r = lerp(r, min32(u.PH/2, 38), clamp01(u.miniA.V))
	r = lerp(r, 0, clamp01(u.fullA.V))
	return float32(math.Round(float64(r)*2) / 2)
}

type Status int

const (
	StNotRunning Status = iota
	StPaused
	StPlaying
	StAd
)

// ---------- actions ----------

// Fixed actions. Volume presets get ids PresetAct(id) so users can add more.
const (
	ActPlay      = iota // play/pause Spotify
	ActShow             // open Akra Player
	ActMute             // mute the window you're in
	ActAppPlay          // play/pause the chosen window
	ActVolCur           // current window volume (toggle)
	ActDuck             // hold to lower music (push-to-talk)
	ActLike             // like the current Spotify song
	ActYouTube          // play/pause YouTube (browser extension)
	ActDeclutter        // declutter / bring everything back
	NumFixed
)

const presetBase = 1000

func PresetAct(id int) int     { return presetBase + id }
func IsPresetAct(act int) bool { return act >= presetBase }
func PresetIDOf(act int) int   { return act - presetBase }
func IsValidAct(act int, ps []PresetUI) bool {
	if act >= 0 && act < NumFixed {
		return true
	}
	for _, p := range ps {
		if PresetAct(p.ID) == act {
			return true
		}
	}
	return false
}

var fixedLabels = [NumFixed]string{
	"Play / pause Spotify", "Open Akra Player", "Mute current window",
	"Play / pause window", "Current window volume", "Hold to lower music",
	"Like song", "Play / pause YouTube", "Declutter / show all",
}

// CanSwitchOff: keybinds with an on/off switch on their card.
func CanSwitchOff(act int) bool { return act == ActAppPlay || act == ActYouTube }

// ActLabel is the display name of an action.
func (u *UI) ActLabel(act int) string {
	if act >= 0 && act < NumFixed {
		return fixedLabels[act]
	}
	if i := u.presetIndex(act); i >= 0 {
		if n := u.Presets[i].App.Name; n != "" {
			return "Volume Toggle · " + n
		}
		return "Volume Toggle"
	}
	return "Unknown"
}

func (u *UI) presetIndex(act int) int {
	for i, p := range u.Presets {
		if PresetAct(p.ID) == act {
			return i
		}
	}
	return -1
}

// AllActs lists every action (fixed + presets).
func (u *UI) AllActs() []int {
	out := make([]int, 0, NumFixed+len(u.Presets))
	for a := 0; a < NumFixed; a++ {
		out = append(out, a)
	}
	for _, p := range u.Presets {
		out = append(out, PresetAct(p.ID))
	}
	return out
}

// Toggle looks (Appearance > "Switched-on look").
const (
	TogEdge    = iota // a green glow along the inside edges
	TogFrosted        // the whole box turns frosted green
	TogClassic        // a switch
)

// Where key-press popups show.
const (
	ToastTopLeft = iota
	ToastTopCenter
	ToastTopRight
	ToastBottomLeft
	ToastBottomCenter
	ToastBottomRight
)

var toastPosNames = [...]string{"Top left", "Top center", "Top right", "Bottom left", "Bottom center", "Bottom right"}

// Pop-out windows.
const (
	PopNone = iota
	PopMixer
	PopKeys
	PopExport // the Export settings window
	PopImport // the Import settings window
)

// MixerApp: one app in the volume mixer.
type MixerApp struct {
	Name, IconKey string
	Vol           float32
	Muted         bool
}

// PresetUI is a "set this app to X%" key.
type PresetUI struct {
	ID  int
	App Target
	Pct int
}

func DefaultFolders() []Folder {
	return []Folder{
		{Name: "Audio", Acts: []int{ActPlay, ActAppPlay, ActLike, ActYouTube}},
		{Name: "Volume", Acts: []int{ActMute, ActVolCur, ActDuck, PresetAct(1), PresetAct(2)}},
		{Name: "Player", Acts: []int{ActShow, ActDeclutter}},
	}
}

// ---------- colors, fonts ----------

// Accent presets: blue, red, green, yellow, orange, black, white. Black and
// white share one swatch (click it again to switch).
var Accents = []Col{
	Hex(0x0a84ff, 1),
	Hex(0xff453a, 1),
	Hex(0x30d158, 1),
	Hex(0xffd60a, 1),
	Hex(0xff9f0a, 1),
	Hex(0x2c2c30, 1),
	Hex(0xf2f2f7, 1),
}

var AccentNames = []string{"Blue", "Red", "Green", "Yellow", "Orange", "Black", "White"}

const (
	AccentBlack = 5
	AccentWhite = 6
)

const (
	AccentCustom  = -1
	AccentRainbow = -2
	AccentCover   = -3 // follows the album cover
)

// BrandRed: Akra Player's own red (the app icon).
var BrandRed = Hex(0x9b1b30, 1)

const (
	FontInter = iota
	FontPoppins
	FontSegoe
	NumFonts
)

var FontNames = [NumFonts]string{"Inter", "Poppins", "Segoe UI"}

// Fluid background color source
const (
	FluidTheme = iota
	FluidCustom
	FluidArt
)

// ---------- hit target ids ----------

const (
	hNone = iota
	hClose
	hPin
	hPrev
	hPlay
	hNext
	hMute
	hColor
	hSettings
	hControls
	hTrack
	hBack
	hKeysSheet
	hSheetClose
	hNewFolder
	hEditField
	hSheetBg
	hPickBg
	hPickBox
	hSwatchCustom
	hSwatchRainbow
	hChipPlay
	hEditKeys
	hProgress
	hHueSV
	hHueBar
	hHueDone
	hHueCancel
	hHueBg
	hHueBox
	hHueTheme
	hHueArt
	hAddPreset
	hProfileAdd
	hProfileRen
	hProfileDel
	hDuckChip
	hHeart
	hArtToggle
	hMini
	hEnlarge
	hShrink
	hSpotVol
	hFluidPick
	hMiniBg
	hMiniRestore
	hPillPrev
	hPillPlay
	hPillNext
	hPillTrack
	hVisShow
	hVisHide
	hResizeNW // corners (resizing)
	hResizeNE
	hResizeSW
	hResizeSE
	hDeclutter
	hShowAll
	hAddToggle
	hMiniVol
	hPillMute
	hPillVol
	hResetDefaults
	hBgDrop
	hPopPin
	hPopClose
	hSwatchCover
	hSetPop // Settings: pop out into its own window (or dock back)
	hExpChange
	hExpOpen
	hImpBrowse
	hImpGo
	hWavePick
	hSleepChip
	hTimeRight
	hLookAdd
	hLookRen
	hLookDel
	hLookRestore
	hExport
	hImport
	hExpGo
	hExpCancel

	hNav0        = 1000
	hBind0       = 10000 // + action
	hMove0       = 20000 // + action
	hFold0       = 30000 // + folder
	hFoldRen0    = 31000
	hFoldDel0    = 32000
	hTog0        = 33000 // + toggle
	hSwatch0     = 34000 // + accent
	hFont0       = 35000 // + font
	hSlider0     = 36000 // + slider
	hPresetChip0 = 37000 // + preset index
	hPresetDel0  = 38000
	hPick0       = 39000 // + picker item
	hProfile0    = 40000 // + profile
	hVisStyle0   = 41000 // + visualizer style
	hVisMon0     = 42000 // + monitor
	hMixVol0     = 43000 // + mixer app (vertical slider)
	hMixIcon0    = 44000 // + mixer app (mute)
	hTogStyle0   = 45000 // + toggle look
	hToastPos0   = 46000 // + key-press popup position
	hBindOn0     = 47000 // + action: its on/off switch
	hLook0       = 48000 // + appearance profile
	hExpInc0     = 49000 // + export part (general, appearance, keybinds)
	hDeclScope0  = 50000 // + declutter key scope (everywhere, in Akra Player)
	hSleep0      = 51000 // + sleep timer choice
	hTogEdit0    = 52000 // + toggle: its pencil (settings for that feature)
	hWaveMode0   = 53000 // + waveform color mode
	hEdgeStyle0  = 54000 // + Screen Edge look (bars, waveform)
	hSongMon0    = 55000 // + 0 = the screen you're working on, 1.. = a screen
	hSongPos0    = 56000 // + now playing popup position
	hImp0        = 57000 // + saved export in the Import window
	hImpInc0     = 58000 // + part to import
	hIDMax       = 59000
)

// appVersion is shown in Settings and written into exports.
const appVersion = "2.9.7"

func inRange(h, base int) bool { return h >= base && h < base+1000 }

// Sliders
const (
	SlClarity = iota
	SlScrollRes
	SlCurVol
	SlDuck
	SlSpotVol
	SlOpenRes
	SlVisSens
	SlVisSmooth
	SlVisP0 // the chosen style's own sliders
	SlVisP1
	SlIdleDelay
	SlIdleOpacity
	SlPreset0 = 100 // + preset index
)

// Settings toggles
const (
	TogStartup = iota
	TogOnTop
	TogArt
	TogToasts
	TogBlur
	TogVis
	TogOutline
	TogFluid
	TogAnchor
	TogGlow
	TogTips
	TogFullscreen
	TogSongPopup  // a popup when the song changes while the player is hidden
	TogIdleFade   // the player fades out while you're not using it
	TogTrayAccent // tray icon in your color (off: the red Akra icon)
	NumToggles
)

// Sleep timer choices.
const (
	SleepOff = iota
	Sleep15
	Sleep30
	Sleep60
	SleepSong // at the end of this song
	NumSleep
)

var sleepNames = [NumSleep]string{"Off", "15 min", "30 min", "1 hour", "End of song"}
var sleepMinutes = [NumSleep]float32{0, 15, 30, 60, 0}

var togLabels = [NumToggles][2]string{
	{"Open on startup", "Start when you sign in"},
	{"Always on top", "Stay above other windows"},
	{"Album art", "Show the cover next to the song"},
	{"Key-press popups", "A note in the corner when a hotkey does something"},
	{"Background blur", "Frost what's behind the glass"},
	{"Visualizer", "Moves with the music"},
	{"Window outline", "Bright rim around the glass"},
	{"Fluid background", "Slow color flow inside the glass"},
	{"Resize around the button", "Enlarge/shrink keep the button under your cursor"},
	{"Cover glow", "Soft light around the album cover in its colors"},
	{"Tooltips", "Explanations when you hover a setting"},
	{"Allow fullscreen", "Enlarge can take over the whole screen"},
	{"Now playing popups", "A note when the song changes while hidden"},
	{"Fade when idle", "Fades out until your mouse comes back"},
	{"Colored tray icon", "Tray icon in your color, not Akra red"},
}

const (
	SecGeneral = iota
	SecLook
	SecKeys
	NumSections
)

var secLabels = [NumSections]string{"General", "Appearance", "Keybinds"}

const (
	SheetNone = iota
	SheetControls
	SheetKeys
)

const (
	PickWindow = iota // windows for play/pause
	PickApp           // apps for a volume preset (PickSlot = preset index, -1 = ducking)
	PickFolder        // folder to move a keybind into (PickSlot = action)
)

const (
	HueAccent = iota
	HueFluid
	HueWave // the waveform visualizer's own color
)

const (
	EditNone = iota
	EditFolder
	EditProfile
	EditLook
)

// What an export can include.
const (
	ExpGeneral = iota
	ExpLook
	ExpKeys
	NumExp
)

var expNames = [NumExp]string{"General", "Appearance", "Keybinds"}

// Target is something chosen in the controls sheet.
type Target struct {
	Name    string
	Sub     string
	IconKey string
}

type PickItem struct {
	Name, Sub, IconKey string
	Active             bool
	Selected           bool
}

type Folder struct {
	Name      string
	Acts      []int
	Collapsed bool
}

// ---------- animation primitives ----------

type Spring struct{ X, V, Target float32 }

func (s *Spring) Step(dt, k, d float32) bool {
	for dt > 0 {
		h := min32(dt, 1.0/240)
		a := k*(s.Target-s.X) - d*s.V
		s.V += a * h
		s.X += s.V * h
		dt -= h
	}
	if abs32(s.Target-s.X) < 0.0005 && abs32(s.V) < 0.0005 {
		s.X, s.V = s.Target, 0
		return false
	}
	return true
}

func (s *Spring) Snap(v float32) { s.X, s.V, s.Target = v, 0, v }

type Tween struct{ V float32 }

func (t *Tween) Step(target, rate, dt float32) bool {
	t.V += (target - t.V) * (1 - float32(math.Exp(float64(-rate*dt))))
	if abs32(target-t.V) < 0.002 {
		t.V = target
		return false
	}
	return true
}

// Scroller: smooth scrolling with a rubber band at the ends.
type Scroller struct {
	Pos  Spring
	Max  float32
	Over float32
	idle float32
}

func (s *Scroller) Step(dt float32) bool {
	busy := s.Pos.Step(dt, 260, 32)
	s.idle += dt
	if s.Over != 0 && s.idle > 0.35 {
		step := 520 * dt
		if abs32(s.Over) <= step {
			s.Over = 0
		} else if s.Over > 0 {
			s.Over -= step
		} else {
			s.Over += step
		}
		busy = true
	}
	if s.Pos.Target > s.Max {
		s.Pos.Target = max32(0, s.Max)
		busy = true
	}
	return busy
}

func (s *Scroller) Rubber(limit float32) float32 {
	if s.Over == 0 {
		return 0
	}
	t := float32(1 - math.Exp(float64(-abs32(s.Over)/max32(limit, 1))))
	d := t * t * 12
	if s.Over > 0 {
		return -d
	}
	return d
}

func (s *Scroller) Scroll(delta, limit float32) int {
	s.idle = 0
	t := s.Pos.Target + delta
	switch {
	case t < 0 && delta < 0 && s.Pos.Target <= 0.5:
		s.Pos.Target = 0
		if s.Over > 0 {
			s.Over = 0
		}
		s.Over += delta
	case t > s.Max && delta > 0 && s.Pos.Target >= s.Max-0.5:
		s.Pos.Target = max32(0, s.Max)
		if s.Over < 0 {
			s.Over = 0
		}
		s.Over += delta
	default:
		s.Over = 0
		s.Pos.Target = max32(0, min32(t, s.Max))
	}
	if limit > 0 && abs32(s.Over) >= limit {
		dir := 1
		if s.Over < 0 {
			dir = -1
		}
		s.Over = 0
		return dir
	}
	return 0
}

type hitArea struct {
	id             int
	x0, y0, x1, y1 float32
	circle         bool
}

type tipZone struct {
	text           string
	x0, y0, x1, y1 float32
}

type TextRenderer interface {
	Render(s string, px float32, weight, font int) *Mask
}

type IconSource func(key string, px int) *Canvas

// ---------- UI state ----------

type UI struct {
	Text  TextRenderer
	Icons IconSource

	Status        Status
	Title, Artist string
	Binds         map[int][]string
	BindNote      map[int]string
	BindWarn      map[int]string // "Also Select All" etc. (shown, never blocks)
	RecIdx        int            // action being recorded, -1 none
	RecKeys       []string
	Toggles       [NumToggles]bool
	Clarity       float32
	ScrollRes     float32
	OpenRes       float32 // how far you scroll on the player to open Settings
	VisStyle      int
	VisMonitor    int
	Monitors      []string
	BaseW, BaseH  float32 // the player's size (logical), set by the app from the window size
	Sizing        bool    // the app is resizing the window (corner drag, enlarge/shrink)
	MaxW, MaxH    float32 // the most the window may grow to (logical; the screen's usable area)
	Declutter     bool    // only the essentials on the player
	MixerApps     []MixerApp
	WinX, WinY    float32 // the window's screen position (pixels)
	TogStyle      int     // how switched-on features look (TogEdge, TogFrosted, TogClassic)
	ToastPos      int     // where key-press popups appear
	BgOpen        bool    // the Background group in Appearance is open
	Pop           int     // pop-out window: PopNone, PopMixer, PopKeys
	PopPinned     bool
	BdX, BdY      float32 // the blur backdrop's screen position (pixels)
	SpotMuted     bool
	SpotVol       float32 // Spotify's own volume 0..1
	Liked         bool
	Page          int
	Section       int
	Sheet         int
	AccentIdx     int
	Custom        Col
	Font          int
	Hover, Press  int
	T             float32
	Toast         string

	PlayTarget Target
	PlayGone   bool    // the chosen window was closed
	Sleep      int     // sleep timer choice
	SleepLeft  float32 // seconds left (minute timers)
	Remaining  bool    // show the time left instead of the length
	// 2.9.5
	VisSens, VisSmooth float32                  // visualizer sensitivity and smoothness (0..1)
	VisP               [NumVisStyles][2]float32 // each style's own sliders (0..1)
	WaveMode           int                      // waveform colors
	WaveCol            Col
	EdgeStyle          int     // Screen Edge: bars or waveform
	IdleDelay          float32 // Fade when idle: seconds before it fades
	IdleOpacity        float32 // and how see-through it gets (0..1)
	SongPos, SongMon   int     // now playing popups: where, and on which screen (-1 = where you're working)
	TogEdit            int     // the feature whose settings are open (-1 none)
	PoppedOut          bool    // Settings is in its own window (the main one shows the player)
	// Export / Import windows
	ExpName, ExpDir string       // what the export will be called, and where it goes
	ExpDone         string       // the folder just exported to ("" = not yet)
	ImpList         []ImportItem // exports found
	ImpSel          int
	ImpInc          [NumExp]bool
	ImpDone         string // what was just imported
	TransferMsg     string // a problem to show in the window
	SetPopView      bool   // drawing / handling that Settings window right now
	toastSet        bool   // the in-window toast belongs to the Settings window
	togEditShown    int
	editA           Tween
	BindOff         map[int]bool
	DeclLocal       bool // the declutter key only works while Akra Player is in front
	Looks           []string
	LookBuiltin     []bool
	LookChanged     bool // the selected built-in look was changed from its preset
	Look            int
	ExpOpen         bool
	ExpInc          [NumExp]bool
	expA            Tween
	Presets         []PresetUI
	CurPct          int
	DuckPct         int
	DuckTarget      Target
	Folders         []Folder
	Profiles        []string
	Profile         int
	EditKind        int
	EditIdx         int
	EditText        string

	PickOpen  bool
	PickMode  int
	PickSlot  int
	PickItems []PickItem

	Art       *Canvas
	ArtVer    int
	Pos, Dur  float32
	PosT      float32
	ProgKnown bool

	Backdrop      *Canvas
	BackdropScale float32
	BackdropVer   int

	PW, PH       float32
	Mini, Full   bool // mini pill / fullscreen
	Base         *BaseLayer
	BaseAsync    func(p BaseParams)
	FocusSlider  int
	Seekable     bool
	Seeking      bool
	SeekFrac     float32
	Levels       []float32
	HueOpen      bool
	HueFor       int
	HueH, HueS   float32
	HueV         float32
	hueBefore    Col
	hueMode      int
	FluidMode    int
	FluidCol     Col
	HoldID       int     // enlarge/shrink being held
	HoldT        float32 // 0..1 progress toward the 3 s hold
	TipID        int     // button showing its tooltip
	ExtConnected bool    // browser extension online

	page, sec, nav, sheetA, pick, hue Spring
	pw, ph                            Spring
	pushA                             Tween
	pushF                             Spring
	pushDir                           int
	flipHold                          float32
	baseWant                          string
	accent                            Col
	morph                             Tween
	tog                               [NumToggles]Tween
	toastA                            Tween
	toastTTL                          float32
	spotVolA, tipA, likeA             Tween
	hov, prs, sel                     map[int]*Tween
	fold                              map[int]*Tween
	secScroll                         [NumSections]Scroller
	sheetScroll, pickScroll           Scroller
	lastSheet                         int
	artCols                           [4]Col // fluid palette pulled from the cover
	artColVer                         int
	fullA, miniA                      Tween
	hoverT                            float32
	lastTip                           int
	playScroll                        Scroller
	artIn                             float32 // 0..1: the cover fizzles in, then its glow blooms
	artInVer                          int
	light                             LightBuf
	waveCache, glowLUT                []float32
	glowS, glowK                      float32
	spriteBytes, maskBytes            int
	widths                            map[string]float32
	tips                              []tipZone
	marqueeOn                         bool
	declA                             Tween
	bgA, popA                         Tween
	popScroll                         Scroller
	popHits                           []hitArea
	smallBases                        map[string]*BaseLayer
	smallFluids                       map[string]*Fluid
	pvol                              [2]Tween // volume bars in the mini player / settings pill
	ResetArmed                        float32  // "click again to reset" countdown
	peakRef, visPhase                 float32
	mixScroll                         Scroller
	pillBase                          *BaseLayer
	pillBaseKey                       string
	pillFluid                         Fluid
	HoldFired                         int // set when a 3 s hold completes; the app clears it
	tipX, tipY                        float32
	spkX, spkY                        float32
	fluid                             Fluid

	hits     []hitArea
	pillHits []hitArea
	sliders  map[int][2]float32
	vsliders map[int][2]float32 // vertical sliders: y0 (top), y1 (bottom)
	cache    map[string]*Mask
	sprites  map[string]*Canvas
	scale    float32
	shownTst string
}

func NewUI(t TextRenderer) *UI {
	u := &UI{Text: t, RecIdx: -1, Clarity: 0.6, ScrollRes: 0.5, AccentIdx: 0, ToastPos: ToastTopRight,
		Custom: Hex(0xbf5af2, 1), CurPct: 40, DuckPct: 20, BackdropScale: 4, SpotVol: 1,
		Binds: map[int][]string{}, BindNote: map[int]string{}, BindWarn: map[int]string{}, BindOff: map[int]bool{}, ExpInc: [NumExp]bool{true, true, true},
		hov: map[int]*Tween{}, prs: map[int]*Tween{}, sel: map[int]*Tween{}, fold: map[int]*Tween{},
		sliders: map[int][2]float32{}, vsliders: map[int][2]float32{}, cache: map[string]*Mask{}, sprites: map[string]*Canvas{}, scale: 1,
		PW: W, PH: H, FluidCol: Hex(0x7b5cff, 1), Profiles: []string{"Default"}}
	u.Toggles[TogOutline] = true
	u.Toggles[TogAnchor] = true
	u.OpenRes = 0.8
	u.Toggles[TogFluid] = true
	u.Toggles[TogGlow] = true
	u.Toggles[TogTips] = true
	u.widths = map[string]float32{}
	u.BaseW, u.BaseH = W, H
	u.artIn, u.artInVer = 1, 0
	u.Toggles[TogToasts] = true
	u.Presets = []PresetUI{{ID: 1, Pct: 30}, {ID: 2, Pct: 50}}
	u.pw.Snap(W)
	u.ph.Snap(H)
	u.Folders = DefaultFolders()
	u.VisSens, u.VisSmooth = 0.5, 0.3
	for i := range u.VisP {
		u.VisP[i] = [2]float32{0.5, 0.5}
	}
	u.WaveCol = Hex(0xff6b8b, 1)
	u.IdleDelay, u.IdleOpacity = 3, 0.28
	u.SongPos, u.SongMon = ToastTopRight, -1
	u.TogEdit, u.togEditShown = -1, -1
	u.accent = u.AccentTarget()
	return u
}

// Waveform colors.
const (
	WaveTheme    = iota // your accent color
	WaveCustom          // a color you pick
	WaveRGB             // every color, shifting
	WaveGradient        // your color in the middle fading into the accent
	WaveLiquid          // its own flowing fluid colors
	NumWaveModes
)

var waveModeNames = [NumWaveModes]string{"Theme", "Custom", "RGB", "Gradient", "Liquid"}

// Screen Edge looks.
const (
	EdgeBars = iota
	EdgeWave
)

// visParamNames: each style's own sliders ("" = none).
var visParamNames = [NumVisStyles][2]string{
	VisBars:    {"Height", "Width"},
	VisHalo:    {"Size", "Ray length"},
	VisEdge:    {"Height", ""},
	VisThreads: {"Amount", "Spread"},
	VisGlow:    {"Glow", "Height"},
	VisMesh:    {"Size", "Wobble"},
	VisRipples: {"Speed", "Rings"},
	VisWave:    {"Height", "Fill"},
	VisSparks:  {"Amount", "Glow"},
}

// visP: a style's own slider mapped onto lo..hi (0.5 = the old default).
func (u *UI) visP(style, k int, lo, mid, hi float32) float32 {
	v := u.VisP[style][k]
	if v < 0.5 {
		return lo + (mid-lo)*v*2
	}
	return mid + (hi-mid)*(v-0.5)*2
}

// HasTogEdit: features with their own settings (a pencil on their box).
func HasTogEdit(i int) bool { return i == TogIdleFade || i == TogSongPopup }

func (u *UI) AccentTarget() Col {
	switch {
	case u.HueOpen && u.HueFor == HueAccent:
		return u.Custom
	case u.AccentIdx == AccentRainbow:
		return HSV(u.T*0.055, 0.62, 1)
	case u.AccentIdx == AccentCover:
		return u.CoverAccent()
	case u.AccentIdx >= 0 && u.AccentIdx < len(Accents):
		return Accents[u.AccentIdx]
	}
	return u.Custom
}

func (u *UI) SnapAnimations() {
	u.page.Snap(float32(u.Page))
	u.sec.Snap(float32(u.Section))
	u.nav.Snap(float32(u.Section))
	u.sheetA.Snap(b2f(u.Sheet != SheetNone))
	u.pick.Snap(b2f(u.PickOpen))
	u.hue.Snap(b2f(u.HueOpen))
	tw, th := u.panelTarget()
	u.pw.Snap(tw)
	u.ph.Snap(th)
	u.PW, u.PH = tw, th
	u.accent = u.AccentTarget()
	for i := range u.tog {
		u.tog[i].V = b2f(u.Toggles[i])
	}
	u.morph.V = b2f(u.playing())
	u.fold = map[int]*Tween{}
	for i := range u.Folders {
		u.fold[i] = &Tween{V: b2f(!u.Folders[i].Collapsed)}
	}
	u.sel = map[int]*Tween{}
}

func b2f(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

func (u *UI) playing() bool { return u.Status == StPlaying || u.Status == StAd }

func (u *UI) ShowToast(s string) {
	u.Toast = s
	u.shownTst = s
	u.toastTTL = 1.9
	u.toastSet = u.SetPopView // shows in the window it came from
}

func (u *UI) scrollLimit() float32 { return 60 + 380*u.ScrollRes }

// panelTarget: the size the glass is heading to.
// panelTarget: the size the glass is heading to. Every page uses the size
// you set (no page grows the window by itself any more); each page lays
// itself out to fit.
func (u *UI) panelTarget() (float32, float32) {
	if u.Mini {
		return MiniW, MiniH
	}
	return u.BaseW, u.BaseH
}

// SnapPanel jumps the glass straight to its target size (while you drag a
// corner the window must follow the mouse exactly).
func (u *UI) SnapPanel() {
	w, h := u.panelTarget()
	u.pw.Snap(w)
	u.ph.Snap(h)
	u.PW, u.PH = float32(math.Round(float64(w))), float32(math.Round(float64(h)))
}

func (u *UI) openLimit() float32 { return 60 + 380*u.OpenRes }

// Resizing reports whether the glass is mid-resize.
func (u *UI) Resizing() bool { return u.pw.V != 0 || u.ph.V != 0 || u.Sizing }

func (u *UI) Step(dt float32) bool {
	if dt > 0.05 {
		dt = 0.05
	}
	busy := false
	u.page.Target = float32(u.Page)
	u.sec.Target = float32(u.Section)
	u.nav.Target = float32(u.Section)
	u.sheetA.Target = b2f(u.Sheet != SheetNone)
	u.pick.Target = b2f(u.PickOpen)
	if u.Sheet != SheetNone {
		u.lastSheet = u.Sheet
	}
	busy = u.page.Step(dt, 210, 25) || busy
	busy = u.sec.Step(dt, 150, 25) || busy
	busy = u.nav.Step(dt, 300, 28) || busy
	busy = u.sheetA.Step(dt, 230, 27) || busy
	busy = u.pick.Step(dt, 320, 30) || busy
	u.hue.Target = b2f(u.HueOpen)
	busy = u.hue.Step(dt, 320, 30) || busy
	u.pw.Target, u.ph.Target = u.panelTarget()
	busy = u.pw.Step(dt, 190, 26) || busy
	busy = u.ph.Step(dt, 190, 26) || busy
	u.PW, u.PH = float32(math.Round(float64(u.pw.X))), float32(math.Round(float64(u.ph.X)))

	sc := &u.secScroll[u.Section]
	lim := u.scrollLimit()
	if u.Page == 0 {
		sc, lim = &u.playScroll, u.openLimit()
	}
	if sc.Over != 0 {
		u.pushDir = 1
		if sc.Over < 0 {
			u.pushDir = -1
		}
		u.pushF.Target = clamp01(abs32(sc.Over) / lim)
	} else if u.flipHold <= 0 {
		u.pushF.Target = 0
	}
	if u.flipHold > 0 {
		u.flipHold -= dt
		busy = true
	}
	busy = u.pushF.Step(dt, 260, 30) || busy
	busy = u.pushA.Step(b2f((sc.Over != 0 || u.flipHold > 0) && u.Sheet == SheetNone && !u.Mini), 10, dt) || busy
	busy = u.playScroll.Step(dt) || busy
	if u.ArtVer != u.artInVer { // a new cover: play its entrance
		u.artInVer = u.ArtVer
		if u.Art != nil {
			u.artIn = 0
		}
	}
	if u.artIn < 1 {
		u.artIn = min32(1, u.artIn+dt/1.25)
		busy = true
	}
	for i := range u.secScroll {
		busy = u.secScroll[i].Step(dt) || busy
	}
	busy = u.sheetScroll.Step(dt) || busy
	busy = u.pickScroll.Step(dt) || busy
	busy = u.mixScroll.Step(dt) || busy

	at := u.AccentTarget()
	if u.AccentIdx == AccentRainbow && !u.HueOpen {
		u.accent = at
	} else if d := abs32(at.R-u.accent.R) + abs32(at.G-u.accent.G) + abs32(at.B-u.accent.B); d > 0.004 {
		u.accent = lerpCol(u.accent, at, 1-float32(math.Exp(float64(-9*dt))))
		busy = true
	} else {
		u.accent = at
	}
	busy = u.morph.Step(b2f(u.playing()), 16, dt) || busy
	for i := range u.tog {
		busy = u.tog[i].Step(b2f(u.Toggles[i]), 14, dt) || busy
	}
	for i := range u.Folders {
		if u.fold[i] == nil {
			u.fold[i] = &Tween{V: b2f(!u.Folders[i].Collapsed)}
		}
		busy = u.fold[i].Step(b2f(!u.Folders[i].Collapsed), 14, dt) || busy
	}
	for _, id := range []int{u.Hover, u.Press} {
		if id != hNone {
			if u.hov[id] == nil {
				u.hov[id] = &Tween{}
			}
			if u.prs[id] == nil {
				u.prs[id] = &Tween{}
			}
		}
	}
	for id, t := range u.hov {
		busy = t.Step(b2f(u.Hover == id), 16, dt) || busy
	}
	for id, t := range u.prs {
		busy = t.Step(b2f(u.Press == id), 26, dt) || busy
	}
	for id, t := range u.sel {
		busy = t.Step(b2f(u.isSelected(id)), 14, dt) || busy
	}
	// Spotify volume bar: opens while the speaker (or the bar) is hovered or dragged
	open := !u.Mini && (u.Hover == hMute || u.Hover == hSpotVol) || u.Press == hSlider0+SlSpotVol || u.Hover == hSlider0+SlSpotVol
	busy = u.spotVolA.Step(b2f(open), 14, dt) || busy
	busy = u.miniA.Step(b2f(u.Mini), 11, dt) || busy
	busy = u.declA.Step(b2f(u.Declutter), 9, dt) || busy
	busy = u.bgA.Step(b2f(u.BgOpen), 12, dt) || busy
	busy = u.popA.Step(b2f(u.Pop != PopNone), 14, dt) || busy
	busy = u.expA.Step(b2f(u.ExpOpen), 12, dt) || busy
	if u.TogEdit >= 0 && !u.Toggles[u.TogEdit] { // the feature was switched off
		u.TogEdit = -1
	}
	if u.TogEdit >= 0 {
		u.togEditShown = u.TogEdit
	}
	busy = u.editA.Step(b2f(u.TogEdit >= 0), 12, dt) || busy
	busy = u.popScroll.Step(dt) || busy
	for i, ids := range [2][2]int{{hMute, hMiniVol}, {hPillMute, hPillVol}} {
		open := u.Hover == ids[0] || u.Hover == ids[1] || u.Press == ids[1]
		if i == 0 && !u.Mini {
			open = false
		}
		busy = u.pvol[i].Step(b2f(open), 12, dt) || busy
	}
	if u.ResetArmed > 0 {
		u.ResetArmed -= dt
		busy = true
	}
	// visualizers: follow the music's own loudness (quiet songs still move)
	var pk float32
	for i := 0; i < 12; i++ {
		pk = max32(pk, u.level(i))
	}
	u.peakRef = max32(max32(pk, u.peakRef*float32(math.Exp(float64(-dt/4)))), 0.04)
	u.visPhase += dt * (0.5 + 2.2*u.levelN(0))
	busy = u.fullA.Step(b2f(u.Full), 9, dt) || busy
	// enlarge / shrink: tooltip after hovering 3 s, action after holding 3 s
	if (u.Hover == hEnlarge || u.Hover == hShrink) && u.Press == hNone {
		u.hoverT += dt
		if u.hoverT >= 3 {
			u.TipID = u.Hover
		}
		busy = busy || u.hoverT < 3.05
	} else {
		u.hoverT = 0
		if u.Press != u.TipID {
			u.TipID = hNone
		}
	}
	if u.HoldID != hNone {
		if u.Press != u.HoldID {
			u.HoldID, u.HoldT = hNone, 0
		} else {
			u.HoldT += dt / 1.5
			if u.HoldT >= 1 {
				u.HoldFired = u.HoldID
				u.HoldID, u.HoldT = hNone, 0
			}
			busy = true
		}
	}
	busy = u.tipA.Step(b2f(u.TipID != hNone), 12, dt) || busy
	busy = u.likeA.Step(b2f(u.Liked), 14, dt) || busy
	if u.toastTTL > 0 {
		u.toastTTL -= dt
		busy = true
	}
	busy = u.toastA.Step(b2f(u.toastTTL > 0), 12, dt) || busy
	return busy
}

func (u *UI) isSelected(id int) bool {
	switch {
	case id == hSwatchCustom:
		return u.AccentIdx == AccentCustom
	case id == hSwatchRainbow:
		return u.AccentIdx == AccentRainbow
	case id == hSwatchCover:
		return u.AccentIdx == AccentCover
	case inRange(id, hSleep0):
		return u.Sleep == id-hSleep0
	case inRange(id, hWaveMode0):
		return u.WaveMode == id-hWaveMode0
	case inRange(id, hEdgeStyle0):
		return u.EdgeStyle == id-hEdgeStyle0
	case inRange(id, hSongMon0):
		return u.SongMon == id-hSongMon0-1
	case inRange(id, hSongPos0):
		return u.SongPos == id-hSongPos0
	case inRange(id, hTogEdit0):
		return u.TogEdit == id-hTogEdit0
	case inRange(id, hImp0):
		return u.ImpSel == id-hImp0
	case inRange(id, hImpInc0):
		return id-hImpInc0 < NumExp && u.ImpInc[id-hImpInc0]
	case id == hSwatch0+AccentBlack: // the black & white swatch
		return (u.AccentIdx == AccentBlack || u.AccentIdx == AccentWhite) && !(u.HueOpen && u.HueFor == HueAccent)
	case id >= hSwatch0 && id < hSwatch0+len(Accents):
		return u.AccentIdx == id-hSwatch0 && !(u.HueOpen && u.HueFor == HueAccent)
	case id >= hFont0 && id < hFont0+NumFonts:
		return u.Font == id-hFont0
	case inRange(id, hProfile0):
		return u.Profile == id-hProfile0
	case id == hVisShow:
		return u.Toggles[TogVis]
	case id == hVisHide:
		return !u.Toggles[TogVis]
	case inRange(id, hVisStyle0):
		return u.VisStyle == id-hVisStyle0
	case inRange(id, hVisMon0):
		return u.VisMonitor == id-hVisMon0
	case inRange(id, hTogStyle0):
		return u.TogStyle == id-hTogStyle0
	case inRange(id, hToastPos0):
		return u.ToastPos == id-hToastPos0
	case id == hPopPin:
		return u.PopPinned
	case inRange(id, hBindOn0):
		return !u.BindOff[id-hBindOn0]
	case inRange(id, hLook0):
		return u.Look == id-hLook0
	case inRange(id, hExpInc0):
		return id-hExpInc0 < NumExp && u.ExpInc[id-hExpInc0]
	case inRange(id, hDeclScope0):
		return u.DeclLocal == (id-hDeclScope0 == 1)
	}
	return false
}

func (u *UI) selV(id int) float32 {
	t := u.sel[id]
	if t == nil {
		t = &Tween{V: b2f(u.isSelected(id))}
		u.sel[id] = t
	}
	return t.V
}

// NeedsIdleFrames: looping animations (waveform, rainbow, caret, fluid, hold fill).
func (u *UI) NeedsIdleFrames() bool {
	return (u.playing() && u.page.X < 0.99) || u.RecIdx >= 0 || u.AccentIdx == AccentRainbow ||
		u.EditKind != EditNone || u.Seeking || u.Toggles[TogFluid] || u.HoldID != hNone || u.marqueeOn
}

// OnlyFluidAnimating: nothing but the slow fluid needs frames.
func (u *UI) OnlyFluidAnimating() bool {
	return !(u.playing() && u.page.X < 0.99) && u.RecIdx < 0 && u.AccentIdx != AccentRainbow &&
		u.EditKind == EditNone && !u.Seeking && u.HoldID == hNone
}

func (u *UI) h(id int) float32 {
	if t := u.hov[id]; t != nil {
		return t.V
	}
	return 0
}

func (u *UI) p(id int) float32 {
	if t := u.prs[id]; t != nil {
		return t.V
	}
	return 0
}

// CoverAccent: a color for buttons taken from the album cover (the most
// colorful part of it, made bright enough to read). Blue without a cover.
func (u *UI) CoverAccent() Col {
	u.ensureArtColors()
	if u.Art == nil || u.artColVer != u.ArtVer {
		return Accents[0]
	}
	best, bestS := u.artCols[0], float32(-1)
	for _, c := range u.artCols[:3] {
		_, s, v := toHSV(c)
		if sc := s * (0.4 + v); sc > bestS {
			best, bestS = c, sc
		}
	}
	h, s, v := toHSV(best)
	if s < 0.12 { // a grey cover: keep it grey, just readable
		return HSV(h, s, max32(0.55, min32(0.85, v)))
	}
	return HSV(h, min32(0.9, max32(0.45, s)), max32(0.72, min32(0.95, v)))
}

// ---------- views (the Settings window) ----------

// View is what differs between the main window and the popped-out Settings
// window. Both are drawn by the same UI: swapView puts the other window's
// view in place (and the current one into v), and calling it again swaps back.
type View struct {
	Page, Sheet, lastSheet int
	page, sheetA, pw, ph   Spring
	Mini, Full             bool
	miniA, fullA           Tween
	PW, PH, BaseW, BaseH   float32
	WinX, WinY             float32
	hits                   []hitArea
	tips                   []tipZone
	Hover, Press           int
	Base                   *BaseLayer
	BaseAsync              func(p BaseParams)
	baseWant               string
	fluid                  Fluid
	marqueeOn              bool
	playScroll             Scroller
	SetPopView             bool
}

func (u *UI) swapView(v *View) {
	u.Page, v.Page = v.Page, u.Page
	u.Sheet, v.Sheet = v.Sheet, u.Sheet
	u.lastSheet, v.lastSheet = v.lastSheet, u.lastSheet
	u.page, v.page = v.page, u.page
	u.sheetA, v.sheetA = v.sheetA, u.sheetA
	u.pw, v.pw = v.pw, u.pw
	u.ph, v.ph = v.ph, u.ph
	u.Mini, v.Mini = v.Mini, u.Mini
	u.Full, v.Full = v.Full, u.Full
	u.miniA, v.miniA = v.miniA, u.miniA
	u.fullA, v.fullA = v.fullA, u.fullA
	u.PW, v.PW = v.PW, u.PW
	u.PH, v.PH = v.PH, u.PH
	u.BaseW, v.BaseW = v.BaseW, u.BaseW
	u.BaseH, v.BaseH = v.BaseH, u.BaseH
	u.WinX, v.WinX = v.WinX, u.WinX
	u.WinY, v.WinY = v.WinY, u.WinY
	u.hits, v.hits = v.hits, u.hits
	u.tips, v.tips = v.tips, u.tips
	u.Hover, v.Hover = v.Hover, u.Hover
	u.Press, v.Press = v.Press, u.Press
	u.Base, v.Base = v.Base, u.Base
	u.BaseAsync, v.BaseAsync = v.BaseAsync, u.BaseAsync
	u.baseWant, v.baseWant = v.baseWant, u.baseWant
	u.fluid, v.fluid = v.fluid, u.fluid
	u.marqueeOn, v.marqueeOn = v.marqueeOn, u.marqueeOn
	u.playScroll, v.playScroll = v.playScroll, u.playScroll
	u.SetPopView, v.SetPopView = v.SetPopView, u.SetPopView
}

// SettingsView: a fresh view for a Settings window of w x h (logical).
func SettingsView(w, h float32) View {
	v := View{Page: 1, PW: w, PH: h, BaseW: w, BaseH: h, Hover: hNone, Press: hNone, SetPopView: true}
	v.page.Snap(1)
	v.pw.Snap(w)
	v.ph.Snap(h)
	return v
}
