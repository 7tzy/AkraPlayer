//go:build windows

package main

// Key remapping tools (PowerToys Keyboard Manager, AutoHotkey…) swap one key
// for another. Akra Player should record and match the key you end up with,
// not the physical one.
//
// Two ways in:
//  1. PowerToys' own remap list is read directly, so a remapped key records
//     as its new key even when our keyboard hook sees it first.
//  2. Keys typed by other programs (how remappers send the new key) are no
//     longer ignored; while recording, a remapped key that arrives right after
//     the physical one wins.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const akraInputTag = 0x414B5059 // marks keys Akra Player itself types (Like song)

type ptRemap struct {
	OriginalKeys string `json:"originalKeys"`
	NewRemapKeys string `json:"newRemapKeys"`
}

// powerToysRemaps: (mods, vk) → (mods, vk) from Keyboard Manager's active profile.
func powerToysRemaps() map[bind]bind {
	out := map[bind]bind{}
	base := filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "PowerToys", "Keyboard Manager")
	profile := "default"
	if b, err := os.ReadFile(filepath.Join(base, "settings.json")); err == nil {
		var st struct {
			Properties struct {
				ActiveConfiguration struct {
					Value string `json:"value"`
				} `json:"activeConfiguration"`
			} `json:"properties"`
			ActiveConfiguration string `json:"activeConfiguration"`
		}
		if json.Unmarshal(b, &st) == nil {
			if v := st.Properties.ActiveConfiguration.Value; v != "" {
				profile = v
			} else if st.ActiveConfiguration != "" {
				profile = st.ActiveConfiguration
			}
		}
	}
	b, err := os.ReadFile(filepath.Join(base, profile+".json"))
	if err != nil {
		return out
	}
	var cfg struct {
		RemapKeys struct {
			InProcess []ptRemap `json:"inProcess"`
		} `json:"remapKeys"`
		RemapShortcuts struct {
			Global []ptRemap `json:"global"`
		} `json:"remapShortcuts"`
	}
	if json.Unmarshal(b, &cfg) != nil {
		return out
	}
	for _, r := range append(cfg.RemapKeys.InProcess, cfg.RemapShortcuts.Global...) {
		from, ok1 := parsePTKeys(r.OriginalKeys)
		to, ok2 := parsePTKeys(r.NewRemapKeys)
		if ok1 && ok2 {
			out[from] = to
		}
	}
	return out
}

// parsePTKeys reads PowerToys' "162;65" (Left Ctrl + A) style key lists.
func parsePTKeys(s string) (bind, bool) {
	var b bind
	parts := strings.Split(strings.TrimSpace(s), ";")
	for i, p := range parts {
		v, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || v <= 0 {
			return b, false
		}
		vk := uint32(v)
		if i < len(parts)-1 || isModifierKey(vk) && len(parts) > 1 {
			switch vk {
			case 0x11, 0xA2, 0xA3:
				b.mods |= MOD_CONTROL
			case 0x10, 0xA0, 0xA1:
				b.mods |= MOD_SHIFT
			case 0x12, 0xA4, 0xA5:
				b.mods |= MOD_ALT
			case 0x5B, 0x5C:
				b.mods |= MOD_WIN
			default:
				if i == len(parts)-1 {
					b.vk = vk
				}
			}
			continue
		}
		b.vk = vk
	}
	return b, b.vk != 0
}
