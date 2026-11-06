//go:build windows

package main

// Appearance profiles ("Default", "Glass", and your own) and exporting /
// importing settings.

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"
)

// ---------- appearance profiles ----------

const (
	lookMine    = iota // made by you
	lookDefault        // built in: Default
	lookGlass          // built in: Glass
)

// look: everything on the Appearance page.
type look struct {
	name      string
	builtin   int
	accent    int
	custom    uint32
	clarity   float32
	blur      bool
	outline   bool
	fluid     bool
	vis       bool
	glow      bool
	anchor    bool
	fluidMode int
	fluidCol  uint32
	visStyle  int
	visMon    int
	togStyle  int
	font      int
}

func lookOf(c *config) look {
	return look{accent: c.accent, custom: c.custom, clarity: c.clarity, blur: c.blur, outline: c.outline,
		fluid: c.fluid, vis: c.vis, glow: c.glow, anchor: c.anchor, fluidMode: c.fluidMode, fluidCol: c.fluidCol,
		visStyle: c.visStyle, visMon: c.visMon, togStyle: c.togStyle, font: c.font}
}

func (c *config) applyLook(l look) {
	c.accent, c.custom, c.clarity = l.accent, l.custom, l.clarity
	c.blur, c.outline, c.fluid, c.vis, c.glow, c.anchor = l.blur, l.outline, l.fluid, l.vis, l.glow, l.anchor
	c.fluidMode, c.fluidCol, c.visStyle, c.visMon, c.togStyle, c.font = l.fluidMode, l.fluidCol, l.visStyle, l.visMon, l.togStyle, l.font
}

// presetLook: the built-in profiles.
func presetLook(kind int) look {
	d := defaultConfig()
	l := lookOf(&d)
	l.builtin = kind
	l.name = "Default"
	l.fluid, l.fluidMode = true, FluidArt // fluid color from the album cover
	if kind == lookGlass {
		l.name = "Glass"
		l.blur, l.outline = true, false
		l.vis, l.visStyle = true, VisBars
	}
	return l
}

// sameLook compares the settings (not the name, or which screen the edge
// visualizer is on).
func sameLook(a, b look) bool {
	a.name, b.name, a.builtin, b.builtin, a.visMon, b.visMon = "", "", 0, 0, 0, 0
	return a == b
}

func (l look) String() string {
	clean := strings.NewReplacer("\n", " ", "\r", " ", "|", "/").Replace(l.name)
	return fmt.Sprintf("%s|%d|%d|%06x|%.3f|%d|%d|%d|%d|%d|%d|%d|%06x|%d|%d|%d|%d", clean, l.builtin, l.accent, l.custom, l.clarity,
		b2i(l.blur), b2i(l.outline), b2i(l.fluid), b2i(l.vis), b2i(l.glow), b2i(l.anchor),
		l.fluidMode, l.fluidCol, l.visStyle, l.visMon, l.togStyle, l.font)
}

func parseLook(v string) (look, bool) {
	f := strings.Split(v, "|")
	if len(f) != 17 {
		return look{}, false
	}
	n := func(i int) int { x, _ := strconv.Atoi(f[i]); return x }
	hex := func(i int) uint32 { x, _ := strconv.ParseUint(f[i], 16, 32); return uint32(x) }
	cl, _ := strconv.ParseFloat(f[4], 32)
	l := look{name: f[0], builtin: n(1), accent: n(2), custom: hex(3), clarity: float32(cl),
		blur: n(5) != 0, outline: n(6) != 0, fluid: n(7) != 0, vis: n(8) != 0, glow: n(9) != 0, anchor: n(10) != 0,
		fluidMode: n(11), fluidCol: hex(12), visStyle: n(13), visMon: n(14), togStyle: n(15), font: n(16)}
	// keep values in range
	if l.accent >= len(Accents) || l.accent < AccentCover {
		l.accent = 0
	}
	if l.builtin < lookMine || l.builtin > lookGlass {
		l.builtin = lookMine
	}
	l.clarity = max32(0, min32(1, l.clarity))
	l.fluidMode = max(FluidTheme, min(FluidArt, l.fluidMode))
	l.visStyle = max(0, min(NumVisStyles-1, l.visStyle))
	l.togStyle = max(0, min(2, l.togStyle))
	l.font = max(0, min(NumFonts-1, l.font))
	l.visMon = max(0, l.visMon)
	return l, l.name != ""
}

