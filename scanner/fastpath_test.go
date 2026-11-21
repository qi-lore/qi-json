package scanner

import (
	"math/rand"
	"testing"
)

// This file pins the production scalar scanners to straightforward
// byte-at-a-time reference implementations. The production string scanner adds
// a bulk word-at-a-time (SWAR) path; the differential tests below assert that
// the two forms agree on every input, so the fast path cannot silently change a
// return index, an error message or an error position. The number and
// whitespace scanners have no fast path today (both were measured to regress and
// are not shipped; see the plan), but they are pinned here all the same, so a
// future fast path cannot land without the same guarantee.

// skipSpaceRef is the plain byte-at-a-time whitespace skip.
func skipSpaceRef(data []byte, from int) int {
	for from < len(data) && spaceTable[data[from]] {
		from++
	}
	return from
}

// scanStringRef scans a string one byte at a time.
func (v *validator) scanStringRef(i int) int {
	data := v.data
	n := len(data)

	for i++; ; {
		if i >= n {
			v.failEnd(n)
			return n
		}
		switch c := data[i]; {
		case c == '"':
			return i + 1
		case c == '\\':
			i = v.scanEscapeRef(i)
			if v.failure != nil {
				return n
			}
		case c < 0x20:
			v.fail(i, "in string literal")
			return n
		default:
			i++
		}
	}
}

// scanEscapeRef scans an escape one byte at a time.
func (v *validator) scanEscapeRef(i int) int {
	data := v.data
	n := len(data)

	i++ // the backslash
	if i >= n {
		v.failAt(int64(n), "in string escape code")
		return n
	}

	switch data[i] {
	case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
		return i + 1
	case 'u':
		for k := 1; k <= 4; k++ {
			if i+k >= n {
				v.failAt(int64(n), `in \u hexadecimal character escape`)
				return n
			}
			if !isHexDigit(data[i+k]) {
				v.fail(i+k, `in \u hexadecimal character escape`)
				return n
			}
		}
		return i + 5
	default:
		v.fail(i, "in string escape code")
		return n
	}
}

// scanNumberRef scans a number one byte at a time.
func (v *validator) scanNumberRef(i int) int {
	data := v.data
	n := len(data)

	if data[i] == '-' {
		i++
		if i >= n {
			v.failAt(int64(n), "in numeric literal")
			return n
		}
	}

	if data[i] == '0' {
		i++
		if i < n && isDigit(data[i]) {
			v.fail(i, "in numeric literal")
			return n
		}
	} else if isDigit(data[i]) {
		for i++; i < n && isDigit(data[i]); i++ {
		}
	} else {
		v.fail(i, "in numeric literal")
		return n
	}

	if i < n && data[i] == '.' {
		i++
		if i >= n {
			v.failAt(int64(n), "after decimal point in numeric literal")
			return n
		}
		if !isDigit(data[i]) {
			v.fail(i, "after decimal point in numeric literal")
			return n
		}
		for i++; i < n && isDigit(data[i]); i++ {
		}
	}

	if i < n && (data[i] == 'e' || data[i] == 'E') {
		i++
		if i < n && (data[i] == '+' || data[i] == '-') {
			i++
		}
		if i >= n {
			v.failAt(int64(n), "in exponent of numeric literal")
			return n
		}
		if !isDigit(data[i]) {
			v.fail(i, "in exponent of numeric literal")
			return n
		}
		for i++; i < n && isDigit(data[i]); i++ {
		}
	}

	return i
}

// requireSameScan asserts that the production and reference scanners returned
// the same index and the same (or no) failure.
func requireSameScan(t *testing.T, label string, in []byte, got int, gf *parseError, want int, wf *parseError) {
	t.Helper()

	if got != want {
		t.Fatalf("%s(%q): index = %d, want %d", label, in, got, want)
	}
	switch {
	case gf == nil && wf == nil:
		return
	case gf == nil || wf == nil:
		t.Fatalf("%s(%q): failure = %v, want %v", label, in, gf, wf)
	}
	if gf.at != wf.at || gf.problem != wf.problem {
		t.Fatalf("%s(%q): failure = {%d %q}, want {%d %q}", label, in, gf.at, gf.problem, wf.at, wf.problem)
	}
}

