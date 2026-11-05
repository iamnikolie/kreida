package images

import (
	"encoding/binary"
	"math"
	"slices"
	"strings"
)

// No byte of a picture's own colour profile leaves the machine. A profile is read for its colour only: if its
// primaries and tone curves are those of a known standard (sRGB, Display P3, Adobe RGB), the picture goes out with
// kreida's own copy of that standard; any other profile, a grey or CMYK one, or one that cannot be read, is dropped,
// and the picture goes out as its pixels decode, which may shift its colours. Dropping carries nothing out; refusing
// would only cost the owner the picture.

const (
	iccHeader  = 128
	iccMaxTags = 100
	iccMaxCurv = 4096

	// primaryTolerance and curveTolerance are how far a profile may stand from a standard and still be it: the
	// rounding of the standards' own published profiles, with room.
	primaryTolerance = 0.003
	curveTolerance   = 0.006
)

// a tone curve: sRGB's piecewise function, or a pure gamma
type trc struct {
	srgb  bool
	gamma float64
}

func (c trc) at(x float64) float64 {
	if !c.srgb {
		return math.Pow(x, c.gamma)
	}

	if x < 0.04045 {
		return x / 12.92
	}

	return math.Pow((x+0.055)/1.055, 2.4)
}

// knownProfile is a standard kreida carries: its name, its D50-adapted primaries and its tone curve.
type knownProfile struct {
	name    string
	r, g, b [3]float64
	curve   trc
	icc     []byte
}

var knownProfiles = []*knownProfile{
	{name: "sRGB", r: [3]float64{0.4361, 0.2225, 0.0139}, g: [3]float64{0.3851, 0.7169, 0.0971}, b: [3]float64{0.1431, 0.0606, 0.7141}, curve: trc{srgb: true}},
	{name: "Display P3", r: [3]float64{0.5151, 0.2412, -0.0011}, g: [3]float64{0.2920, 0.6922, 0.0419}, b: [3]float64{0.1571, 0.0666, 0.7841}, curve: trc{srgb: true}},
	{name: "Adobe RGB (1998)", r: [3]float64{0.6097, 0.3111, 0.0195}, g: [3]float64{0.2053, 0.6257, 0.0609}, b: [3]float64{0.1492, 0.0632, 0.7446}, curve: trc{gamma: 563.0 / 256}},
}

func init() {
	for _, k := range knownProfiles {
		k.icc = buildICC(k)
	}
}

// knownICC names the standard a profile is, by its colour, and answers kreida's copy of it; nil when it is none.
func knownICC(p []byte) []byte {
	tags := readICC(p)
	if tags == nil || !pure(p, tags) {
		return nil
	}

	for _, k := range knownProfiles {
		if near(tags["rXYZ"], k.r) && near(tags["gXYZ"], k.g) && near(tags["bXYZ"], k.b) &&
			sameCurve(tags["rTRC"], k.curve) && sameCurve(tags["gTRC"], k.curve) && sameCurve(tags["bTRC"], k.curve) {
			return k.icc
		}
	}

	return nil
}

// d50 is the PCS illuminant every matrix profile is relative to.
var d50 = [3]float64{0.9642, 1, 0.8249}

// pure says whether a profile's colour is its matrix and tone curves alone, so that matching those is matching the
// profile: a display profile of version 4 or older, relative to D50, without any tag a CMM would take over the matrix
// — a table or multi-process transform (A2B0…, B2A0…, D2B0…, B2D0…) or coding-independent code points (cicp). Other
// tags (names, copyright, white point, private ones) do not change how its pixels look under a perceptual or relative
// intent.
func pure(p []byte, tags map[string][]byte) bool {
	if p[8] > 4 || string(p[12:16]) != "mntr" || !near(slices.Concat([]byte("XYZ \x00\x00\x00\x00"), p[68:80]), d50) {
		return false
	}

	for sig := range tags {
		if sig == "cicp" || sig[1] == '2' && strings.ContainsRune("ABDM", rune(sig[0])) && strings.ContainsRune("ABDM", rune(sig[2])) {
			return false
		}
	}

	return true
}

// readICC finds the matrix and tone curves of an RGB display profile. Nil when it is not one, or cannot be read.
func readICC(p []byte) map[string][]byte {
	if len(p) < iccHeader+4 || string(p[36:40]) != "acsp" || string(p[16:20]) != "RGB " || string(p[20:24]) != "XYZ " {
		return nil
	}

	size := int(binary.BigEndian.Uint32(p))
	if size < iccHeader+4 || size > len(p) {
		return nil
	}

	n := int(binary.BigEndian.Uint32(p[iccHeader:]))
	if n > iccMaxTags || iccHeader+4+12*n > size {
		return nil
	}

	tags := map[string][]byte{}

	for i := range n {
		e := p[iccHeader+4+12*i:]
		sig, off, length := string(e[:4]), int(binary.BigEndian.Uint32(e[4:])), int(binary.BigEndian.Uint32(e[8:]))

		if length < 8 || off < 0 || off > size-length {
			return nil
		}

		if _, twice := tags[sig]; twice {
			return nil
		}

		tags[sig] = p[off : off+length]
	}

	return tags
}

// near says whether an XYZ tag holds these three values.
func near(tag []byte, want [3]float64) bool {
	if len(tag) < 20 || string(tag[:4]) != "XYZ " {
		return false
	}

	for i := range 3 {
		if math.Abs(s15(tag[8+4*i:])-want[i]) > primaryTolerance {
			return false
		}
	}

	return true
}

func s15(b []byte) float64 { return float64(int32(binary.BigEndian.Uint32(b))) / 65536 }

