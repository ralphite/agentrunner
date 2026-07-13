package main

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadBodyRejectsTrailingAndOversizedJSON(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		code int
	}{
		{"second value", `{"name":"one"}{"name":"two"}`, http.StatusBadRequest},
		{"trailing garbage", `{"name":"one"} nope`, http.StatusBadRequest},
		{"oversized", `{"name":"` + strings.Repeat("x", maxJSONBodyBytes) + `"}`, http.StatusRequestEntityTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			var dst map[string]any
			if readBody(rec, req, &dst) {
				t.Fatal("malformed/oversized body was accepted")
			}
			if rec.Code != tc.code {
				t.Fatalf("status = %d body=%s, want %d", rec.Code, rec.Body.String(), tc.code)
			}
		})
	}
}

func uploadRequest(t *testing.T, content []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "proof.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/upload", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestHandleUploadRejectsOversizeWithoutTruncatedFile(t *testing.T) {
	runtimeDir := t.TempDir()
	uploads := filepath.Join(runtimeDir, "uploads")
	if err := os.MkdirAll(uploads, 0o700); err != nil {
		t.Fatal(err)
	}
	s := &server{runtimeDir: runtimeDir}
	rec := httptest.NewRecorder()
	s.handleUpload(rec, uploadRequest(t, bytes.Repeat([]byte("x"), maxUploadBytes+1)))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d body=%s, want 413", rec.Code, rec.Body.String())
	}
	entries, err := os.ReadDir(uploads)
	if err != nil || len(entries) != 0 {
		t.Fatalf("oversized upload left files = %v, err = %v", entries, err)
	}
}

func TestHandleUploadPreservesSmallFile(t *testing.T) {
	runtimeDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(runtimeDir, "uploads"), 0o700); err != nil {
		t.Fatal(err)
	}
	s := &server{runtimeDir: runtimeDir}
	rec := httptest.NewRecorder()
	want := []byte("small upload")
	s.handleUpload(rec, uploadRequest(t, want))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(response.Path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("uploaded content = %q, err = %v", got, err)
	}
}
