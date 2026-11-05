package images

import (
	"bytes"
	"encoding/binary"
	"errors"
	"slices"
)

// errProfile is a colour profile that cannot go out clean: unreadable, or not one kreida can rebuild.
var errProfile = errors.New("its colour profile is not one kreida can carry; save the picture as sRGB")

const (
	iccHeader  = 128
	iccMaxTags = 100
	iccMaxCurv = 4096
)

// iccOrder is every tag a profile that leaves the machine may have after desc and cprt, in the order it is written,
// with the types it may have. A matrix/TRC profile is all a display needs; lookup tables, names, text, dictionaries and
// private tags are not carried.
var iccOrder = []struct {
	sig   string
	types []string
}{
	{"wtpt", []string{"XYZ "}}, {"bkpt", []string{"XYZ "}},
	{"rXYZ", []string{"XYZ "}}, {"gXYZ", []string{"XYZ "}}, {"bXYZ", []string{"XYZ "}},
	{"rTRC", []string{"curv", "para"}}, {"gTRC", []string{"curv", "para"}}, {"bTRC", []string{"curv", "para"}},
	{"kTRC", []string{"curv", "para"}},
	{"chad", []string{"sf32"}}, {"cicp", []string{"cicp"}},
}

// paraParams is how many parameters each parametric curve function has.
var paraParams = map[uint16]int{0: 1, 1: 3, 2: 4, 3: 5, 4: 7}

// cleanICC rebuilds an ICC profile from what it says about colour, and nothing else. Every tag kept is parsed and
// written again in its canonical form: reserved bytes zero, nothing after its numbers. The description and copyright
// say "ICC"; the header keeps only the version, class, colour space, rendering intent and the standard D50 illuminant.
// It carries matrix/TRC profiles (RGB) and TRC profiles (grey); anything else, and anything it cannot read, is
// refused. It answers the profile and its colour space ("RGB " or "GRAY").
func cleanICC(p []byte) ([]byte, string, error) {
	if len(p) < iccHeader+4 || string(p[36:40]) != "acsp" || string(p[20:24]) != "XYZ " {
		return nil, "", errProfile
	}

	size := int(binary.BigEndian.Uint32(p))
	if size < iccHeader+4 || size > len(p) {
		return nil, "", errProfile
	}

	p = p[:size]

	space := string(p[16:20])
	if space != "RGB " && space != "GRAY" {
		return nil, "", errProfile
	}

	if class := string(p[12:16]); class != "mntr" && class != "scnr" && class != "spac" {
		return nil, "", errProfile
	}

	n := int(binary.BigEndian.Uint32(p[iccHeader:]))
	if n > iccMaxTags || iccHeader+4+12*n > size {
		return nil, "", errProfile
	}

	bodies := map[string][]byte{}
	seen := map[string]bool{}

	for i := range n {
		e := p[iccHeader+4+12*i:]
		sig, off, length := string(e[:4]), int(binary.BigEndian.Uint32(e[4:])), int(binary.BigEndian.Uint32(e[8:]))

		if seen[sig] || off < iccHeader+4+12*n || length < 8 || off > size-length {
			return nil, "", errProfile
		}

		seen[sig] = true

		at := slices.IndexFunc(iccOrder, func(t struct {
			sig   string
			types []string
		}) bool {
			return t.sig == sig
		})
		if at < 0 {
			continue
		}

		data := p[off : off+length]
		if !slices.Contains(iccOrder[at].types, string(data[:4])) {
			return nil, "", errProfile
		}

		body, err := iccBody(data)
		if err != nil {
			return nil, "", err
		}

		bodies[sig] = body
	}

	need := []string{"wtpt", "kTRC"}
	if space == "RGB " {
		need = []string{"wtpt", "rXYZ", "gXYZ", "bXYZ", "rTRC", "gTRC", "bTRC"}
	}

	for _, sig := range need {
		if bodies[sig] == nil {
			return nil, "", errProfile
		}
	}

	v4 := p[8] >= 4

	sigs := []string{"desc", "cprt"}
	bodies["desc"], bodies["cprt"] = iccText(v4, true), iccText(v4, false)

	for _, t := range iccOrder {
		if bodies[t.sig] != nil {
			sigs = append(sigs, t.sig)
		}
	}

	head := make([]byte, iccHeader)
	copy(head[8:10], p[8:10])
	copy(head[12:24], p[12:24])
	copy(head[36:40], "acsp")

	if intent := binary.BigEndian.Uint32(p[64:]); intent <= 3 {
		binary.BigEndian.PutUint32(head[64:], intent)
	}

	copy(head[68:80], []byte{0, 0, 0xf6, 0xd6, 0, 1, 0, 0, 0, 0, 0xd3, 0x2d})

	// the same body twice (one curve for three channels) is written once
	table := iccHeader + 4 + 12*len(sigs)
	var data []byte

	entries := make([]byte, 0, 12*len(sigs))

	for _, sig := range sigs {
		body := bodies[sig]

		off := bytes.Index(data, body)
		if off < 0 || off%4 != 0 {
			for len(data)%4 != 0 {
				data = append(data, 0)
			}

			off = len(data)
			data = append(data, body...)
		}

		entries = append(entries, sig...)
		entries = binary.BigEndian.AppendUint32(entries, uint32(table+off))
		entries = binary.BigEndian.AppendUint32(entries, uint32(len(body)))
	}

	for len(data)%4 != 0 {
		data = append(data, 0)
	}

	out := slices.Concat(head, binary.BigEndian.AppendUint32(nil, uint32(len(sigs))), entries, data)
	binary.BigEndian.PutUint32(out, uint32(len(out)))

	return out, space, nil
}

