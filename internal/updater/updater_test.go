package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
)

func TestCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tag_name":"v0.2.0"}`)
	}))
	defer server.Close()
	for _, tc := range []struct {
		current string
		want    bool
	}{
		{"0.1.9", true},
		{"0.2.0", false},
		{"0.3.0", false},
	} {
		_, available, err := check(context.Background(), tc.current, server.URL)
		if err != nil || available != tc.want {
			t.Errorf("check(%q) = %v, %v; want %v", tc.current, available, err, tc.want)
		}
	}
	if _, _, err := check(context.Background(), "dev", server.URL); err == nil {
		t.Fatal("development build should not check for updates")
	}
}

func TestChecksumForExactArchive(t *testing.T) {
	checksum := sha256.Sum256([]byte("archive"))
	content := fmt.Sprintf("%x  hookbin_0.2.0_linux_amd64.tar.gz\n", checksum)
	got, err := checksumFor([]byte(content), "hookbin_0.2.0_linux_amd64.tar.gz")
	if err != nil || hex.EncodeToString(got) != hex.EncodeToString(checksum[:]) {
		t.Fatalf("checksumFor() = %x, %v", got, err)
	}
	if _, err := checksumFor([]byte(content), "hookbin_0.2.0_linux_arm64.tar.gz"); err == nil {
		t.Fatal("checksum from another archive was accepted")
	}
}

func TestExtractBinary(t *testing.T) {
	archive := testArchive(t, "binary")
	name := "hookbin"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	got, err := extractBinary(archive, name)
	if err != nil || string(got) != "binary" {
		t.Fatalf("extractBinary() = %q, %v", got, err)
	}
}

func TestInstallVerifiesBeforeReplacing(t *testing.T) {
	archive := testArchive(t, "new binary")
	checksum := sha256.Sum256(archive)
	extension := "tar.gz"
	filename := "hookbin"
	if runtime.GOOS == "windows" {
		extension = "zip"
		filename += ".exe"
	}
	archiveName := fmt.Sprintf("hookbin_0.2.0_%s_%s.%s", runtime.GOOS, runtime.GOARCH, extension)
	var validChecksum atomic.Bool
	validChecksum.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v0.2.0/SHA256SUMS" {
			if validChecksum.Load() {
				fmt.Fprintf(w, "%x  %s\n", checksum, archiveName)
			} else {
				fmt.Fprintf(w, "%064x  %s\n", 0, archiveName)
			}
			return
		}
		if r.URL.Path == "/v0.2.0/"+archiveName {
			w.Write(archive)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	target := filepath.Join(t.TempDir(), filename)
	if err := os.WriteFile(target, []byte("old binary"), 0755); err != nil {
		t.Fatal(err)
	}
	release := Release{Tag: "v0.2.0"}
	validChecksum.Store(false)
	if err := install(context.Background(), release, server.URL, target); err == nil {
		t.Fatal("invalid checksum was accepted")
	}
	old, err := os.ReadFile(target)
	if err != nil || string(old) != "old binary" {
		t.Fatalf("invalid update changed executable: %q, %v", old, err)
	}
	validChecksum.Store(true)
	if err := install(context.Background(), release, server.URL, target); err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(target)
	if err != nil || string(updated) != "new binary" {
		t.Fatalf("updated executable = %q, %v", updated, err)
	}
}

func testArchive(t *testing.T, binary string) []byte {
	t.Helper()
	var archive bytes.Buffer
	if runtime.GOOS == "windows" {
		writer := zip.NewWriter(&archive)
		file, err := writer.Create("hookbin.exe")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(binary)); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		return archive.Bytes()
	}
	gzipWriter := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: "hookbin", Mode: 0755, Size: int64(len(binary))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write([]byte(binary)); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}
