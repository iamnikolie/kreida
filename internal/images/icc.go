package images

import (
	"encoding/binary"
	"errors"
	"slices"
)

// iccColourTypes are the ICC tag types that describe colour: curves, matrices, lookup tables, chromaticities. Every
// other type (text, descriptions, dictionaries, names, private types) is dropped from a profile that leaves the
// machine.
var iccColourTypes = map[string]bool{
	"curv": true, "para": true, "XYZ ": true, "sf32": true, "mft1": true, "mft2": true, "mAB ": true, "mBA ": true,
	"mpet": true, "chrm": true, "cicp": true,
}

const (
	iccHeader  = 128
	iccMaxTags = 100
)

// cleanICC keeps the colour of an ICC profile and none of its words. The description and the copyright become "ICC",
// tags of any type but colour data go, and the header loses the fields that name a vendor, a device, a creator, a
// date or the profile's own hash. The answer is the profile and its colour space ("RGB ", "GRAY", "CMYK", ...). A
// profile that cannot be read is refused.
func cleanICC(p []byte) ([]byte, string, error) {
	bad := errors.New("unreadable colour profile")

	if len(p) < iccHeader+4 || string(p[36:40]) != "acsp" {
		return nil, "", bad
	}

	size := int(binary.BigEndian.Uint32(p))
	if size < iccHeader+4 || size > len(p) {
		return nil, "", bad
	}

	p = p[:size]
	v4 := p[8] >= 4

	n := int(binary.BigEndian.Uint32(p[iccHeader:]))
	if n > iccMaxTags || iccHeader+4+12*n > size {
		return nil, "", bad
	}

	type tag struct {
		sig       string
		off, size int
	}

	var keep []tag

	for i := range n {
		e := p[iccHeader+4+12*i:]
		t := tag{string(e[:4]), int(binary.BigEndian.Uint32(e[4:])), int(binary.BigEndian.Uint32(e[8:]))}

		if t.off < iccHeader+4+12*n || t.size < 8 || t.off > size-t.size {
			return nil, "", bad
		}

		if t.sig != "desc" && t.sig != "cprt" && iccColourTypes[string(p[t.off:t.off+4])] {
			keep = append(keep, t)
		}
	}

	head := append([]byte{}, p[:iccHeader]...)

	for _, r := range [][2]int{{4, 8}, {24, 36}, {40, 44}, {48, 64}, {80, 128}} {
		clear(head[r[0]:r[1]])
	}

	text, desc := iccText(v4, false), iccText(v4, true)

	sigs := make([]string, 0, len(keep)+2)
	sigs = append(sigs, "desc", "cprt")

	for _, t := range keep {
		sigs = append(sigs, t.sig)
	}

	// shared data (one curve for three channels) stays shared
	table := iccHeader + 4 + 12*len(sigs)
	data := []byte{}
	where := map[[2]int]int{}
	entries := make([][3]int, 0, len(sigs))

	put := func(b []byte) int {
		for len(data)%4 != 0 {
			data = append(data, 0)
		}

		off := table + len(data)
		data = append(data, b...)

		return off
	}

	entries = append(entries, [3]int{put(desc), len(desc)}, [3]int{put(text), len(text)})

	for _, t := range keep {
		k := [2]int{t.off, t.size}

		off, ok := where[k]
		if !ok {
			off = put(p[t.off : t.off+t.size])
			where[k] = off
		}

		entries = append(entries, [3]int{off, t.size})
	}

	for len(data)%4 != 0 {
		data = append(data, 0)
	}

	out := slices.Concat(head, binary.BigEndian.AppendUint32(nil, uint32(len(sigs))))
	for i, s := range sigs {
		out = append(out, s...)
		out = binary.BigEndian.AppendUint32(out, uint32(entries[i][0]))
		out = binary.BigEndian.AppendUint32(out, uint32(entries[i][1]))
	}

	out = append(out, data...)
	binary.BigEndian.PutUint32(out, uint32(len(out)))

	return out, string(p[16:20]), nil
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