// randomFragments builds n random fragments, each starting with first and
// otherwise drawn from alphabet.
func randomFragments(r *rand.Rand, n int, first byte, alphabet string) [][]byte {
	out := make([][]byte, 0, n)
	for i := 0; i < n; i++ {
		size := r.Intn(48)
		b := make([]byte, 0, size+1)
		b = append(b, first)
		for j := 0; j < size; j++ {
			b = append(b, alphabet[r.Intn(len(alphabet))])
		}
		out = append(out, b)
	}
	return out
}

func TestSkipSpaceMatchesReference(t *testing.T) {
	corpus := [][]byte{
		{}, {' '}, {' ', ' '}, {'\t', '\n', '\r'}, {'x'}, {' ', 'x'},
		[]byte("   \t\r\n   "), []byte("        "), []byte("            x"),
	}
	r := rand.New(rand.NewSource(1))
	corpus = append(corpus, randomFragments(r, 2000, ' ', " \t\r\nx")...)

	for _, in := range corpus {
		for from := 0; from <= len(in); from++ {
			got := skipSpace(in, from)
			want := skipSpaceRef(in, from)
			if got != want {
				t.Fatalf("skipSpace(%q, %d) = %d, want %d", in, from, got, want)
			}
		}
	}
}

func TestScanStringMatchesReference(t *testing.T) {
	corpus := [][]byte{
		[]byte(`""`),
		[]byte(`"abc"`),
		[]byte(`"a\"b"`),
		[]byte(`"back\\slash"`),
		[]byte(`"solidus\/"`),
		[]byte(`"\b\f\n\r\t"`),
		[]byte(`"\u0041"`),
		[]byte(`"\uD834\uDD1E"`),
		[]byte(`"\q"`),
		[]byte(`"\u"`),
		[]byte(`"\u00"`),
		[]byte(`"\u12g"`),
		[]byte(`"\u12`),
		[]byte("\"a\nb\""),
		[]byte("\"a\tb\""),
		[]byte("\"a\x00b\""),
		[]byte("\"aaaaaaaaaaaa\x00b\""),
		[]byte(`"aaaaaaaaaaaaaaaaaaaa`),
		[]byte(`"`),
		[]byte(`"a`),
		[]byte(`"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`),
		[]byte(`"aaaa\naaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`),
	}
	r := rand.New(rand.NewSource(2))
	corpus = append(corpus, randomFragments(r, 3000, '"', "a\"\\\x00\x1fnu01/xZ")...)

	for _, in := range corpus {
		gotV := &validator{data: in}
		wantV := &validator{data: in}
		got := gotV.scanString(0)
		want := wantV.scanStringRef(0)
		requireSameScan(t, "scanString", in, got, gotV.failure, want, wantV.failure)
	}
}

func TestScanNumberMatchesReference(t *testing.T) {
	corpus := [][]byte{
		[]byte(`0`),
		[]byte(`-0`),
		[]byte(`1`),
		[]byte(`12345`),
		[]byte(`-12345`),
		[]byte(`1.5`),
		[]byte(`-1.5e3`),
		[]byte(`1E+10`),
		[]byte(`01`),
		[]byte(`-`),
		[]byte(`1.`),
		[]byte(`1.e1`),
		[]byte(`1e`),
		[]byte(`1e+`),
		[]byte(`1e++1`),
		[]byte(`123456789012345678901234567890`),
		[]byte(`123456789.123456789e123456789`),
	}
	r := rand.New(rand.NewSource(3))
	digits := randomFragments(r, 1000, '1', "0123456789")
	corpus = append(corpus, digits...)
	corpus = append(corpus, randomFragments(r, 1000, '-', "0123456789.eE+-")...)
	corpus = append(corpus, randomFragments(r, 1000, '0', "0123456789.eE+-")...)

	for _, in := range corpus {
		gotV := &validator{data: in}
		wantV := &validator{data: in}
		got := gotV.scanNumber(0)
		want := wantV.scanNumberRef(0)
		requireSameScan(t, "scanNumber", in, got, gotV.failure, want, wantV.failure)
	}
}
