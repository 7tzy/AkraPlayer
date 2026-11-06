//go:build !windows

// Dev-only tool (not part of the Windows exe):
//
//	go run . -preview out.png   renders the UI over a fake desktop
//	go run . -syso path         writes the Windows icon resource object
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"strings"
)

const pyText = `
import sys
from PIL import ImageFont, Image, ImageDraw
path, size, s = sys.argv[1], float(sys.argv[2]), sys.argv[3]
f = ImageFont.truetype(path, max(1, round(size)))
asc, desc = f.getmetrics()
w = int(f.getlength(s)) + 2
h = int((asc + desc) * 1.08)
im = Image.new("L", (w, h), 0)
ImageDraw.Draw(im).text((1, int((asc+desc)*0.04)), s, font=f, fill=255)
sys.stdout.buffer.write(w.to_bytes(4, "little") + h.to_bytes(4, "little") + im.tobytes())
`

type pilText struct{}

func (pilText) Render(s string, px float32, weight, font int) *Mask {
	f := "fonts/Inter-Medium.otf"
	switch {
	case font == FontPoppins && weight >= 700:
		f = "fonts/Poppins-Bold.ttf"
	case font == FontPoppins:
		f = "fonts/Poppins-Medium.ttf"
	case weight >= 700:
		f = "fonts/InterDisplay-SemiBold.otf"
	case weight >= 600:
		f = "fonts/Inter-SemiBold.otf"
	}
	out, err := exec.Command("python3", "-c", pyText, f, fmt.Sprint(px), s).Output()
	if err != nil || len(out) < 8 {
		return &Mask{}
	}
	w := int(binary.LittleEndian.Uint32(out[0:]))
	h := int(binary.LittleEndian.Uint32(out[4:]))
	m := &Mask{W: w, H: h, A: make([]float32, w*h)}
	for i := range m.A {
		m.A[i] = float32(out[8+i]) / 255
	}
	return m
}

// fake app icons for the preview
func fakeIcon(key string, px int) *Canvas {
	c := NewCanvas(px, px, float32(px)/32)
	cols := map[string]uint32{"spotify": 0x1ed760, "discord": 0x5865f2, "valorant": 0xfa4454, "vlc": 0xff8800, "chrome": 0x4285f4}
	col, ok := cols[key]
	if !ok {
		return nil
	}
	c.RoundRect(0, 0, 32, 32, 8, VGrad(0, 32, Hex(col, 1).Mix(White(1), 0.2), Hex(col, 1)))
	c.Circle(16, 16, 8, Solid(White(0.9)))
	c.Circle(16, 16, 4, Solid(Hex(col, 1)))
	return c
}