// syncLook stores the current appearance in the selected profile.
func syncLook(c *config) {
	if c.lookIdx >= 0 && c.lookIdx < len(c.looks) {
		l := lookOf(c)
		l.name, l.builtin = c.looks[c.lookIdx].name, c.looks[c.lookIdx].builtin
		c.looks[c.lookIdx] = l
	}
}

// fixLooks makes sure Default and Glass exist. Updating from 2.8 or older
// keeps your current look as "My look" (unless it already matches Default).
func fixLooks(c *config, hadFile bool) {
	has := map[int]bool{}
	for _, l := range c.looks {
		has[l.builtin] = true
	}
	if len(c.looks) == 0 {
		c.looks = []look{presetLook(lookDefault), presetLook(lookGlass)}
		c.lookIdx = 0
		if mine := lookOf(c); hadFile && !sameLook(mine, presetLook(lookDefault)) {
			mine.name = "My look"
			c.looks = append(c.looks, mine)
			c.lookIdx = 2
		} else {
			c.applyLook(c.looks[0])
		}
		return
	}
	if !has[lookDefault] {
		c.looks = append([]look{presetLook(lookDefault)}, c.looks...)
		c.lookIdx++
	}
	if !has[lookGlass] {
		c.looks = append(c.looks, presetLook(lookGlass))
	}
	if c.lookIdx < 0 || c.lookIdx >= len(c.looks) {
		c.lookIdx = 0
	}
}

// syncLookUI copies the profile names into the UI (cheap: runs every frame).
func (a *App) syncLookUI() {
	u, c := a.ui, &a.cfg
	if len(u.Looks) != len(c.looks) {
		u.Looks = make([]string, len(c.looks))
		u.LookBuiltin = make([]bool, len(c.looks))
	}
	for i, l := range c.looks {
		u.Looks[i], u.LookBuiltin[i] = l.name, l.builtin != lookMine
	}
	u.Look = c.lookIdx
	u.LookChanged = false
	if c.lookIdx >= 0 && c.lookIdx < len(c.looks) {
		if b := c.looks[c.lookIdx].builtin; b != lookMine {
			u.LookChanged = !sameLook(lookOf(c), presetLook(b))
		}
	}
}

// applyLookNow puts a look on screen.
func (a *App) applyLookNow(l look) {
	a.cfg.applyLook(l)
	startup := a.ui.Toggles[TogStartup]
	a.applyConfigToUI()
	a.ui.Toggles[TogStartup] = startup
	if !a.cfg.blur {
		a.ui.Backdrop = nil
	}
	a.refreshTrayIcon()
	a.updateMeters()
	a.requestCapture()
	a.kick()
	a.kickPill()
}

func (a *App) switchLook(i int) {
	c := &a.cfg
	if i < 0 || i >= len(c.looks) || i == c.lookIdx {
		return
	}
	a.commitEdit()
	syncLook(c)
	c.lookIdx = i
	a.applyLookNow(c.looks[i])
	saveConfig(a.cfg)
	a.ui.ShowToast("Appearance: " + c.looks[i].name)
}

func (a *App) addLook() {
	c := &a.cfg
	a.commitEdit()
	syncLook(c)
	l := lookOf(c)
	l.name = fmt.Sprintf("Look %d", len(c.looks)+1)
	c.looks = append(c.looks, l)
	c.lookIdx = len(c.looks) - 1
	saveConfig(a.cfg)
	a.startEdit(EditLook, c.lookIdx)
}

func (a *App) deleteLook() {
	c := &a.cfg
	i := c.lookIdx
	if i < 0 || i >= len(c.looks) || c.looks[i].builtin != lookMine {
		return
	}
	a.commitEdit()
	name := c.looks[i].name
	c.looks = append(c.looks[:i], c.looks[i+1:]...)
	c.lookIdx = max(0, i-1)
	a.applyLookNow(c.looks[c.lookIdx])
	saveConfig(a.cfg)
	a.ui.ShowToast("Deleted " + name)
}

