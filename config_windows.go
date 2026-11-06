//go:build windows

package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type bind struct{ mods, vk uint32 }

// winTarget remembers a window across restarts (handles change, so keep
// enough to find it again).
type winTarget struct {
	path, class, title string
}

type presetCfg struct {
	id  int
	app string // exe path
	pct int
}

// profile: a named set of keys, presets and volumes ("Gaming", "Studying").
type profile struct {
	name    string
	binds   map[int]bind
	presets []presetCfg
	folders []Folder
	play    winTarget
	curPct  int
	duckPct int
	duckApp string       // exe path; "" = Spotify
	off     map[int]bool // keybinds switched off
}

type config struct {
	profiles   []profile
	prof       int
	clarity    float32
	scrollRes  float32
	pinned     bool
	blur       bool
	art        bool
	toasts     bool
	vis        bool
	outline    bool
	fluid      bool
	anchor     bool
	fluidMode  int
	fluidCol   uint32
	accent     int
	custom     uint32 // 0xRRGGBB
	font       int
	zoom       float32 // 2.4 only (now the window size decides)
	winW       float32 // player size in DIPs
	winH       float32
	visStyle   int
	visMon     int
	openRes    float32
	shine      bool
	glow       bool // cover glow
	fullscreen bool
	togStyle   int
	toastPos   int
	tips       bool // setting tooltips
	declutter  bool
	declLocal  bool // the declutter key only works while Akra Player is in front
	songPopup  bool // popup when the song changes while hidden
	idleFade   bool // fade out while not in use
	trayAccent bool // tray icon in your color (off: Akra red)
	remaining  bool // show time left instead of the song's length
	// 2.9.5
	idleDelay, idleOpacity float32
	songPos, songMon       int
	visSens, visSmooth     float32
	visP                   [NumVisStyles][2]float32
	waveMode               int
	waveCol                uint32
	edgeStyle              int
	looks                  []look // appearance profiles
	lookIdx                int
	x, y                   int32
	hasPos                 bool
}

func (c *config) p() *profile { return &c.profiles[c.prof] }

func newProfile(name string) profile {
	return profile{name: name, binds: map[int]bind{}, off: map[int]bool{}, curPct: 40, duckPct: 20}
}

func defaultProfile() profile {
	p := newProfile("Default")
	p.binds[ActPlay] = bind{MOD_CONTROL | MOD_SHIFT, '8'}
	p.binds[ActShow] = bind{MOD_CONTROL | MOD_SHIFT, '7'}
	p.binds[ActMute] = bind{MOD_CONTROL | MOD_SHIFT, '0'}
	p.binds[ActYouTube] = bind{MOD_CONTROL | MOD_SHIFT, '9'}
	p.presets = []presetCfg{{id: 1, pct: 30}, {id: 2, pct: 50}}
	p.folders = DefaultFolders()
	return p
}

func (p *profile) clone(name string) profile {
	q := *p
	q.name = name
	q.binds = map[int]bind{}
	for k, v := range p.binds {
		q.binds[k] = v
	}
	q.off = map[int]bool{}
	for k, v := range p.off {
		q.off[k] = v
	}
	q.presets = append([]presetCfg(nil), p.presets...)
	q.folders = nil
	for _, f := range p.folders {
		f.Acts = append([]int(nil), f.Acts...)
		q.folders = append(q.folders, f)
	}
	return q
}

func (p *profile) nextPresetID() int {
	id := 1
	for _, ps := range p.presets {
		if ps.id >= id {
			id = ps.id + 1
		}
	}
	return id
}

func defaultConfig() config {
	return config{
		clarity: 0.6, scrollRes: 0.5, blur: false, fluid: true, fluidMode: FluidArt, art: true, toasts: true, outline: true, anchor: true, toastPos: ToastTopRight,
		idleDelay: 3, idleOpacity: 0.28, songPos: ToastTopRight, songMon: -1, visSens: 0.5, visSmooth: 0.3, waveCol: 0xff6b8b,
		visP:   defaultVisP(),
		accent: 0, custom: 0xbf5af2, fluidCol: 0x7b5cff, zoom: 1, openRes: 0.8, winW: 650, winH: 500, glow: true, tips: true,
	}
}

func configDir(name string) string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, name)
}

