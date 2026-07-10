package tool

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pngBytes carries the real PNG signature so http.DetectContentType sniffs
// image/png; the tail is arbitrary.
var pngBytes = append([]byte("\x89PNG\r\n\x1a\n"), []byte("fake-image-data")...)

var pdfBytes = []byte("%PDF-1.4 fake pdf body")

// fakeBlobs is a BlobStore that remembers what it stored.
type fakeBlobs struct {
	data map[string][]byte
	n    int
}

func (f *fakeBlobs) Put(data []byte) (string, error) {
	if f.data == nil {
		f.data = map[string][]byte{}
	}
	f.n++
	ref := fmt.Sprintf("blob-%d", f.n)
	f.data[ref] = append([]byte(nil), data...)
	return ref, nil
}

// read_file on an image (INC-33): the result is a media envelope carrying the
// CAS ref — never the bytes — and the blob holds the exact content.
func TestReadFileImage(t *testing.T) {
	e, root := newExec(t)
	blobs := &fakeBlobs{}
	e.SetBlobs(blobs)
	if err := os.WriteFile(filepath.Join(root, "shot.png"), pngBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	m, isErr := run(t, e, "read_file", `{"path":"shot.png"}`)
	if isErr {
		t.Fatalf("image read errored: %v", m)
	}
	if m["kind"] != "image" || m["media_type"] != "image/png" {
		t.Fatalf("envelope = %v", m)
	}
	ref, _ := m["ref"].(string)
	if ref == "" {
		t.Fatal("envelope missing ref")
	}
	if !bytes.Equal(blobs.data[ref], pngBytes) {
		t.Fatal("blob does not hold the image bytes")
	}
	// The payload must carry NO raw bytes (journal stays byte-free).
	if raw := fmt.Sprintf("%v", m); strings.Contains(raw, "fake-image-data") {
		t.Error("envelope leaked raw image bytes")
	}
}

// read_file on a PDF: kind=file with application/pdf.
func TestReadFilePDF(t *testing.T) {
	e, root := newExec(t)
	e.SetBlobs(&fakeBlobs{})
	if err := os.WriteFile(filepath.Join(root, "doc.pdf"), pdfBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	m, isErr := run(t, e, "read_file", `{"path":"doc.pdf"}`)
	if isErr {
		t.Fatalf("pdf read errored: %v", m)
	}
	if m["kind"] != "file" || m["media_type"] != "application/pdf" || m["ref"] == "" {
		t.Fatalf("envelope = %v", m)
	}
}

// The default text path is byte-identical to before: plain files still return
// {content, truncated} with no envelope keys.
func TestReadFileTextUnchanged(t *testing.T) {
	e, root := newExec(t)
	e.SetBlobs(&fakeBlobs{})
	mkfile(t, root, "a.txt", "hello world\n")
	m, isErr := run(t, e, "read_file", `{"path":"a.txt"}`)
	if isErr {
		t.Fatalf("text read errored: %v", m)
	}
	if m["content"] != "hello world\n" {
		t.Fatalf("content = %v", m["content"])
	}
	if _, has := m["ref"]; has {
		t.Error("text read grew a media envelope")
	}
}

// A bare executor (no blob store injected) refuses media reads explicitly —
// never silently, never with garbage text.
func TestReadFileMediaNoBlobStore(t *testing.T) {
	e, root := newExec(t)
	if err := os.WriteFile(filepath.Join(root, "shot.png"), pngBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	m, isErr := run(t, e, "read_file", `{"path":"shot.png"}`)
	if !isErr {
		t.Fatalf("media read without a blob store should be a model-visible error, got %v", m)
	}
}

// Oversize media is refused with the cap named.
func TestReadFileOversizeMedia(t *testing.T) {
	e, root := newExec(t)
	e.SetBlobs(&fakeBlobs{})
	big := append(append([]byte(nil), pngBytes...), bytes.Repeat([]byte("x"), mediaReadMaxBytes)...)
	if err := os.WriteFile(filepath.Join(root, "huge.png"), big, 0o644); err != nil {
		t.Fatal(err)
	}
	m, isErr := run(t, e, "read_file", `{"path":"huge.png"}`)
	if !isErr {
		t.Fatalf("oversize media should error, got %v", m)
	}
}
