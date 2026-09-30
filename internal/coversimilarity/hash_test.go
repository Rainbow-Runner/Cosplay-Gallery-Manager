package coversimilarity

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestSignatureStableAndAspectGate(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 160, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 160; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x ^ y), A: 255})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	a, err := Compute(bytes.NewReader(data.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Compute(bytes.NewReader(data.Bytes()))
	if err != nil || a != b || DHashDistance(a, b) != 0 || PHashDistance(a, b) != 0 {
		t.Fatalf("unstable signatures: a=%+v b=%+v err=%v", a, b, err)
	}
	if !SimilarAspect(a, Signature{Width: 480, Height: 360}) || SimilarAspect(a, Signature{Width: 360, Height: 480}) {
		t.Fatal("aspect ratio guard failed")
	}
	for _, value := range []uint64{0, 1, ^uint64(0), 0x123456789abcdef0} {
		decoded, err := ParseHex(Hex(value))
		if err != nil || decoded != value {
			t.Fatalf("hex round trip %x -> %x: %v", value, decoded, err)
		}
	}
}

func TestSelectRequiresAbsoluteMatchNotRunnerUpGap(t *testing.T) {
	old := Signature{Width: 480, Height: 320}
	selected, qualified := Select(old, []Candidate{
		{ID: "b", Signature: Signature{Width: 480, Height: 320, PHash: 1, DHashH: 2}},
		{ID: "a", Signature: Signature{Width: 480, Height: 320, PHash: 1, DHashH: 1}},
		{ID: "other", Signature: Signature{Width: 480, Height: 320, PHash: ^uint64(0)}},
	})
	if selected != "a" || qualified != 2 {
		t.Fatalf("selected=%s qualified=%d", selected, qualified)
	}
	selected, qualified = Select(old, []Candidate{{ID: "false-positive", Signature: Signature{
		Width: 480, Height: 320, PHash: 0xff}}, {ID: "wrong-aspect", Signature: Signature{
		Width: 320, Height: 480}}})
	if selected != "" || qualified != 0 {
		t.Fatalf("false match selected=%s qualified=%d", selected, qualified)
	}
}

func TestSelectDHashFirstFiveAndStableCutoff(t *testing.T) {
	old := Signature{Width: 480, Height: 320}
	var candidates []Candidate
	for _, id := range []string{"f", "e", "d", "c", "b"} {
		candidates = append(candidates, Candidate{ID: id, Signature: Signature{Width: 480, Height: 320, PHash: 1}})
	}
	// This perfect pHash match is beyond the dHash first-five boundary.
	candidates = append(candidates, Candidate{ID: "z", Signature: Signature{Width: 480, Height: 320, DHashH: ^uint64(0)}})
	selected, qualified := Select(old, candidates)
	if selected != "b" || qualified != 5 {
		t.Fatalf("selected=%s qualified=%d", selected, qualified)
	}
}