// iccBody writes a colour tag again from its numbers: its type, four zero bytes, then exactly what the type holds.
func iccBody(d []byte) ([]byte, error) {
	typ := string(d[:4])
	out := append([]byte(typ), 0, 0, 0, 0)

	switch typ {
	case "XYZ ":
		if len(d) < 20 {
			return nil, errProfile
		}

		return append(out, d[8:20]...), nil
	case "sf32":
		if len(d) < 44 {
			return nil, errProfile
		}

		return append(out, d[8:44]...), nil
	case "cicp":
		if len(d) < 12 {
			return nil, errProfile
		}

		return append(out, d[8:12]...), nil
	case "curv":
		if len(d) < 12 {
			return nil, errProfile
		}

		n := int(binary.BigEndian.Uint32(d[8:]))
		if n > iccMaxCurv || len(d) < 12+2*n {
			return nil, errProfile
		}

		return slices.Concat(out, d[8:12+2*n]), nil
	case "para":
		if len(d) < 12 {
			return nil, errProfile
		}

		fn := binary.BigEndian.Uint16(d[8:])

		k, ok := paraParams[fn]
		if !ok || len(d) < 12+4*k {
			return nil, errProfile
		}

		return slices.Concat(out, d[8:10], []byte{0, 0}, d[12:12+4*k]), nil
	}

	return nil, errProfile
}

// iccText is the text "ICC" as a profile of that version writes a description (desc) or a copyright.
func iccText(v4, desc bool) []byte {
	if v4 {
		// multiLocalizedUnicode: one record, en-US, "ICC" in UTF-16BE
		return []byte("mluc\x00\x00\x00\x00\x00\x00\x00\x01\x00\x00\x00\x0cenUS\x00\x00\x00\x06\x00\x00\x00\x1c\x00I\x00C\x00C")
	}

	if desc {
		// textDescription: ASCII "ICC", no Unicode, no ScriptCode
		return append([]byte("desc\x00\x00\x00\x00\x00\x00\x00\x04ICC\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"), make([]byte, 67)...)
	}

	return []byte("text\x00\x00\x00\x00ICC\x00")
}