// sameCurve says whether a curv or para tag draws the same tone curve, compared along its length.
func sameCurve(tag []byte, want trc) bool {
	f := curveOf(tag)
	if f == nil {
		return false
	}

	for i := 1; i <= 20; i++ {
		x := float64(i) / 20
		if math.Abs(f(x)-want.at(x)) > curveTolerance {
			return false
		}
	}

	return true
}

// curveOf reads a tone curve tag as a function on [0, 1]; nil when it cannot.
func curveOf(tag []byte) func(float64) float64 {
	if len(tag) < 12 {
		return nil
	}

	switch string(tag[:4]) {
	case "curv":
		n := int(binary.BigEndian.Uint32(tag[8:]))
		if n > iccMaxCurv || len(tag) < 12+2*n {
			return nil
		}

		pts := make([]float64, n)
		for i := range n {
			pts[i] = float64(binary.BigEndian.Uint16(tag[12+2*i:])) / 65535
		}

		switch n {
		case 0:
			return func(x float64) float64 { return x }
		case 1:
			g := float64(binary.BigEndian.Uint16(tag[12:])) / 256

			return func(x float64) float64 { return math.Pow(x, g) }
		}

		return func(x float64) float64 {
			at := x * float64(n-1)
			i := min(int(at), n-2)

			return pts[i] + (pts[i+1]-pts[i])*(at-float64(i))
		}
	case "para":
		k := map[uint16]int{0: 1, 1: 3, 2: 4, 3: 5, 4: 7}[binary.BigEndian.Uint16(tag[8:])]
		if k == 0 && binary.BigEndian.Uint16(tag[8:]) != 0 || len(tag) < 12+4*max(k, 1) {
			return nil
		}

		v := make([]float64, 7)
		for i := range max(k, 1) {
			v[i] = s15(tag[12+4*i:])
		}

		g, a, b, c, d, e, f := v[0], v[1], v[2], v[3], v[4], v[5], v[6]
		pow := func(x float64) float64 { return math.Pow(math.Max(x, 0), g) }

		switch binary.BigEndian.Uint16(tag[8:]) {
		case 0:
			return pow
		case 1:
			return func(x float64) float64 { return pow(a*x + b) }
		case 2:
			return func(x float64) float64 { return pow(a*x+b) + c }
		case 3:
			return func(x float64) float64 {
				if x >= d {
					return pow(a*x + b)
				}

				return c * x
			}
		case 4:
			return func(x float64) float64 {
				if x >= d {
					return pow(a*x+b) + e
				}

				return c*x + f
			}
		}
	}

	return nil
}

// buildICC writes kreida's own version 2 display profile of a standard: its name, its primaries, its tone curve and
// the D65 to D50 adaptation, nothing else.
func buildICC(k *knownProfile) []byte {
	xyz := func(v [3]float64) []byte {
		out := []byte("XYZ \x00\x00\x00\x00")
		for _, f := range v {
			out = binary.BigEndian.AppendUint32(out, uint32(int32(math.Round(f*65536))))
		}

		return out
	}

	var curve []byte
	if k.curve.srgb {
		curve = []byte("para\x00\x00\x00\x00\x00\x03\x00\x00")
		for _, f := range []float64{2.4, 1 / 1.055, 0.055 / 1.055, 1 / 12.92, 0.04045} {
			curve = binary.BigEndian.AppendUint32(curve, uint32(int32(math.Round(f*65536))))
		}
	} else {
		curve = binary.BigEndian.AppendUint16([]byte("curv\x00\x00\x00\x00\x00\x00\x00\x01"), uint16(math.Round(k.curve.gamma*256)))
	}

	chad := []byte("sf32\x00\x00\x00\x00")
	for _, f := range []float64{1.0479, 0.0229, -0.0502, 0.0296, 0.9904, -0.0171, -0.0092, 0.0151, 0.7519} {
		chad = binary.BigEndian.AppendUint32(chad, uint32(int32(math.Round(f*65536))))
	}

	desc := slices.Concat([]byte("desc\x00\x00\x00\x00"), binary.BigEndian.AppendUint32(nil, uint32(len(k.name)+1)), []byte(k.name), make([]byte, 1+4+4+2+1+67))

	tags := []struct {
		sig  string
		body []byte
	}{
		{"desc", desc}, {"cprt", []byte("text\x00\x00\x00\x00No copyright, use freely\x00")},
		{"wtpt", xyz(d50)},
		{"rXYZ", xyz(k.r)}, {"gXYZ", xyz(k.g)}, {"bXYZ", xyz(k.b)},
		{"rTRC", curve}, {"gTRC", curve}, {"bTRC", curve}, {"chad", chad},
	}

	head := make([]byte, iccHeader)
	copy(head[8:], []byte{2, 0x10})
	copy(head[12:], "mntrRGB XYZ ")
	copy(head[36:], "acsp")
	copy(head[68:], []byte{0, 0, 0xf6, 0xd6, 0, 1, 0, 0, 0, 0, 0xd3, 0x2d})

	table := iccHeader + 4 + 12*len(tags)

	var data, entries []byte

	shared := map[string]int{}

	for _, t := range tags {
		off, ok := shared[string(t.body)]
		if !ok {
			for len(data)%4 != 0 {
				data = append(data, 0)
			}

			off = table + len(data)
			shared[string(t.body)] = off
			data = append(data, t.body...)
		}

		entries = append(entries, t.sig...)
		entries = binary.BigEndian.AppendUint32(entries, uint32(off))
		entries = binary.BigEndian.AppendUint32(entries, uint32(len(t.body)))
	}

	for len(data)%4 != 0 {
		data = append(data, 0)
	}

	out := slices.Concat(head, binary.BigEndian.AppendUint32(nil, uint32(len(tags))), entries, data)
	binary.BigEndian.PutUint32(out, uint32(len(out)))

	return out
}