func loadConfig() config {
	c := defaultConfig()
	data, err := os.ReadFile(filepath.Join(configDir("AkraPlayer"), "settings.txt"))
	if err != nil {
		if old, err2 := os.ReadFile(filepath.Join(configDir("SpotifyToggle"), "settings.txt")); err2 == nil {
			parseConfig(&c, string(old), true)
		}
	} else {
		parseConfig(&c, string(data), false)
	}
	if len(c.profiles) == 0 {
		c.profiles = []profile{defaultProfile()}
	}
	if c.winW <= 0 || c.winH <= 0 { // first run
		c.winW, c.winH = 650, 500
	}
	if c.prof < 0 || c.prof >= len(c.profiles) {
		c.prof = 0
	}
	for i := range c.profiles {
		fixProfile(&c.profiles[i])
	}
	fixLooks(&c, err == nil)
	c.fullscreen = false // the Allow fullscreen switch is gone (2.9.5)
	return c
}

func defaultVisP() (p [NumVisStyles][2]float32) {
	for i := range p {
		p[i] = [2]float32{0.5, 0.5}
	}
	return
}

// fixProfile makes sure every keybind is in exactly one folder.
func fixProfile(p *profile) {
	valid := map[int]bool{}
	for a := 0; a < NumFixed; a++ {
		valid[a] = true
	}
	for _, ps := range p.presets {
		valid[PresetAct(ps.id)] = true
	}
	if len(p.folders) == 0 {
		p.folders = DefaultFolders()
	}
	seen := map[int]bool{}
	for i := range p.folders {
		var keep []int
		for _, a := range p.folders[i].Acts {
			if valid[a] && !seen[a] {
				seen[a] = true
				keep = append(keep, a)
			}
		}
		p.folders[i].Acts = keep
	}
	// new actions go to the folder they belong to by default
	home := map[int]string{}
	for _, f := range DefaultFolders() {
		for _, a := range f.Acts {
			home[a] = f.Name
		}
	}
	all := []int{}
	for a := 0; a < NumFixed; a++ {
		all = append(all, a)
	}
	for _, ps := range p.presets {
		all = append(all, PresetAct(ps.id))
	}
	for _, a := range all {
		if seen[a] {
			continue
		}
		dst := 0
		want := home[a]
		if IsPresetAct(a) {
			want = "Volume"
		}
		for i, f := range p.folders {
			if f.Name == want {
				dst = i
			}
		}
		p.folders[dst].Acts = append(p.folders[dst].Acts, a)
	}
	for k := range p.binds {
		if !valid[k] {
			delete(p.binds, k)
		}
	}
	if p.curPct <= 0 {
		p.curPct = 40
	}
	if p.duckPct < 0 {
		p.duckPct = 20
	}
}

// oldAct maps 2.3 action numbers (0..6) to 2.4 ones.
func oldAct(i int) int {
	switch i {
	case 4:
		return PresetAct(1)
	case 5:
		return PresetAct(2)
	case 6:
		return ActVolCur
	}
	return i
}

// fileVersion reads the "version=N" line (0 for 2.3 and older).
func fileVersion(data string) int {
	for _, line := range strings.Split(data, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "version="); ok {
			n, _ := strconv.Atoi(v)
			return n
		}
	}
	return 0
}

