package utils

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ico "github.com/biessek/golang-ico"
)

func TestOpenConfigFile_ReadsConfigFromWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.jsonc"), []byte(`{"groups": []}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	content, err := OpenConfigFile("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(content) != `{"groups": []}` {
		t.Fatalf("unexpected content: %q", content)
	}
}

func TestOpenConfigFile_PrefersConfigJsonOverJsonc(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`json`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.jsonc"), []byte(`jsonc`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	content, err := OpenConfigFile("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(content) != "json" {
		t.Fatalf("expected config.json to win, got %q", content)
	}
}

func TestOpenConfigFile_ErrorsWhenNoConfigExists(t *testing.T) {
	t.Chdir(t.TempDir())

	if _, err := OpenConfigFile(""); err == nil {
		t.Fatal("expected an error when no configuration file exists")
	}
}

func TestOpenConfigFile_ExplicitPathReadsThatFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom.jsonc")
	if err := os.WriteFile(path, []byte(`{"explicit": true}`), 0o600); err != nil {
		t.Fatal(err)
	}

	content, err := OpenConfigFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(content) != `{"explicit": true}` {
		t.Fatalf("unexpected content: %q", content)
	}
}

func TestOpenConfigFile_ExplicitMissingPathErrorsWithPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.jsonc")

	_, err := OpenConfigFile(path)
	if err == nil {
		t.Fatal("expected an error for a missing explicit path")
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("error %q should mention the path %q", err, path)
	}
}

func TestOpenConfigFile_ExplicitMissingPathDoesNotFallBackToDiscovery(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.jsonc"), []byte(`{"groups": []}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	if _, err := OpenConfigFile(filepath.Join(dir, "missing.jsonc")); err == nil {
		t.Fatal("expected an error: an explicit path must not fall back to config.jsonc in the working directory")
	}
}

func TestGetFileSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.bin")
	if err := os.WriteFile(path, []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}

	size, err := GetFileSize(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if size != 5 {
		t.Fatalf("expected size 5, got %d", size)
	}

	if _, err := GetFileSize(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

func TestConvertToBytes(t *testing.T) {
	tests := []struct {
		in   string
		want int64
	}{
		{"2KB", 2 * 1024},
		{"5MB", 5 * 1024 * 1024},
		{"3GB", 3 * 1024 * 1024 * 1024},
		{"1GB", 1 << 30},
		{"512", 512},
		{"100B", 100},
		{"0KB", 0},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := ConvertToBytes(tc.in); got != tc.want {
				t.Fatalf("ConvertToBytes(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestConvertFromBytes(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{0, "0B"},
		{500, "500B"},
		{1023, "1023B"},
		{1024, "1.0KB"},
		{1536, "1.5KB"},
		{1024 * 1024, "1.0MB"},
		{1024 * 1024 * 1024, "1.0GB"},
		{5 * 1024 * 1024 * 1024 / 2, "2.5GB"},
	}

	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			if got := ConvertFromBytes(tc.in); got != tc.want {
				t.Fatalf("ConvertFromBytes(%d) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestExtractExtension(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"https://example.com/a/b/avatar.png", "png"},
		{"https://example.com/a/b/avatar.ico?size=64", "ico"},
		{"https://example.com/a/b/avatar.jpg#frag", "jpg"},
		{"https://example.com/a/b/avatar", ""},
		{"/local/path/file.JPG", "JPG"},
		{"http://[::1", ""},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := ExtractExtension(tc.in); got != tc.want {
				t.Fatalf("ExtractExtension(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestConvertICOtoPNG_ProducesDecodablePNGWithSamePixels(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	red := color.RGBA{R: 255, G: 0, B: 0, A: 255}
	for x := 0; x < 2; x++ {
		for y := 0; y < 2; y++ {
			src.Set(x, y, red)
		}
	}

	icoBuf := &bytes.Buffer{}
	if err := ico.Encode(icoBuf, src); err != nil {
		t.Fatalf("could not build ICO fixture: %v", err)
	}

	pngBuf, err := ConvertICOtoPNG(icoBuf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(pngBuf.Bytes()))
	if err != nil {
		t.Fatalf("result is not a valid PNG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 2 || b.Dy() != 2 {
		t.Fatalf("expected a 2x2 image, got %v", b)
	}
	if got := color.RGBAModel.Convert(img.At(1, 1)); got != red {
		t.Fatalf("expected pixel to be red, got %v", got)
	}
}

func TestConvertICOtoPNG_RejectsInvalidData(t *testing.T) {
	if _, err := ConvertICOtoPNG(bytes.NewBufferString("not an icon")); err == nil {
		t.Fatal("expected an error for invalid ICO data")
	}
}
