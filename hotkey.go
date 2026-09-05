package main

import (
	"fmt"
	"strings"
)

// Win32 RegisterHotKey modifier bits (MOD_*). Declared here rather than in
// hotkey_windows.go so the parser — and its tests — build on every platform.
const (
	modAlt     uint32 = 0x0001
	modControl uint32 = 0x0002
	modShift   uint32 = 0x0004
	modWin     uint32 = 0x0008
	// MOD_NOREPEAT stops Windows firing the hotkey repeatedly while held.
	modNoRepeat uint32 = 0x4000
)

// Hotkey is a parsed global shortcut.
type Hotkey struct {
	Mods uint32 // MOD_* bits, without MOD_NOREPEAT
	VK   uint32 // virtual-key code
}

// namedKeys maps the key names we accept to virtual-key codes. Letters and
// digits are handled arithmetically (VK_A == 'A'), so only the rest is listed.
var namedKeys = map[string]uint32{
	"space": 0x20, "enter": 0x0D, "return": 0x0D, "tab": 0x09, "esc": 0x1B,
	"escape": 0x1B, "backspace": 0x08, "delete": 0x2E, "insert": 0x2D,
	"home": 0x24, "end": 0x23, "pageup": 0x21, "pagedown": 0x22,
	"left": 0x25, "up": 0x26, "right": 0x27, "down": 0x28,
	"f1": 0x70, "f2": 0x71, "f3": 0x72, "f4": 0x73, "f5": 0x74, "f6": 0x75,
	"f7": 0x76, "f8": 0x77, "f9": 0x78, "f10": 0x79, "f11": 0x7A, "f12": 0x7B,
}

var modAliases = map[string]uint32{
	"ctrl": modControl, "control": modControl,
	"alt": modAlt, "option": modAlt,
	"shift": modShift,
	"win":   modWin, "super": modWin, "meta": modWin, "cmd": modWin,
}

// ParseHotkey turns "ctrl+shift+n" into modifier bits and a virtual-key code.
// Parsing is case- and space-insensitive. An empty spec means "no hotkey" and
// returns ok=false with no error, so callers can treat "" as disabled.
func ParseHotkey(spec string) (Hotkey, bool, error) {
	s := strings.ToLower(strings.TrimSpace(spec))
	if s == "" {
		return Hotkey{}, false, nil
	}
	var hk Hotkey
	var key string
	for _, part := range strings.Split(s, "+") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if m, ok := modAliases[part]; ok {
			hk.Mods |= m
			continue
		}
		if key != "" {
			return Hotkey{}, false, fmt.Errorf("hotkey %q: more than one non-modifier key", spec)
		}
		key = part
	}
	if key == "" {
		return Hotkey{}, false, fmt.Errorf("hotkey %q: no key, only modifiers", spec)
	}
	if vk, ok := namedKeys[key]; ok {
		hk.VK = vk
		return hk, true, nil
	}
	if len(key) == 1 {
		c := key[0]
		switch {
		case c >= 'a' && c <= 'z':
			hk.VK = uint32(c - 'a' + 'A') // VK_A..VK_Z
			return hk, true, nil
		case c >= '0' && c <= '9':
			hk.VK = uint32(c) // VK_0..VK_9
			return hk, true, nil
		}
	}
	return Hotkey{}, false, fmt.Errorf("hotkey %q: unknown key %q", spec, key)
}
