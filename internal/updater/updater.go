package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/minio/selfupdate"
)

const (
	repository = "AlexanderT02/hookbin-cli"
	maxArchive = 64 << 20
	maxBinary  = 64 << 20
)

var client = &http.Client{Timeout: 20 * time.Second}

type Release struct {
	Tag string `json:"tag_name"`
}

func (r Release) Version() string { return strings.TrimPrefix(r.Tag, "v") }

func Check(ctx context.Context, current string) (Release, bool, error) {
	return check(ctx, current, "https://api.github.com/repos/"+repository+"/releases/latest")
}

func check(ctx context.Context, current, endpoint string) (Release, bool, error) {
	if _, err := parseVersion(current); err != nil {
		return Release{}, false, fmt.Errorf("current version %q is not a release build", current)
	}
	data, err := fetch(ctx, endpoint, 1<<20)
	if err != nil {
		return Release{}, false, fmt.Errorf("check releases: %w", err)
	}
	var release Release
	if err := json.Unmarshal(data, &release); err != nil {
		return Release{}, false, fmt.Errorf("read release: %w", err)
	}
	latest, err := parseVersion(release.Version())
	if err != nil || !strings.HasPrefix(release.Tag, "v") {
		return Release{}, false, errors.New("latest release has an invalid version tag")
	}
	installed, _ := parseVersion(current)
	for i := range latest {
		if latest[i] != installed[i] {
			return release, latest[i] > installed[i], nil
		}
	}
	return release, false, nil
}

func parseVersion(value string) ([3]uint64, error) {
	var parts [3]uint64
	value = strings.TrimPrefix(value, "v")
	fields := strings.Split(value, ".")
	if len(fields) != 3 {
		return parts, errors.New("expected major.minor.patch")
	}
	for i, field := range fields {
		if field == "" || (len(field) > 1 && field[0] == '0') {
			return parts, errors.New("invalid version component")
		}
		for _, char := range field {
			if char < '0' || char > '9' {
				return parts, errors.New("invalid version component")
			}
		}
		part, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return parts, err
		}
		parts[i] = part
	}
	return parts, nil
}

func Install(ctx context.Context, release Release) error {
	if _, err := parseVersion(release.Version()); err != nil || !strings.HasPrefix(release.Tag, "v") {
		return errors.New("invalid release version")
	}
	path, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find current executable: %w", err)
	}
	return install(ctx, release, "https://github.com/"+repository+"/releases/download", path)
}

func install(ctx context.Context, release Release, baseURL, path string) error {
	extension := "tar.gz"
	binaryName := "hookbin"
	if runtime.GOOS == "windows" {
		extension = "zip"
		binaryName += ".exe"
	}
	archiveName := fmt.Sprintf("hookbin_%s_%s_%s.%s", release.Version(), runtime.GOOS, runtime.GOARCH, extension)
	baseURL += "/" + release.Tag + "/"
	checksums, err := fetch(ctx, baseURL+"SHA256SUMS", 1<<20)
	if err != nil {
		return fmt.Errorf("download checksums: %w", err)
	}
	expected, err := checksumFor(checksums, archiveName)
	if err != nil {
		return err
	}
	archive, err := fetch(ctx, baseURL+archiveName, maxArchive)
	if err != nil {
		return fmt.Errorf("download release: %w", err)
	}
	actual := sha256.Sum256(archive)
	if !bytes.Equal(actual[:], expected) {
		return errors.New("release checksum does not match SHA256SUMS")
	}
	binary, err := extractBinary(archive, binaryName)
	if err != nil {
		return err
	}
	if err := selfupdate.Apply(bytes.NewReader(binary), selfupdate.Options{TargetPath: path}); err != nil {
		if rollback := selfupdate.RollbackError(err); rollback != nil {
			return fmt.Errorf("replace executable: %w; rollback failed: %v", err, rollback)
		}
		return fmt.Errorf("replace executable: %w", err)
	}
	return nil
}

func fetch(ctx context.Context, url string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "hookbin-cli")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("download exceeds size limit")
	}
	return data, nil
}

func checksumFor(data []byte, archiveName string) ([]byte, error) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(strings.TrimPrefix(fields[1], "*"), "./") != archiveName {
			continue
		}
		checksum, err := hex.DecodeString(fields[0])
		if err != nil || len(checksum) != sha256.Size {
			return nil, errors.New("invalid release checksum")
		}
		return checksum, nil
	}
	return nil, fmt.Errorf("no checksum for %s", archiveName)
}

func extractBinary(archive []byte, name string) ([]byte, error) {
	if runtime.GOOS == "windows" {
		reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, err
		}
		for _, file := range reader.File {
			if file.Name != name || file.UncompressedSize64 > maxBinary {
				continue
			}
			stream, err := file.Open()
			if err != nil {
				return nil, err
			}
			data, readErr := io.ReadAll(io.LimitReader(stream, maxBinary+1))
			stream.Close()
			if readErr != nil || len(data) > maxBinary {
				return nil, errors.New("invalid binary in release archive")
			}
			return data, nil
		}
	} else {
		stream, err := gzip.NewReader(bytes.NewReader(archive))
		if err != nil {
			return nil, err
		}
		defer stream.Close()
		reader := tar.NewReader(stream)
		for {
			header, err := reader.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, err
			}
			if header.Name != name || !header.FileInfo().Mode().IsRegular() {
				continue
			}
			if header.Size > maxBinary {
				return nil, errors.New("binary in release archive exceeds size limit")
			}
			return io.ReadAll(io.LimitReader(reader, maxBinary+1))
		}
	}
	return nil, fmt.Errorf("release archive does not contain %s", name)
}
