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

func TestAmbiguousEscapeScanner(t *testing.T) {
	ok := []string{
		`"plain"`, `"😀"`, `"😀"`, `"𐀀"`, `"􏿿"`, `"􏰀"`, `"𐏿"`,
		`"퟿"`, `""`, `"\u0001"`, `"ÿ"`, `"\u12G4"`, `"\u12"`, `"\\ud800"`, `"\\\\ud800"`, `"a\"b"`, `"\"😀\""`,
		`"x" "😀" "y"`, `\ud800`, `{"a":"😀","b":"é"}`, `"\n\t\r\b\f\/\\"`, `"\u"`, `"\`, `"😀😀"`,
	}
	for _, s := range ok {
		if err := rejectAmbiguousEscapes([]byte(s), "t"); err != nil {
			t.Errorf("%s: %v", s, err)
		}
	}
	bad := []string{
		`"\ud800"`, `"\udbff"`, `"\udc00"`, `"\udfff"`, `"\u0000"`, `"\u0000A"`, `"a\u0000"`,
		`"\ud800A"`, `"\ud800\ud800"`, `"\ud800x"`, `"\ud800\`, `"\ud800\u`, `"\ud800\ud`, `"\ud800\udc0`, `"\ud800\udc0"`,
		`"\ud83d\ude0"`, `"\ud800\udbff"`, `"\ud800"`, `"\ud800\uDBFF"`, `"\\\ud800"`, `"\ud800😀"`, `"\ude00\ud83d"`,
		`"a\\\u0000"`, `"\"\ud800"`, `{"k\ud800":1}`, `["\udc00"]`,
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