// restoreLook puts a built-in profile back the way it came.
func (a *App) restoreLook() {
	c := &a.cfg
	if c.lookIdx < 0 || c.lookIdx >= len(c.looks) || c.looks[c.lookIdx].builtin == lookMine {
		return
	}
	l := presetLook(c.looks[c.lookIdx].builtin)
	l.name = c.looks[c.lookIdx].name
	l.visMon = c.visMon
	c.looks[c.lookIdx] = l
	a.applyLookNow(l)
	saveConfig(a.cfg)
	a.ui.ShowToast(l.name + " restored")
}

// ---------- export / import ----------

var expFiles = [NumExp]string{"general.txt", "appearance.txt", "keybinds.txt"}

const (
	WM_FOLDERPICKED = WM_APP + 12

	BIF_RETURNONLYFSDIRS = 0x1
	BIF_NEWDIALOGSTYLE   = 0x40
)

var (
	pSHBrowseForFolderW   = shell32.NewProc("SHBrowseForFolderW")
	pSHGetPathFromIDListW = shell32.NewProc("SHGetPathFromIDListW")
	pCoTaskMemFree        = ole32.NewProc("CoTaskMemFree")
	pCoUninitialize       = ole32.NewProc("CoUninitialize")

	folderMu     sync.Mutex
	folderResult string
	folderFor    int // 1 export, 2 import
	folderBusy   bool
)

type browseInfo struct {
	Owner       uintptr
	Root        uintptr
	DisplayName *uint16
	Title       *uint16
	Flags       uint32
	Fn          uintptr
	LParam      uintptr
	Image       int32
}

// pickFolder shows Windows' folder picker on its own thread (it needs COM
// set up a certain way) and posts WM_FOLDERPICKED when it closes.
func (a *App) pickFolder(title string, purpose int) {
	folderMu.Lock()
	if folderBusy {
		folderMu.Unlock()
		return
	}
	folderBusy = true
	folderMu.Unlock()
	hwnd := a.hwnd
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		pCoInitializeEx.Call(0, 2) // COINIT_APARTMENTTHREADED
		defer pCoUninitialize.Call()
		name := make([]uint16, 260)
		bi := browseInfo{DisplayName: &name[0], Title: u16(title), Flags: BIF_RETURNONLYFSDIRS | BIF_NEWDIALOGSTYLE}
		pidl, _, _ := pSHBrowseForFolderW.Call(uintptr(unsafe.Pointer(&bi)))
		path := ""
		if pidl != 0 {
			buf := make([]uint16, 1024)
			if r, _, _ := pSHGetPathFromIDListW.Call(pidl, uintptr(unsafe.Pointer(&buf[0]))); r != 0 {
				path = utf16ToString(buf)
			}
			pCoTaskMemFree.Call(pidl)
		}
		folderMu.Lock()
		folderResult, folderFor, folderBusy = path, purpose, false
		folderMu.Unlock()
		pPostMessageW.Call(hwnd, WM_FOLDERPICKED, 0, 0)
	}()
}

func utf16ToString(b []uint16) string {
	n := 0
	for n < len(b) && b[n] != 0 {
		n++
	}
	r := make([]rune, 0, n)
	for i := 0; i < n; i++ {
		c := rune(b[i])
		if c >= 0xd800 && c < 0xdc00 && i+1 < n {
			c = (c-0xd800)<<10 + (rune(b[i+1]) - 0xdc00) + 0x10000
			i++
		}
		r = append(r, c)
	}
	return string(r)
}

func (a *App) onFolderPicked() {
	folderMu.Lock()
	path, purpose := folderResult, folderFor
	folderMu.Unlock()
	u := a.ui
	if u.Pop == PopExport || u.Pop == PopImport {
		a.popFocus = true // back to the Export / Import window
		a.kick()
	} else {
		a.show()
	}
	if path == "" {
		return // cancelled
	}
	u.TransferMsg = ""
	switch purpose {
	case 1: // Export: a different place to save to
		a.expDir = path
		u.ExpDir = path
		u.ExpName = filepath.Base(exportFolder(path, a.cfg.p().name))
	case 2: // Import: a folder from somewhere else
		it, ok := readExport(path)
		if !ok {
			u.TransferMsg = "No Akra Player settings in that folder"
			return
		}
		for k, e := range u.ImpList { // already listed: just select it
			if strings.EqualFold(e.Path, it.Path) {
				a.selectImport(k)
				return
			}
		}
		u.ImpList = append([]ImportItem{it}, u.ImpList...)
		a.selectImport(0)
	}
	a.kick()
}

