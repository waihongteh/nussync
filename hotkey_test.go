package main

import "testing"

func TestParseHotkey(t *testing.T) {
	cases := []struct {
		spec string
		mods uint32
		vk   uint32
	}{
		{"ctrl+shift+n", modControl | modShift, 'N'},
		{"CTRL+SHIFT+N", modControl | modShift, 'N'},
		{" ctrl + shift + n ", modControl | modShift, 'N'},
		{"control+alt+delete", modControl | modAlt, 0x2E},
		{"win+space", modWin, 0x20},
		{"cmd+1", modWin, '1'},
		{"alt+f4", modAlt, 0x73},
		{"shift+pageup", modShift, 0x21},
		{"n", 0, 'N'}, // modifiers are optional
	}
	for _, c := range cases {
		hk, ok, err := ParseHotkey(c.spec)
		if err != nil || !ok {
			t.Errorf("ParseHotkey(%q) = (_, %v, %v), want ok", c.spec, ok, err)
			continue
		}
		if hk.Mods != c.mods || hk.VK != c.vk {
			t.Errorf("ParseHotkey(%q) = {mods %#x, vk %#x}, want {mods %#x, vk %#x}",
				c.spec, hk.Mods, hk.VK, c.mods, c.vk)
		}
	}
}

func TestParseHotkeyEmptyMeansDisabled(t *testing.T) {
	for _, spec := range []string{"", "   "} {
		hk, ok, err := ParseHotkey(spec)
		if ok || err != nil || hk != (Hotkey{}) {
			t.Errorf("ParseHotkey(%q) = (%+v, %v, %v), want disabled with no error",
				spec, hk, ok, err)
		}
	}
}

func TestParseHotkeyErrors(t *testing.T) {
	for _, spec := range []string{
		"ctrl+shift", // modifiers only
		"ctrl+n+m",   // two non-modifier keys
		"ctrl+nope",  // unknown key name
		"ctrl+f13",   // out of the supported function-key range
	} {
		if _, ok, err := ParseHotkey(spec); err == nil || ok {
			t.Errorf("ParseHotkey(%q) = (_, %v, %v), want an error", spec, ok, err)
		}
	}
}

func TestParseHotkeyModAliases(t *testing.T) {
	for _, spec := range []string{"super+n", "meta+n", "win+n", "cmd+n"} {
		hk, ok, _ := ParseHotkey(spec)
		if !ok || hk.Mods != modWin {
			t.Errorf("ParseHotkey(%q) mods = %#x, want %#x", spec, hk.Mods, modWin)
		}
	}
	for _, spec := range []string{"option+n", "alt+n"} {
		hk, ok, _ := ParseHotkey(spec)
		if !ok || hk.Mods != modAlt {
			t.Errorf("ParseHotkey(%q) mods = %#x, want %#x", spec, hk.Mods, modAlt)
		}
	}
}
