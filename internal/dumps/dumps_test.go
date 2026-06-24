package dumps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeServer serves four tiny "dump" files plus a CHECKSUM.txt computed over them.
func fakeServer(t *testing.T, date string) *httptest.Server {
	t.Helper()
	bodies := map[string]string{}
	var lines []string
	for _, typ := range Types {
		name := FileName(date, typ)
		body := "GZIPDATA-" + typ
		bodies[name] = body
		sum := sha256.Sum256([]byte(body))
		lines = append(lines, fmt.Sprintf("%s  %s", hex.EncodeToString(sum[:]), name))
	}
	bodies[ChecksumName(date)] = strings.Join(lines, "\n") + "\n"

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dl := r.URL.Query().Get("download") // e.g. data/2026/discogs_20260601_releases.xml.gz
		name := dl[strings.LastIndex(dl, "/")+1:]
		body, ok := bodies[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	}))
}

func TestDownloadAndVerify(t *testing.T) {
	const date = "20260601"
	srv := fakeServer(t, date)
	defer srv.Close()
	dir := t.TempDir()

	if err := Download(context.Background(), srv.URL, date, dir, srv.Client()); err != nil {
		t.Fatalf("Download: %v", err)
	}
	for _, typ := range Types {
		if _, err := os.Stat(filepath.Join(dir, FileName(date, typ))); err != nil {
			t.Fatalf("missing %s: %v", typ, err)
		}
	}
	if err := VerifyChecksums(dir, date); err != nil {
		t.Fatalf("VerifyChecksums: %v", err)
	}
}

func TestVerifyChecksums_DetectsCorruption(t *testing.T) {
	const date = "20260601"
	srv := fakeServer(t, date)
	defer srv.Close()
	dir := t.TempDir()
	if err := Download(context.Background(), srv.URL, date, dir, srv.Client()); err != nil {
		t.Fatal(err)
	}
	// corrupt one file
	os.WriteFile(filepath.Join(dir, FileName(date, "labels")), []byte("tampered"), 0o644)
	if err := VerifyChecksums(dir, date); err == nil {
		t.Fatalf("expected checksum mismatch error, got nil")
	}
}

func TestVerifyChecksums_MissingDump(t *testing.T) {
	const date = "20260601"
	// Build a checksum file that only contains three of the four expected dumps.
	dir := t.TempDir()
	var lines []string
	for i, typ := range Types {
		if i == len(Types)-1 {
			break // omit the last type ("releases")
		}
		name := FileName(date, typ)
		body := "GZIPDATA-" + typ
		os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644)
		sum := sha256.Sum256([]byte(body))
		lines = append(lines, fmt.Sprintf("%s  %s", hex.EncodeToString(sum[:]), name))
	}
	checksum := strings.Join(lines, "\n") + "\n"
	os.WriteFile(filepath.Join(dir, ChecksumName(date)), []byte(checksum), 0o644)

	if err := VerifyChecksums(dir, date); err == nil {
		t.Fatalf("expected error for missing dump entry, got nil")
	}
}