// exportsDir: the "Exported Profiles" folder next to AkraPlayer.exe (or in
// Documents if that folder can't be written to, like in Program Files).
func exportsDir() string {
	if exe, err := os.Executable(); err == nil {
		d := filepath.Join(filepath.Dir(exe), "Exported Profiles")
		if os.MkdirAll(d, 0755) == nil {
			test := filepath.Join(d, ".akra-write-test")
			if os.WriteFile(test, nil, 0644) == nil {
				os.Remove(test)
				return d
			}
		}
	}
	home, _ := os.UserHomeDir()
	d := filepath.Join(home, "Documents", "Akra Player", "Exported Profiles")
	os.MkdirAll(d, 0755)
	return d
}

// exportFolder: "Akra Player Export - <profile>", numbered if it's taken.
func exportFolder(dir, profile string) string {
	clean := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < 32 {
			return '-'
		}
		return r
	}, strings.TrimSpace(profile))
	if clean == "" {
		clean = "Default"
	}
	base := "Akra Player Export - " + clean
	out := filepath.Join(dir, base)
	for i := 2; ; i++ {
		if _, err := os.Stat(out); os.IsNotExist(err) {
			return out
		}
		out = filepath.Join(dir, fmt.Sprintf("%s (%d)", base, i))
	}
}

// openExport shows the Export settings window.
func (a *App) openExport() {
	u := a.ui
	if a.expDir == "" {
		a.expDir = exportsDir()
	}
	u.ExpDir = a.expDir
	u.ExpName = filepath.Base(exportFolder(a.expDir, a.cfg.p().name))
	u.ExpDone, u.TransferMsg = "", ""
	if u.Pop != PopExport {
		a.openPop(PopExport)
	}
}

// startExport saves the ticked parts into a new folder.
func (a *App) startExport() {
	u := a.ui
	any := false
	for _, on := range u.ExpInc {
		any = any || on
	}
	if !any {
		u.TransferMsg = "Pick at least one thing to export"
		a.kick()
		return
	}
	a.exportTo(a.expDir)
}

func (a *App) exportTo(dir string) {
	u := a.ui
	c := a.cfg
	syncLook(&c)
	out := exportFolder(dir, c.p().name)
	if err := os.MkdirAll(out, 0755); err != nil {
		u.TransferMsg = "Couldn't make the folder there. Try another place."
		a.kick()
		return
	}
	for i := 0; i < NumExp; i++ {
		if !u.ExpInc[i] {
			continue
		}
		var b strings.Builder
		fmt.Fprintf(&b, "# Akra Player %s settings: %s\nversion=%d\n", appVersion, expNames[i], configVersion)
		switch i {
		case ExpGeneral:
			writeGeneral(&b, &c)
		case ExpLook:
			writeLook(&b, &c)
		case ExpKeys:
			writeKeys(&b, &c)
		}
		if err := os.WriteFile(filepath.Join(out, expFiles[i]), []byte(b.String()), 0644); err != nil {
			u.TransferMsg = "Couldn't save " + expFiles[i]
			a.kick()
			return
		}
	}
	u.ExpDone, u.TransferMsg = out, ""
	a.kick()
}

// openFolder opens a folder in File Explorer.
func openFolder(dir string) {
	pShellExecuteW.Call(0, uintptr(unsafe.Pointer(u16("open"))), uintptr(unsafe.Pointer(u16(dir))), 0, 0, 1)
}

