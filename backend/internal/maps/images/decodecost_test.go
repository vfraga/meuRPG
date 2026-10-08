package images

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"runtime"
	"testing"
)

// png.DecodeConfig names gray for a gray PNG with a tRNS chunk, but png.Decode returns NRGBA (NRGBA64 with 16 bits), and a 16-bit gray gets to8bit's copy: the estimate must follow the decoded type, not the header's model.

// grayPNG is a w x h gradient PNG of one gray channel, with a tRNS chunk when trns.
func grayPNG(t *testing.T, w, h, depth int, trns bool) []byte {
	t.Helper()
	bpp := depth / 8
	row := make([]byte, 1+w*bpp)
	for x := range w {
		v := uint16(x * 65535 / w) //nolint:gosec // test gradient
		if depth == 16 {
			binary.BigEndian.PutUint16(row[1+2*x:], v)
		} else {
			row[1+x] = byte(v >> 8)
		}
	}
	var z bytes.Buffer
	zw, _ := zlib.NewWriterLevel(&z, zlib.BestSpeed)
	for range h {
		_, _ = zw.Write(row)
	}
	_ = zw.Close()
	ihdr := binary.BigEndian.AppendUint32(nil, uint32(w)) //nolint:gosec // test
	ihdr = binary.BigEndian.AppendUint32(ihdr, uint32(h)) //nolint:gosec // test
	ihdr = append(ihdr, byte(depth), 0, 0, 0, 0)          //nolint:gosec // test: 8 or 16
	out := []byte("\x89PNG\r\n\x1a\n")
	out = append(out, pngChunk("IHDR", ihdr)...)
	if trns {
		out = append(out, pngChunk("tRNS", []byte{0, 7})...)
	}
	out = append(out, pngChunk("IDAT", z.Bytes())...)
	out = append(out, pngChunk("IEND", nil)...)
	return out
}

func TestDecodeCostCoversAGrayPNGAsItIsDecoded(t *testing.T) {
	cases := []struct {
		name  string
		depth int
		trns  bool
	}{
		{"gray16+tRNS", 16, true},
		{"gray8+tRNS", 8, true},
		{"gray16 no tRNS", 16, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data := grayPNG(t, 8000, 5000, c.depth, c.trns)
			if len(data) > MaxBytes {
				t.Fatalf("test PNG too big: %d", len(data))
			}
			cfg, err := decodeConfig(formatPNG, data)
			if err != nil {
				t.Fatal(err)
			}
			est := decodeCost(formatPNG, cfg, jpegHeader{}, readPNGInfo(data))
			var perr error
			peak, _ := peakHeap(func() { _, perr = Process(data) })
			runtime.GC()
			t.Logf("%s: file %.1f MiB, estimate %.0f MiB, measured peak %.0f MiB, err=%v",
				c.name, float64(len(data))/(1<<20), float64(est)/(1<<20), float64(peak)/(1<<20), perr)
			if errors.Is(perr, ErrDimensions) {
				return
			}
			if perr != nil {
				t.Fatalf("unexpected error %v", perr)
			}
			if float64(peak) > float64(est)*1.15 {
				t.Errorf("peak %d exceeds estimate %d by more than 15%%", peak, est)
			}
			if peak > maxDecodeBytes {
				t.Errorf("accepted input peaked at %d > maxDecodeBytes %d", peak, int64(maxDecodeBytes))
			}
		})
	}
}
