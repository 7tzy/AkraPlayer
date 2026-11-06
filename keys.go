package main

import "fmt"

// Modifier bits (same values Windows uses for RegisterHotKey).
const (
	kAlt   = 0x1
	kCtrl  = 0x2
	kShift = 0x4
	kWin   = 0x8
)

// Mouse buttons can be bound too (handy for push-to-talk).
const (
	vkMButton  = 0x04
	vkXButton1 = 0x05
	vkXButton2 = 0x06
)

func isMouseVK(vk uint32) bool { return vk == vkMButton || vk == vkXButton1 || vk == vkXButton2 }

// shortcutNames: well-known shortcuts a keybind would take over.
var shortcutNames = map[[2]uint32]string{
	{kCtrl, 'A'}: "Select all", {kCtrl, 'C'}: "Copy", {kCtrl, 'V'}: "Paste", {kCtrl, 'X'}: "Cut",
	{kCtrl, 'Z'}: "Undo", {kCtrl, 'Y'}: "Redo", {kCtrl, 'S'}: "Save", {kCtrl, 'F'}: "Find",
	{kCtrl, 'P'}: "Print", {kCtrl, 'N'}: "New", {kCtrl, 'O'}: "Open", {kCtrl, 'W'}: "Close tab",
	{kCtrl, 'T'}: "New tab", {kCtrl, 'R'}: "Reload", {kCtrl, 'L'}: "Address bar", {kCtrl, 'D'}: "Bookmark",
	{kCtrl, 'H'}: "History", {kCtrl, 'J'}: "Downloads", {kCtrl, 'K'}: "Search", {kCtrl, 'B'}: "Bold",
	{kCtrl, 'I'}: "Italic", {kCtrl, 'U'}: "Underline", {kCtrl, 'E'}: "Search", {kCtrl, 'G'}: "Find next",
	{kCtrl, 'Q'}: "Quit (some apps)", {kCtrl, 0x09}: "Next tab", {kCtrl, 0x20}: "Autocomplete",
	{kCtrl, 0x0D}: "Send / open", {kCtrl, 0x08}: "Delete word", {kCtrl, 0x2E}: "Delete word",
	{kCtrl, 0x25}: "Word left", {kCtrl, 0x27}: "Word right", {kCtrl, 0x24}: "Go to start", {kCtrl, 0x23}: "Go to end",
	{kCtrl, 0xBB}: "Zoom in", {kCtrl, 0xBD}: "Zoom out", {kCtrl, '0'}: "Reset zoom",
	{kCtrl | kShift, 'T'}: "Reopen closed tab", {kCtrl | kShift, 'N'}: "New private window / folder",
	{kCtrl | kShift, 0x1B}: "Task Manager", {kCtrl | kShift, 'I'}: "Developer tools",
	{kCtrl | kShift, 'J'}: "Console", {kCtrl | kShift, 'C'}: "Inspect element", {kCtrl | kShift, 'Z'}: "Redo",
	{kCtrl | kShift, 'V'}: "Paste as plain text", {kCtrl | kShift, 'S'}: "Save as", {kCtrl | kShift, 'P'}: "Command palette",
	{kCtrl | kShift, 'M'}: "Discord: mute", {kCtrl | kShift, 'D'}: "Discord: deafen", {kCtrl | kShift, 'B'}: "Bookmarks bar",
	{kCtrl | kShift, 'W'}: "Close window", {kCtrl | kShift, 'Q'}: "Quit browser", {kCtrl | kShift, 'R'}: "Hard reload",
	{kCtrl | kShift, 'E'}: "Explorer (some apps)", {kCtrl | kShift, 0x09}: "Previous tab", {kCtrl | kShift, 'H'}: "Replace",
	{kCtrl | kAlt, 0x2E}: "Security screen", {kCtrl | kAlt, 'T'}: "Terminal (some PCs)",
	{kAlt, 0x73}: "Close window", {kAlt, 0x09}: "Switch apps", {kAlt, 0x0D}: "Fullscreen (games)",
	{kAlt, 0x20}: "Window menu", {kAlt, 0x25}: "Back", {kAlt, 0x27}: "Forward", {kAlt, 'Z'}: "NVIDIA overlay",
	{kAlt, 0x78}: "NVIDIA: record", {kAlt, 0x79}: "NVIDIA: save replay", {kAlt, 'R'}: "AMD overlay",
	{kAlt, 'D'}: "Address bar", {kAlt, 'F'}: "App menu", {kAlt, 'E'}: "App menu", {kAlt, 'H'}: "Help menu",
	{kAlt | kShift, 'B'}: "Spotify: like song", {kAlt | kShift, 0x09}: "Switch apps (back)",
	{kShift, 0x79}: "Context menu", {kShift, 0x2E}: "Delete permanently", {kShift, 0x09}: "Previous field",
	{kWin, 'D'}: "Show desktop", {kWin, 'E'}: "File Explorer", {kWin, 'R'}: "Run", {kWin, 'L'}: "Lock PC",
	{kWin, 'G'}: "Xbox Game Bar", {kWin, 'V'}: "Clipboard history", {kWin, 'I'}: "Settings", {kWin, 'S'}: "Search",
	{kWin, 'A'}: "Quick settings", {kWin, 'N'}: "Notifications", {kWin, 'X'}: "Quick link menu", {kWin, 0x09}: "Task view",
	{kWin | kShift, 'S'}: "Screenshot", {kWin | kAlt, 'R'}: "Game Bar: record", {kWin, 0x2C}: "Screenshot",
	{0, 0x70}: "Help", {0, 0x74}: "Refresh", {0, 0x7A}: "Fullscreen", {0, 0x7B}: "Developer tools (browsers)",
	{0, 0x72}: "Find next", {0, 0x2C}: "Screenshot", {0, 0x2E}: "Delete", {0, 0x2D}: "Insert",
	{0, 0x24}: "Home", {0, 0x23}: "End", {0, 0x21}: "Page up", {0, 0x22}: "Page down",
	{0, 0xB3}: "Media play/pause", {0, 0xB0}: "Media next", {0, 0xB1}: "Media previous",
	{0, 0xAD}: "System mute", {0, 0xAE}: "System volume", {0, 0xAF}: "System volume",
}