func parseConfig(c *config, data string, legacy bool) {
	ver := fileVersion(data)
	v24, v26, v27, v28 := ver >= 24, ver >= 26, ver >= 27, ver >= 28
	defer func() {
		if !v28 { // from 2.7 and older
			c.fluid, c.blur = true, false // the new defaults
			// colors were reordered: green blue yellow orange red b&w → blue red green yellow orange black white
			switch c.accent {
			case 0:
				c.accent = 2
			case 1:
				c.accent = 0
			case 2:
				c.accent = 3
			case 3:
				c.accent = 4
			case 4:
				c.accent = 1
			case 5:
				c.accent = AccentWhite
			}
		}
		if !v27 { // from 2.6 and older: the new defaults
			c.winW, c.winH = 650, 500
			c.openRes, c.scrollRes = 0.8, 0.5
		}
		if !v26 { // from 2.5 and older
			c.openRes = 0.5 // new default for scrolling into Settings
			for i := range c.profiles {
				for j := range c.profiles[i].folders {
					if c.profiles[i].folders[j].Name == "Music" {
						c.profiles[i].folders[j].Name = "Audio"
					}
				}
			}
		}
	}()
	var cur *profile
	need := func() *profile {
		if cur == nil {
			p := newProfile("Default")
			if !v24 { // 2.3 and older: two presets existed
				p.presets = []presetCfg{{id: 1, pct: 30}, {id: 2, pct: 50}}
			}
			c.profiles = append(c.profiles, p)
			cur = &c.profiles[len(c.profiles)-1]
		}
		return cur
	}
	presetByID := func(p *profile, id int) *presetCfg {
		for i := range p.presets {
			if p.presets[i].id == id {
				return &p.presets[i]
			}
		}
		p.presets = append(p.presets, presetCfg{id: id, pct: 50})
		return &p.presets[len(p.presets)-1]
	}
	for _, line := range strings.Split(data, "\n") {
		k, v, ok := strings.Cut(strings.TrimRight(line, "\r"), "=")
		if !ok {
			continue
		}
		n, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
		pct := int(math.Max(0, math.Min(100, n)))
		switch {
		case k == "pname":
			p := newProfile(v)
			c.profiles = append(c.profiles, p)
			cur = &c.profiles[len(c.profiles)-1]
		case k == "profile":
			c.prof = int(n)
		case legacy && k == "mods":
			b := need().binds[ActPlay]
			b.mods = uint32(n)
			need().binds[ActPlay] = b
		case legacy && k == "vk" && n > 0:
			b := need().binds[ActPlay]
			b.vk = uint32(n)
			need().binds[ActPlay] = b
		case strings.HasPrefix(k, "bind"):
			i, err := strconv.Atoi(k[4:])
			if err != nil {
				continue
			}
			if !v24 {
				i = oldAct(i)
			}
			if m, vk, ok := strings.Cut(v, ","); ok {
				mm, _ := strconv.Atoi(m)
				vv, _ := strconv.Atoi(vk)
				if vv != 0 {
					need().binds[i] = bind{uint32(mm), uint32(vv)}
				}
			}
		case k == "preset": // preset=id|path|pct
			parts := strings.Split(v, "|")
			if len(parts) == 3 {
				id, _ := strconv.Atoi(parts[0])
				pc, _ := strconv.Atoi(parts[2])
				if id > 0 {
					ps := presetByID(need(), id)
					ps.app, ps.pct = parts[1], max(0, min(100, pc))
				}
			}
		case k == "app0" && v != "": // 2.1: exe name of the play/pause app
			need().play.path = v
		case k == "app1" || k == "app2": // 2.1: volume preset apps
			presetByID(need(), int(k[3]-'0')).app = v
		case k == "volapp0" || k == "volapp1": // 2.2-2.3
			presetByID(need(), int(k[6]-'0')+1).app = v
		case k == "vol0" || k == "vol1":
			presetByID(need(), int(k[3]-'0')+1).pct = pct
		case k == "vol2" || k == "curpct":
			need().curPct = pct
		case k == "duckpct":
			need().duckPct = pct
		case k == "duckapp":
			need().duckApp = v
		case k == "off":
			for _, s := range strings.Split(v, ",") {
				if a, err := strconv.Atoi(s); err == nil && CanSwitchOff(a) {
					need().off[a] = true
				}
			}
		case k == "decllocal":
			c.declLocal = n != 0
		case k == "songpopup":
			c.songPopup = n != 0
		case k == "idlefade":
			c.idleFade = n != 0
		case k == "trayaccent":
			c.trayAccent = n != 0
		case k == "remaining":
			c.remaining = n != 0
		case k == "idledelay":
			c.idleDelay = float32(math.Max(1, math.Min(15, n)))
		case k == "idleopacity":
			c.idleOpacity = float32(math.Max(0.05, math.Min(0.9, n)))
		case k == "songpos":
			if int(n) >= 0 && int(n) < len(toastPosNames) {
				c.songPos = int(n)
			}
		case k == "songmon":
			c.songMon = max(-1, int(n))
		case k == "vissens":
			c.visSens = float32(math.Max(0, math.Min(1, n)))
		case k == "vissmooth":
			c.visSmooth = float32(math.Max(0, math.Min(1, n)))
		case k == "visp": // a,b;a,b;... one pair per style
			for i, pair := range strings.Split(v, ";") {
				if i >= NumVisStyles {
					break
				}
				if x, y, ok := strings.Cut(pair, ","); ok {
					fx, _ := strconv.ParseFloat(x, 64)
					fy, _ := strconv.ParseFloat(y, 64)
					c.visP[i] = [2]float32{float32(math.Max(0, math.Min(1, fx))), float32(math.Max(0, math.Min(1, fy)))}
				}
			}
		case k == "wavemode":
			if int(n) >= 0 && int(n) < NumWaveModes {
				c.waveMode = int(n)
			}
		case k == "wavecol":
			if x, err := strconv.ParseUint(strings.TrimSpace(v), 16, 32); err == nil {
				c.waveCol = uint32(x)
			}
		case k == "edgestyle":
			c.edgeStyle = max(0, min(EdgeWave, int(n)))
		case k == "look":
			if l, ok := parseLook(v); ok {
				c.looks = append(c.looks, l)
			}
		case k == "lookidx":
			c.lookIdx = int(n)
		case k == "playpath":
			need().play.path = v
		case k == "playclass":
			need().play.class = v
		case k == "playtitle":
			need().play.title = v
		case k == "folder":
			// folder=Name|act,act|collapsed
			parts := strings.Split(v, "|")
			f := Folder{Name: parts[0]}
			if len(parts) > 1 {
				for _, s := range strings.Split(parts[1], ",") {
					if a, err := strconv.Atoi(s); err == nil {
						if !v24 {
							a = oldAct(a)
						}
						f.Acts = append(f.Acts, a)
					}
				}
			}
			f.Collapsed = len(parts) > 2 && parts[2] == "1"
			p := need()
			p.folders = append(p.folders, f)
		case k == "clarity":
			c.clarity = float32(math.Max(0, math.Min(1, n)))
		case k == "scrollres":
			c.scrollRes = float32(math.Max(0, math.Min(1, n)))
		case k == "pinned":
			c.pinned = n != 0
		case k == "blur":
			c.blur = n != 0
		case k == "art":
			c.art = n != 0
		case k == "toasts":
			c.toasts = n != 0
		case k == "vis":
			c.vis = n != 0
		case k == "outline":
			c.outline = n != 0
		case k == "fluid":
			c.fluid = n != 0
		case k == "anchor":
			c.anchor = n != 0
		case k == "fluidmode":
			if int(n) >= FluidTheme && int(n) <= FluidArt {
				c.fluidMode = int(n)
			}
		case k == "fluidcol":
			if x, err := strconv.ParseUint(strings.TrimSpace(v), 16, 32); err == nil {
				c.fluidCol = uint32(x)
			}
		case k == "winw":
			c.winW = float32(math.Max(0, n))
		case k == "winh":
			c.winH = float32(math.Max(0, n))
		case k == "visstyle":
			if int(n) >= 0 && int(n) < NumVisStyles {
				c.visStyle = int(n)
			}
		case k == "vismon":
			c.visMon = max(0, int(n))
		case k == "openres":
			c.openRes = float32(math.Max(0, math.Min(1, n)))
		case k == "shine":
			c.shine = n != 0
		case k == "fullscreen":
			c.fullscreen = n != 0
		case k == "togstyle":
			if int(n) >= 0 && int(n) <= 2 {
				c.togStyle = int(n)
			}
		case k == "toastpos":
			if int(n) >= 0 && int(n) < len(toastPosNames) {
				c.toastPos = int(n)
			}
		case k == "glow":
			c.glow = n != 0
		case k == "tips":
			c.tips = n != 0
		case k == "declutter":
			c.declutter = n != 0
		case k == "zoom":
			c.zoom = float32(math.Max(0.6, math.Min(4, n)))
		case k == "font":
			if int(n) >= 0 && int(n) < NumFonts {
				c.font = int(n)
			}
		case k == "accent":
			c.accent = int(n)
			if c.accent >= len(Accents) || c.accent < AccentCover {
				c.accent = 0
			}
		case k == "custom":
			if x, err := strconv.ParseUint(strings.TrimSpace(v), 16, 32); err == nil {
				c.custom = uint32(x)
			}
		case k == "x":
			c.x, c.hasPos = int32(n), true
		case k == "y":
			c.y = int32(n)
		}
	}
	// people upgrading get the YouTube key the old YT Toggle extension used
	if !v24 && len(c.profiles) > 0 {
		p := &c.profiles[0]
		yt := bind{MOD_CONTROL | MOD_SHIFT, '9'}
		used := false
		for _, b := range p.binds {
			if b == yt {
				used = true
			}
		}
		if !used {
			p.binds[ActYouTube] = yt
		}
	}
}