// readExport looks at one folder: which parts it has, its name and date.
func readExport(dir string) (ImportItem, bool) {
	it := ImportItem{Name: filepath.Base(dir), Path: dir}
	var newest time.Time
	any := false
	for i, f := range expFiles {
		if st, err := os.Stat(filepath.Join(dir, f)); err == nil && !st.IsDir() {
			it.Has[i], any = true, true
			if st.ModTime().After(newest) {
				newest = st.ModTime()
			}
		}
	}
	if any {
		it.Date = newest.Format("Jan 2, 2006  3:04 PM")
	}
	return it, any
}

// openImport shows the Import settings window with every export it finds.
func (a *App) openImport() {
	u := a.ui
	dir := exportsDir()
	type found struct {
		it ImportItem
		at time.Time
	}
	var list []found
	if ents, err := os.ReadDir(dir); err == nil {
		for _, e := range ents {
			if !e.IsDir() {
				continue
			}
			p := filepath.Join(dir, e.Name())
			if it, ok := readExport(p); ok {
				st, _ := os.Stat(p)
				list = append(list, found{it, st.ModTime()})
			}
		}
	}
	for i := 1; i < len(list); i++ { // newest first
		for j := i; j > 0 && list[j].at.After(list[j-1].at); j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
	u.ImpList = u.ImpList[:0]
	for _, f := range list {
		u.ImpList = append(u.ImpList, f.it)
	}
	u.ImpDone, u.TransferMsg = "", ""
	u.popScroll.Pos.Snap(0)
	a.selectImport(0)
	if u.Pop != PopImport {
		a.openPop(PopImport)
	}
}

func (a *App) selectImport(i int) {
	u := a.ui
	u.ImpSel = -1
	u.ImpInc = [NumExp]bool{}
	if i >= 0 && i < len(u.ImpList) {
		u.ImpSel = i
		u.ImpInc = u.ImpList[i].Has // everything it has, to start with
	}
	a.kick()
}

func (a *App) startImport() {
	u := a.ui
	if !u.canImport() {
		return
	}
	it := u.ImpList[u.ImpSel]
	var inc [NumExp]bool
	for i := range inc {
		inc[i] = u.ImpInc[i] && it.Has[i]
	}
	if got := a.importFrom(it.Path, inc); got != "" {
		u.ImpDone, u.TransferMsg = got, ""
	}
	a.kick()
}

// importFrom reads whichever of the three files the folder has.
func (a *App) importFrom(dir string, inc [NumExp]bool) string {
	u := a.ui
	n := a.cfg
	var got []string
	read := func(i int) (string, bool) {
		if !inc[i] {
			return "", false
		}
		b, err := os.ReadFile(filepath.Join(dir, expFiles[i]))
		if err != nil {
			return "", false
		}
		got = append(got, strings.ToLower(expNames[i]))
		return string(b), true
	}
	if d, ok := read(ExpGeneral); ok {
		parseConfig(&n, d, false)
	}
	if d, ok := read(ExpLook); ok {
		n.looks = nil
		parseConfig(&n, d, false)
		fixLooks(&n, false)
	}
	if d, ok := read(ExpKeys); ok {
		n.profiles = nil
		parseConfig(&n, d, false)
		if len(n.profiles) == 0 {
			n.profiles = a.cfg.profiles
		}
		for i := range n.profiles {
			fixProfile(&n.profiles[i])
		}
		if n.prof < 0 || n.prof >= len(n.profiles) {
			n.prof = 0
		}
	}
	if len(got) == 0 {
		u.TransferMsg = "Nothing to import from that folder"
		return ""
	}
	n.fullscreen = false // (gone since 2.9.5)
	a.leaveProfile()
	a.cfg = n
	startup := u.Toggles[TogStartup]
	a.applyConfigToUI()
	u.Toggles[TogStartup] = startup
	a.setPinned(n.pinned)
	if !n.blur {
		u.Backdrop = nil
	}
	a.playHwnd = 0
	a.resetFolds()
	a.refreshBinds()
	a.registerAll()
	a.refreshTrayIcon()
	a.updateMeters()
	a.requestCapture()
	if !u.Full && !u.Mini {
		a.resizeTo(n.winW, n.winH, hNone)
	}
	saveConfig(a.cfg)
	a.kick()
	s := strings.Join(got, ", ")
	return strings.ToUpper(s[:1]) + s[1:] + " from " + filepath.Base(dir)
}
