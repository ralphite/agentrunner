package store

import (
	"bytes"
	"strings"
	"testing"
)

func TestArtifactPutGetRoundTrip(t *testing.T) {
	a, err := OpenArtifactStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("the report body")
	ref, err := a.Put(content)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ref, "sha256-") {
		t.Fatalf("ref = %q", ref)
	}
	// CAS: identical content → identical ref, no error.
	ref2, err := a.Put(content)
	if err != nil || ref2 != ref {
		t.Fatalf("re-put: %q, %v", ref2, err)
	}
	got, err := a.Get(ref)
	if err != nil || !bytes.Equal(got, content) {
		t.Fatalf("get = %q, %v", got, err)
	}
	if _, err := a.Get("sha256-nope"); err == nil {
		t.Error("missing ref must error")
	}
	if _, err := a.Get("../escape"); err == nil {
		t.Error("path-escaping ref must be rejected")
	}
}

func TestArtifactPublishVersionChain(t *testing.T) {
	a, err := OpenArtifactStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	v1, err := a.Publish("report", []byte("draft"))
	if err != nil {
		t.Fatal(err)
	}
	v2, err := a.Publish("report", []byte("final"))
	if err != nil {
		t.Fatal(err)
	}
	other, err := a.Publish("notes", []byte("aside"))
	if err != nil {
		t.Fatal(err)
	}
	if v1.Version != 1 || v2.Version != 2 || other.Version != 1 {
		t.Fatalf("versions = %d, %d, %d", v1.Version, v2.Version, other.Version)
	}

	latest, ok, err := a.Latest("report")
	if err != nil || !ok || latest.Version != 2 || latest.Ref != v2.Ref {
		t.Fatalf("latest = %+v, %v, %v", latest, ok, err)
	}
	if _, ok, _ := a.Latest("void"); ok {
		t.Error("empty stream must report !ok")
	}

	streams, err := a.Streams()
	if err != nil || len(streams) != 2 || len(streams["report"]) != 2 {
		t.Fatalf("streams = %+v, %v", streams, err)
	}

	// Re-open: the manifest is durable.
	b, err := OpenArtifactStore(a.root)
	if err != nil {
		t.Fatal(err)
	}
	latest, ok, err = b.Latest("report")
	if err != nil || !ok || latest.Version != 2 {
		t.Fatalf("after reopen: %+v, %v, %v", latest, ok, err)
	}
	got, err := b.Get(latest.Ref)
	if err != nil || string(got) != "final" {
		t.Fatalf("content after reopen = %q, %v", got, err)
	}
}

func TestArtifactEmptyStreamRejected(t *testing.T) {
	a, _ := OpenArtifactStore(t.TempDir())
	if _, err := a.Publish("", []byte("x")); err == nil {
		t.Error("empty stream must be rejected")
	}
}
