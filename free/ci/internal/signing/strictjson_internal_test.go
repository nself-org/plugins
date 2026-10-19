package signing

import "testing"

func TestFoldKeyJoinsTheSimpleFoldOrbit(t *testing.T) {
	for _, pair := range [][2]string{{"subject", "Subject"}, {"subject", "ſubject"}, {"k", "K"}, {"k", "K"}, {"_type", "_TYPE"}, {"ß", "ẞ"}, {"σ", "ς"}} {
		if foldKey(pair[0]) != foldKey(pair[1]) {
			t.Errorf("%q and %q fold apart", pair[0], pair[1])
		}
	}
	for _, pair := range [][2]string{{"subject", "subjects"}, {"a", "b"}, {"name", "nam"}, {"x1", "x2"}} {
		if foldKey(pair[0]) == foldKey(pair[1]) {
			t.Errorf("%q and %q fold together", pair[0], pair[1])
		}
	}
	if fold('a') != fold('A') || fold('a') != 'A' || fold('1') != '1' {
		t.Errorf("fold: %q %q %q", fold('a'), fold('A'), fold('1'))
	}
}

// esc is the JSON escape of one UTF-16 unit, spelled without a literal \u sequence in the source.
func esc(hex ...string) string {
	out := ""
	for _, h := range hex {
		out += bs + "u" + h
	}
	return out
}

const bs = "\\"

func TestAmbiguousEscapeScanner(t *testing.T) {
	q := func(s string) string { return `"` + s + `"` }
	ok := []string{
		q("plain"), q(esc("d83d", "de00")), q(esc("D83D", "DE00")), q(esc("d800", "dc00")), q(esc("dbff", "dfff")), q(esc("dbff", "dc00")), q(esc("d800", "dfff")),
		q(esc("d7ff")), q(esc("e000")), q(esc("0001")), q(esc("00ff")), q(bs + "u12G4"), q(bs + "u12"), q(bs + bs + "ud800"), q(bs + bs + bs + bs + "ud800"),
		q("a" + bs + `"b`), q(bs + `"` + esc("d83d", "de00") + bs + `"`), q("x") + " " + q(esc("d83d", "de00")) + " " + q("y"), esc("d800"),
		`{"a":` + q(esc("d83d", "de00")) + `,"b":` + q(esc("00e9")) + `}`, q(bs + "n" + bs + "t" + bs + "r" + bs + "b" + bs + "f" + bs + "/" + bs + bs),
		q(bs + "u"), `"` + bs, q(esc("d83d", "de00", "d83d", "de00")),
		`"` + esc("d800", "dc00"), `"` + esc("d83d", "de00"), esc("d800", "dc00"),
	}
	for _, s := range ok {
		if err := rejectAmbiguousEscapes([]byte(s), "t"); err != nil {
			t.Errorf("%s: %v", s, err)
		}
	}
	bad := []string{
		q(esc("d800")), q(esc("dbff")), q(esc("dc00")), q(esc("dfff")), q(esc("0000")), q(esc("0000") + "A"), q("a" + esc("0000")),
		q(esc("d800") + "A"), q(esc("d800", "d800")), q(esc("d800") + "x"), `"` + esc("d800") + bs, `"` + esc("d800") + bs + "u", `"` + esc("d800") + bs + "ud",
		`"` + esc("d800") + bs + "udc0", q(esc("d800") + bs + "udc0"), q(esc("d83d") + bs + "ude0"), q(esc("d800", "dbff")), q(esc("d800", "e000")), q(esc("d800", "DBFF")),
		q(bs + bs + esc("d800")), q(esc("d800", "d83d", "de00")), q(esc("de00", "d83d")), q("a" + bs + bs + esc("0000")), q(bs + `"` + esc("d800")),
		`{"k` + esc("d800") + `":1}`, `[` + q(esc("dc00")) + `]`,
	}
	for _, s := range bad {
		if err := rejectAmbiguousEscapes([]byte(s), "t"); !IsReason(err, ReasonPayloadRejected) {
			t.Errorf("%s: %v", s, err)
		}
	}
}

func TestHexDigit(t *testing.T) {
	for c := 0; c < 256; c++ {
		var want uint16
		ok := false
		switch {
		case c >= '0' && c <= '9':
			want, ok = uint16(c-'0'), true
		case c >= 'a' && c <= 'f':
			want, ok = uint16(c-'a')+10, true
		case c >= 'A' && c <= 'F':
			want, ok = uint16(c-'A')+10, true
		}
		if got, gok := hexDigit(byte(c)); gok != ok || got != want {
			t.Errorf("%q: %d %v, want %d %v", rune(c), got, gok, want, ok)
		}
	}
}

func TestHex4(t *testing.T) {
	for in, want := range map[string]uint16{"0000": 0, "ffff": 0xffff, "FFFF": 0xffff, "d800": 0xd800, "09aF": 0x09af, "1234": 0x1234} {
		if got, ok := hex4([]byte(in), 0); !ok || got != want {
			t.Errorf("%s: %x %v", in, got, ok)
		}
	}
	for _, in := range []string{"", "123", "12g4", "12 4", "-123", "gggg", "000G"} {
		if _, ok := hex4([]byte(in), 0); ok {
			t.Errorf("%q parsed", in)
		}
	}
	if v, ok := hex4([]byte("xx00ff"), 2); !ok || v != 0xff {
		t.Errorf("offset: %x %v", v, ok)
	}
	if _, ok := hex4([]byte("00ff"), 1); ok {
		t.Error("a window that runs past the end parsed")
	}
}