// ShortcutConflict says what a keybind would take over, or "" if nothing
// common. It never blocks a key; it's only shown as a heads-up.
// pass = the key still reaches other apps (hold-to-lower keys).
func ShortcutConflict(mods, vk uint32, pass bool) string {
	if vk == 0 {
		return ""
	}
	if n, ok := shortcutNames[[2]uint32{mods, vk}]; ok {
		return "Also " + n
	}
	if pass {
		return ""
	}
	typing := func() string {
		switch {
		case vk == 0x20:
			return "Space"
		case vk == 0x0D:
			return "Enter"
		case vk == 0x09:
			return "Tab"
		case vk == 0x08:
			return "Backspace"
		case vk >= 0x25 && vk <= 0x28:
			return "the arrow key"
		case vk >= 'A' && vk <= 'Z', vk >= '0' && vk <= '9':
			return string(rune(vk))
		case vk >= 0x60 && vk <= 0x6F:
			return "that numpad key"
		case vk >= 0xBA && vk <= 0xDF:
			return "that key"
		}
		return ""
	}()
	if typing == "" {
		return ""
	}
	switch mods {
	case 0:
		return fmt.Sprintf("Blocks typing %s everywhere", typing)
	case kShift:
		return fmt.Sprintf("Blocks Shift+%s while typing", typing)
	}
	return ""
}

// NoModifierHint: shown when a key has no Ctrl/Alt/Shift (allowed, but risky).
func NoModifierHint(mods, vk uint32) bool {
	if mods != 0 || isMouseVK(vk) {
		return false
	}
	return !((vk >= 0x70 && vk <= 0x87) || (vk >= 0xA6 && vk <= 0xB7) || vk == 0x13 || vk == 0x91)
}