func main() {
	prev := flag.String("preview", "", "write preview png")
	syso := flag.String("syso", "", "write icon .syso")
	clar := flag.Float64("clarity", 0.6, "clarity")
	state := flag.String("state", "playing", "playing|paused|off")
	page := flag.Float64("page", 0, "page 0=player 1=settings")
	sec := flag.Int("section", 0, "settings section")
	scroll := flag.Float64("scroll", 0, "scroll offset of the current section/sheet")
	over := flag.Float64("over", 0, "overscroll push")
	accent := flag.Int("accent", 0, "accent index, -1 custom, -2 rainbow")
	hover := flag.Int("hover", 0, "hover id")
	rec := flag.Int("rec", -1, "recording keybind index")
	blur := flag.Bool("blur", false, "background blur")
	art := flag.Bool("art", false, "album art layout")
	noimg := flag.Bool("noimg", false, "art layout without an image")
	toast := flag.String("toast", "", "toast text")
	pick := flag.Int("pick", -1, "picker mode")
	sheet := flag.Int("sheet", 0, "sheet")
	font := flag.Int("font", 0, "font")
	edit := flag.Bool("edit", false, "rename a folder")
	hue := flag.Bool("hue", false, "custom color picker")
	push := flag.Float64("push", 0, "keep-scrolling pill fill (0..1)")
	vis := flag.Bool("vis", false, "bottom visualizer")
	fluid := flag.Int("fluid", -1, "fluid background mode (0 theme, 1 custom, 2 album)")
	mini := flag.Bool("mini", false, "mini mode")
	full := flag.Bool("full", false, "fullscreen (simulated 1280x720 screen)")
	hold := flag.Float64("hold", 0, "enlarge button hold progress")
	tip := flag.Bool("tip", false, "show the enlarge tooltip")
	spotvol := flag.Bool("spotvol", false, "Spotify volume bar open")
	overlay := flag.String("overlay", "", "render an overlay instead: pill | toast")
	noOutline := flag.Bool("nooutline", false, "window outline off")
	liked := flag.Bool("liked", false, "song liked")
	visStyle := flag.Int("visstyle", 0, "visualizer style (0 bars, 1 halo, 2 edge, 3 threads, 4 glow, 5 mesh)")
	size := flag.String("size", "", "player size in logical units, e.g. 520x340")
	shine := flag.Bool("shine", false, "button shine")
	declutter := flag.Bool("declutter", false, "declutter mode")
	pvol := flag.Bool("pvol", false, "pill/mini volume bar open")
	artIn := flag.Float64("artin", 1, "cover entrance progress (0..1)")
	playPush := flag.Float64("playpush", 0, "scroll-to-settings pill on the player (0..1)")
	togStyle := flag.Int("togstyle", 0, "switched-on look (0 edge glow, 1 frosted, 2 classic)")
	bgOpen := flag.Bool("bgopen", false, "Background dropdown open")
	popPin := flag.Bool("poppin", false, "pop-out pinned")
	expOpen := flag.Bool("expopen", false, "export panel open")
	sleep := flag.Int("sleep", 0, "sleep timer choice")
	togEdit := flag.Int("togedit", -1, "open a feature's settings (toggle index)")
	setPop := flag.Bool("setpop", false, "draw the popped-out Settings window")
	waveMode := flag.Int("wavemode", 0, "waveform colors")
	edgeStyle := flag.Int("edgestyle", 0, "screen edge look")
	remaining := flag.Bool("remaining", false, "show time left")
	bindOff := flag.Bool("bindoff", false, "YouTube keybind switched off")
	playGone := flag.Bool("playgone", false, "the chosen play/pause window was closed")
	lookChanged := flag.Bool("lookchanged", false, "the selected built-in look was changed")
	exticons := flag.String("exticons", "", "write browser-extension icons to this folder")
	flag.Parse()
	if *exticons != "" {
		for _, n := range []int{16, 48, 128} {
			c := NewCanvas(n, n, float32(n)/32)
			DrawBrandIcon(c, 1, 1, 30)
			savePNG(fmt.Sprintf("%s/icon%d.png", *exticons, n), c)
		}
	}
	if *syso != "" {
		if err := writeSyso(*syso); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}
	if *prev == "" {
		return
	}
	u := NewUI(pilText{})
	u.Icons = fakeIcon
	u.Clarity = float32(*clar)
	u.Font = *font
	u.Binds = map[int][]string{ActPlay: {"Ctrl", "Shift", "8"}, ActShow: {"Ctrl", "Shift", "7"}, ActMute: {"F7"},
		ActAppPlay: {"F8"}, PresetAct(1): {"Ctrl", "Alt", "Shift", "Num 1"}, PresetAct(2): {"F10"}, ActDuck: {"Mouse 4"},
		ActYouTube: {"Ctrl", "Shift", "9"}, ActLike: {"Alt", "Shift", "B"}}
	u.BindWarn[ActLike] = "Also Spotify: like song"
	u.BindWarn[ActDuck] = "Also Discord push-to-talk (still works)"
	u.Title, u.Artist = "Blinding Lights", "The Weeknd"
	u.T = 1.3
	u.AccentIdx = *accent
	u.Toggles[TogStartup] = true
	u.Toggles[TogBlur] = *blur
	u.Toggles[TogArt] = *art
	u.Toggles[TogVis] = *vis
	u.Toggles[TogOutline] = !*noOutline
	if *fluid >= 0 {
		u.Toggles[TogFluid] = true
		u.FluidMode = *fluid
	}
	u.Liked = *liked
	u.VisStyle = *visStyle
	u.Toggles[TogGlow] = !*shine
	u.Declutter = *declutter
	u.TogStyle, u.BgOpen, u.PopPinned = *togStyle, *bgOpen, *popPin
	u.Looks, u.LookBuiltin, u.LookChanged = []string{"Default", "Glass", "My look"}, []bool{true, true, false}, *lookChanged
	u.ExpOpen, u.PlayGone = *expOpen, *playGone
	u.Sleep, u.SleepLeft, u.Remaining = *sleep, 22*60+10, *remaining
	u.WaveMode, u.EdgeStyle = *waveMode, *edgeStyle
	if *setPop {
		u.PoppedOut, u.SetPopView = true, true
	}
	if *togEdit >= 0 {
		u.Toggles[*togEdit] = true
		u.TogEdit, u.togEditShown = *togEdit, *togEdit
		u.editA.V = 1
	}
	u.expA.V = b2f(*expOpen)
	u.BindOff[ActYouTube] = *bindOff
	u.Binds[ActDeclutter] = []string{"Ctrl", "Shift", "D"}
	u.DeclLocal = true
	u.Toggles[TogFluid] = *fluid != -2
	if *bgOpen {
		u.bgA.V = 1
	}
	u.MixerApps = []MixerApp{{"Spotify", "spotify", 0.7, false}, {"Discord", "discord", 0.45, false},
		{"Valorant", "valorant", 0.9, false}, {"Chrome", "chrome", 0.3, true}, {"VLC", "vlc", 1, false}}
	u.Monitors = []string{"Display 1 (main)", "Display 2"}
	if *size != "" {
		fmt.Sscanf(*size, "%fx%f", &u.BaseW, &u.BaseH)
	}
	u.RecIdx = *rec
	u.PlayTarget = Target{"VLC media player", "VLC", "vlc"}
	u.Presets = []PresetUI{{ID: 1, App: Target{"Discord", "Discord", "discord"}, Pct: 30}, {ID: 2, App: Target{"Spotify", "Spotify", "spotify"}, Pct: 70}}
	u.Profiles = []string{"Default", "Gaming", "Studying"}
	u.Profile = 1
	u.SpotVol = 0.62
	u.Pos, u.Dur, u.PosT, u.ProgKnown, u.Seekable = 84, 200, 1.3, true, true
	for i := 0; i < 64; i++ { // a plausible level history
		v := 0.35 + 0.3*math.Abs(math.Sin(float64(i)*0.55)) + 0.25*math.Abs(math.Sin(float64(i)*1.9+1))
		u.Levels = append(u.Levels, float32(v)*0.8)
	}
	if *art && !*noimg {
		u.Art = sampleArt()
		u.ArtVer = 1
	}
	switch *state {
	case "playing":
		u.Status = StPlaying
	case "paused":
		u.Status = StPaused
	default:
		u.Status = StNotRunning
	}
	u.Page, u.Section, u.Sheet = int(*page+0.5), *sec, *sheet
	if *edit {
		u.EditKind, u.EditIdx, u.EditText = EditFolder, 1, "Gaming"
	}
	if *hue {
		u.Custom = Hex(0xbf5af2, 1)
		if *fluid >= 0 {
			u.OpenHue(HueFluid)
		} else {
			u.OpenHue(HueAccent)
		}
	}
	u.Mini = *mini
	if *full {
		u.Full, u.BaseW, u.BaseH = true, 1280/1.5, 720/1.5
	}
	if *pick >= 0 {
		u.PickOpen, u.PickMode = true, *pick
		switch *pick {
		case PickWindow:
			u.PickItems = []PickItem{
				{"Blinding Lights", "Spotify", "spotify", true, false},
				{"VLC media player", "VLC", "vlc", true, true},
				{"VALORANT", "VALORANT", "valorant", true, false},
				{"#general - Discord", "Discord", "discord", false, false},
				{"YouTube - Google Chrome", "Chrome", "chrome", false, false},
			}
		case PickFolder:
			u.PickSlot = PresetAct(1)
			u.PickItems = []PickItem{{Name: "Music"}, {Name: "Volume", Selected: true}, {Name: "Player"}, {Name: "+  New folder"}}
		}
	}
	u.SnapAnimations()
	u.miniA.V, u.fullA.V = b2f(u.Mini), b2f(u.Full)
	u.page.Snap(float32(*page))
	if *hold > 0 {
		u.HoldID, u.HoldT, u.Press = hEnlarge, float32(*hold), hEnlarge
	}
	if *tip {
		u.TipID, u.tipA.V = hEnlarge, 1
	}
	if *spotvol {
		u.spotVolA.V = 1
	}
	u.likeA.V = b2f(u.Liked)
	u.declA.V = b2f(u.Declutter)
	if *pvol {
		u.pvol[0].V, u.pvol[1].V = 1, 1
	}
	u.peakRef = 0.6
	u.SnapPanel()
	u.artIn, u.artInVer = float32(*artIn), u.ArtVer
	if *playPush > 0 {
		u.playScroll.Over = float32(*playPush) * u.openLimit()
		u.pushA.V, u.pushF.X, u.pushDir = 1, float32(*playPush), 1
	}
	if *page >= 0.5 {
		u.secScroll[u.Section].Max = 1000
		u.secScroll[u.Section].Pos.Snap(float32(*scroll))
		u.secScroll[u.Section].Over = float32(*over)
	} else {
		u.sheetScroll.Max = 1000
		u.sheetScroll.Pos.Snap(float32(*scroll))
	}
	if *push > 0 {
		u.pushA.V, u.pushF.X, u.pushDir = 1, float32(*push), 1
	}
	u.Hover = *hover
	if *hover != 0 {
		u.hov[*hover] = &Tween{V: 1}
	}
	if *toast != "" {
		u.ShowToast(*toast)
		u.toastA.V = 1
	}
	switch *overlay {
	case "pill":
		renderOverlay(*prev, func(c *Canvas) { u.RenderPill(c, 0, 0) }, PillW, PillH)
		return
	case "tip":
		txt := togTips[TogToasts]
		w, h := u.TipBubbleSize(txt)
		renderOverlay(*prev, func(c *Canvas) { u.RenderTipBubble(c, txt) }, w, h)
		return
	case "edge":
		renderOverlay(*prev, func(c *Canvas) { u.RenderEdge(c) }, 900, 110)
		return
	case "popmixer", "popkeys", "popexport", "popimport", "popexported", "popimported":
		kind := PopMixer
		switch *overlay {
		case "popkeys":
			kind = PopKeys
		case "popexport", "popexported":
			kind = PopExport
			u.ExpName = "Akra Player Export - Gaming"
			u.ExpDir = `C:\Users\philb\Desktop\AkraPlayer\Exported Profiles`
			if *overlay == "popexported" {
				u.ExpDone = u.ExpDir + `\` + u.ExpName
			}
		case "popimport", "popimported":
			kind = PopImport
			u.ImpList = []ImportItem{
				{Name: "Akra Player Export - Gaming", Date: "Nov 6, 2026  3:20 AM", Has: [NumExp]bool{true, true, true}},
				{Name: "Akra Player Export - Studying", Date: "Nov 5, 2026  9:41 PM", Has: [NumExp]bool{false, true, true}},
				{Name: "Akra Player settings 2026-11-06", Date: "Nov 6, 2026  2:31 AM", Has: [NumExp]bool{true, true, true}},
			}
			u.ImpSel, u.ImpInc = 1, [NumExp]bool{false, true, true}
			if *overlay == "popimported" {
				u.ImpDone = "Appearance, keybinds from Akra Player Export - Studying"
			}
		}
		u.Pop = kind
		w, h := u.PopSize(kind)
		renderOverlay(*prev, func(c *Canvas) { u.RenderPopup(c, kind, 0, 0) }, w, h)
		return
	case "toast":
		w := u.ToastWidth("Discord", "30%")
		renderOverlay(*prev, func(c *Canvas) { u.RenderKeyToast(c, "discord", "Discord", "30%", 1) }, w, ToastH)
		return
	}
	renderPreview(*prev, u, *blur)
}

func renderOverlay(path string, draw func(c *Canvas), lw, lh float32) {
	scale := float32(1.5)
	w, h := int(lw*scale+0.99), int(lh*scale+0.99)
	DW, DH := w+80, h+60
	d := NewCanvas(DW, DH, 1)
	d.Fill(0, 0, float32(DW), float32(DH), func(x, y float32) float32 { return -1 },
		func(x, y float32) Col { return lerpCol(Hex(0x22324a, 1), Hex(0x8a5a3a, 1), (x+y)/float32(DW+DH)) }, 0, nil)
	c := NewCanvas(w, h, scale)
	draw(c)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			si := (y*w + x) * 4
			di := ((y+30)*DW + x + 40) * 4
			a := c.Pix[si+3]
			for k := 0; k < 3; k++ {
				d.Pix[di+k] = c.Pix[si+k] + d.Pix[di+k]*(1-a)
			}
		}
	}
	savePNG(path, d)
}

func sampleArt() *Canvas {
	c := NewCanvas(300, 300, 1)
	c.Fill(0, 0, 300, 300, func(x, y float32) float32 { return -1 }, func(x, y float32) Col {
		return lerpCol(Hex(0xb3122a, 1), Hex(0x1a0508, 1), clamp01((x*0.3+y)/360))
	}, 0, nil)
	c.Circle(170, 120, 70, Solid(Hex(0xff5468, 0.85)))
	c.Circle(170, 120, 40, Solid(Hex(0xffd0d6, 0.6)))
	c.RoundRect(40, 220, 220, 14, 7, Solid(White(0.75)))
	c.RoundRect(40, 244, 140, 10, 5, Solid(White(0.45)))
	return c
}

func renderPreview(path string, u *UI, blur bool) {
	scale := float32(1.5)
	w, h := u.WinSize(scale)
	DW, DH := w+240, h+140
	d := NewCanvas(DW, DH, 1)
	d.Fill(0, 0, float32(DW), float32(DH), func(x, y float32) float32 { return -1 },
		func(x, y float32) Col {
			t := (x + y) / float32(DW+DH)
			return lerpCol(Hex(0x3b2a6b, 1), Hex(0xe0794a, 1), t)
		}, 0, nil)
	d.Circle(float32(DW)*0.80, float32(DH)*0.22, 130, Solid(Hex(0xffc75f, 0.95)))
	d.RoundRect(40, 50, float32(DW)*0.55, float32(DH)-110, 10, Solid(Hex(0xf6f7f9, 1)))
	d.RoundRect(40, 50, float32(DW)*0.55, 36, 10, Solid(Hex(0xe3e6ea, 1)))
	for i := 0; i < 16; i++ {
		ww := float32(DW)*0.45 - float32((i*37)%120)
		d.RoundRect(64, 110+float32(i)*26, ww, 10, 5, Solid(Hex(0xb9c0c9, 1)))
	}
	d.RoundRect(64, 110, 180, 14, 6, Solid(Hex(0x2f6fde, 1)))
	ex := float32(DW) * 0.50
	d.RoundRect(ex, 150, float32(DW)-ex-30, float32(DH)-190, 10, Solid(Hex(0x1e1f24, 0.97)))
	cols := []uint32{0xc678dd, 0x98c379, 0x61afef, 0xe5c07b, 0xabb2bf}
	for i := 0; i < 17; i++ {
		x := ex + 24 + float32((i*3)%5)*14
		d.RoundRect(x, 180+float32(i)*22, 90+float32((i*53)%160), 8, 4, Solid(Hex(cols[i%5], 0.85)))
	}
	ox, oy := 130, 70
	if blur { // the app's own backdrop path: capture behind the window at 1/4 size, then blur
		region := NewCanvas(w, h, 1)
		for y := 0; y < h; y++ {
			copy(region.Pix[y*w*4:(y+1)*w*4], d.Pix[((y+oy)*DW+ox)*4:((y+oy)*DW+ox+w)*4])
		}
		bd := region.Resized(w/4, h/4)
		bd.BoxBlur(3)
		u.Backdrop, u.BackdropScale, u.BackdropVer = bd, 4, 1
	}
	c := NewCanvas(w, h, scale)
	u.Render(c)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			si := (y*w + x) * 4
			di := ((y+oy)*DW + x + ox) * 4
			a := c.Pix[si+3]
			for k := 0; k < 3; k++ {
				d.Pix[di+k] = c.Pix[si+k] + d.Pix[di+k]*(1-a)
			}
			d.Pix[di+3] = 1
		}
	}
	savePNG(path, d)
}

func savePNG(path string, c *Canvas) {
	img := image.NewNRGBA(image.Rect(0, 0, c.W, c.H))
	buf := make([]byte, c.W*c.H*4)
	c.BGRA(buf, false)
	for i := 0; i < len(buf); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = buf[i+2], buf[i+1], buf[i], buf[i+3]
	}
	f, _ := os.Create(path)
	png.Encode(f, img)
	f.Close()
}

// ---------- Windows icon resource (.syso) ----------

// putAll writes each value little-endian (binary.Write can't take []any).
func putAll(b *bytes.Buffer, vals ...any) {
	for _, v := range vals {
		if err := binary.Write(b, binary.LittleEndian, v); err != nil {
			panic(err)
		}
	}
}

func iconCanvas(s int) *Canvas {
	c := NewCanvas(s, s, float32(s)/32)
	DrawBrandIcon(c, 1.5, 1.2, 29)
	return c
}

func iconDIB(s int) []byte {
	c := iconCanvas(s)
	px := make([]byte, s*s*4)
	c.BGRA(px, false)
	var b bytes.Buffer
	putAll(&b,
		uint32(40), int32(s), int32(s*2), uint16(1), uint16(32), uint32(0), uint32(0),
		int32(0), int32(0), uint32(0), uint32(0),
	)
	for y := s - 1; y >= 0; y-- { // bottom-up
		b.Write(px[y*s*4 : (y+1)*s*4])
	}
	maskRow := ((s + 31) / 32) * 4
	b.Write(make([]byte, maskRow*s)) // AND mask (alpha does the work)
	return b.Bytes()
}

func iconPNG(s int) []byte {
	c := iconCanvas(s)
	img := image.NewNRGBA(image.Rect(0, 0, s, s))
	buf := make([]byte, s*s*4)
	c.BGRA(buf, false)
	for i := 0; i < len(buf); i += 4 {
		img.Set((i/4)%s, (i/4)/s, color.NRGBA{buf[i+2], buf[i+1], buf[i], buf[i+3]})
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes()
}

func writeSyso(path string) error {
	sizes := []int{16, 20, 24, 32, 40, 48, 64, 256}
	var imgs [][]byte
	for _, s := range sizes {
		if s >= 256 {
			imgs = append(imgs, iconPNG(s))
		} else {
			imgs = append(imgs, iconDIB(s))
		}
	}
	var grp bytes.Buffer
	binary.Write(&grp, binary.LittleEndian, []uint16{0, 1, uint16(len(sizes))})
	for i, s := range sizes {
		bw := byte(s)
		if s >= 256 {
			bw = 0
		}
		grp.Write([]byte{bw, bw, 0, 0})
		putAll(&grp, uint16(1), uint16(32), uint32(len(imgs[i])), uint16(i+1))
	}

	type res struct {
		typ, id uint32
		data    []byte
	}
	var list []res
	for i := range imgs {
		list = append(list, res{3, uint32(i + 1), imgs[i]})
	}
	list = append(list, res{14, 1, grp.Bytes()})

	// layout: root | typeDir(3) | typeDir(14) | langDirs | dataEntries | blobs
	n := len(list)
	off := 0
	rootOff := off
	off += 16 + 2*8
	type3Off := off
	off += 16 + (n-1)*8
	type14Off := off
	off += 16 + 8
	langOff := off
	off += n * (16 + 8)
	dataEntOff := off
	off += n * 16
	blobOff := make([]int, n)
	for i, r := range list {
		off = (off + 7) &^ 7
		blobOff[i] = off
		off += len(r.data)
	}
	sec := make([]byte, (off+7)&^7)
	le := binary.LittleEndian
	dir := func(at, count int) {
		le.PutUint16(sec[at+14:], uint16(count))
	}
	entry := func(at int, id uint32, target int, isDir bool) {
		le.PutUint32(sec[at:], id)
		v := uint32(target)
		if isDir {
			v |= 0x80000000
		}
		le.PutUint32(sec[at+4:], v)
	}
	dir(rootOff, 2)
	entry(rootOff+16, 3, type3Off, true)
	entry(rootOff+24, 14, type14Off, true)
	dir(type3Off, n-1)
	dir(type14Off, 1)
	var relocs []int
	for i, r := range list {
		ld := langOff + i*24
		if r.typ == 3 {
			entry(type3Off+16+i*8, r.id, ld, true)
		} else {
			entry(type14Off+16, r.id, ld, true)
		}
		dir(ld, 1)
		de := dataEntOff + i*16
		entry(ld+16, 0x409, de, false)
		le.PutUint32(sec[de:], uint32(blobOff[i])) // RVA, fixed up by relocation
		le.PutUint32(sec[de+4:], uint32(len(r.data)))
		relocs = append(relocs, de)
		copy(sec[blobOff[i]:], r.data)
	}

	var out bytes.Buffer
	hdrSize := 20 + 40
	relocAt := hdrSize + len(sec)
	symAt := relocAt + len(relocs)*10
	// COFF file header
	putAll(&out, uint16(0x8664), uint16(1), uint32(0), uint32(symAt), uint32(1), uint16(0), uint16(0))
	// section header
	out.WriteString(".rsrc\x00\x00\x00")
	putAll(&out, uint32(0), uint32(0), uint32(len(sec)), uint32(hdrSize), uint32(relocAt), uint32(0),
		uint16(len(relocs)), uint16(0), uint32(0x40000040))
	out.Write(sec)
	for _, r := range relocs {
		putAll(&out, uint32(r), uint32(0), uint16(3)) // IMAGE_REL_AMD64_ADDR32NB
	}
	// symbol table: .rsrc section symbol
	out.WriteString(".rsrc\x00\x00\x00")
	putAll(&out, uint32(0), int16(1), uint16(0), uint8(3), uint8(0))
	binary.Write(&out, le, uint32(4)) // empty string table
	if !strings.HasSuffix(path, ".syso") {
		return fmt.Errorf("path must end in .syso")
	}
	return os.WriteFile(path, out.Bytes(), 0644)
}