const configVersion = 31

// The settings file is written in three parts, the same parts an export can
// include (General, Appearance, Keybinds).
func writeGeneral(b *strings.Builder, c *config) {
	fmt.Fprintf(b, "pinned=%d\nart=%d\ntoasts=%d\ntips=%d\nfullscreen=%d\ntoastpos=%d\n",
		b2i(c.pinned), b2i(c.art), b2i(c.toasts), b2i(c.tips), b2i(c.fullscreen), c.toastPos)
	fmt.Fprintf(b, "scrollres=%.3f\nopenres=%.3f\ndeclutter=%d\nwinw=%.1f\nwinh=%.1f\n",
		c.scrollRes, c.openRes, b2i(c.declutter), c.winW, c.winH)
	fmt.Fprintf(b, "songpopup=%d\nidlefade=%d\ntrayaccent=%d\nremaining=%d\n",
		b2i(c.songPopup), b2i(c.idleFade), b2i(c.trayAccent), b2i(c.remaining))
	fmt.Fprintf(b, "idledelay=%.1f\nidleopacity=%.3f\nsongpos=%d\nsongmon=%d\n", c.idleDelay, c.idleOpacity, c.songPos, c.songMon)
}

func writeLook(b *strings.Builder, c *config) {
	fmt.Fprintf(b, "clarity=%.3f\nblur=%d\nfont=%d\naccent=%d\ncustom=%06x\nglow=%d\nshine=%d\ntogstyle=%d\n",
		c.clarity, b2i(c.blur), c.font, c.accent, c.custom, b2i(c.glow), b2i(c.shine), c.togStyle)
	fmt.Fprintf(b, "vis=%d\noutline=%d\nfluid=%d\nanchor=%d\nfluidmode=%d\nfluidcol=%06x\nvisstyle=%d\nvismon=%d\n",
		b2i(c.vis), b2i(c.outline), b2i(c.fluid), b2i(c.anchor), c.fluidMode, c.fluidCol, c.visStyle, c.visMon)
	fmt.Fprintf(b, "vissens=%.3f\nvissmooth=%.3f\nwavemode=%d\nwavecol=%06x\nedgestyle=%d\n",
		c.visSens, c.visSmooth, c.waveMode, c.waveCol, c.edgeStyle)
	var vp []string
	for _, p := range c.visP {
		vp = append(vp, fmt.Sprintf("%.3f,%.3f", p[0], p[1]))
	}
	fmt.Fprintf(b, "visp=%s\n", strings.Join(vp, ";"))
	for _, l := range c.looks {
		fmt.Fprintf(b, "look=%s\n", l.String())
	}
	fmt.Fprintf(b, "lookidx=%d\n", c.lookIdx)
}

func writeKeys(b *strings.Builder, c *config) {
	clean := func(s string) string { return strings.NewReplacer("\n", " ", "\r", " ", "|", "/").Replace(s) }
	fmt.Fprintf(b, "decllocal=%d\n", b2i(c.declLocal))
	fmt.Fprintf(b, "profile=%d\n", c.prof)
	for _, p := range c.profiles {
		fmt.Fprintf(b, "pname=%s\n", clean(p.name))
		for act, bd := range p.binds {
			if bd.vk != 0 {
				fmt.Fprintf(b, "bind%d=%d,%d\n", act, bd.mods, bd.vk)
			}
		}
		for _, ps := range p.presets {
			fmt.Fprintf(b, "preset=%d|%s|%d\n", ps.id, clean(ps.app), ps.pct)
		}
		fmt.Fprintf(b, "playpath=%s\nplayclass=%s\nplaytitle=%s\n", clean(p.play.path), clean(p.play.class), clean(p.play.title))
		fmt.Fprintf(b, "curpct=%d\nduckpct=%d\nduckapp=%s\n", p.curPct, p.duckPct, clean(p.duckApp))
		var offs []string
		for a, o := range p.off {
			if o {
				offs = append(offs, strconv.Itoa(a))
			}
		}
		if len(offs) > 0 {
			fmt.Fprintf(b, "off=%s\n", strings.Join(offs, ","))
		}
		for _, f := range p.folders {
			acts := make([]string, len(f.Acts))
			for i, a := range f.Acts {
				acts[i] = strconv.Itoa(a)
			}
			fmt.Fprintf(b, "folder=%s|%s|%d\n", clean(f.Name), strings.Join(acts, ","), b2i(f.Collapsed))
		}
	}
}

func saveConfig(c config) {
	syncLook(&c) // the selected appearance profile follows every change
	dir := configDir("AkraPlayer")
	os.MkdirAll(dir, 0755)
	var b strings.Builder
	fmt.Fprintf(&b, "version=%d\n", configVersion)
	writeGeneral(&b, &c)
	writeLook(&b, &c)
	if c.hasPos {
		fmt.Fprintf(&b, "x=%d\ny=%d\n", c.x, c.y)
	}
	writeKeys(&b, &c)
	os.WriteFile(filepath.Join(dir, "settings.txt"), []byte(b.String()), 0644)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
