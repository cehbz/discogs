package dumps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Types are the four Discogs dump categories, in load order.
var Types = []string{"artists", "labels", "masters", "releases"}

// DefaultBaseURL is the public Discogs dumps front end.
const DefaultBaseURL = "https://data.discogs.com"

func FileName(date, typ string) string {
	return fmt.Sprintf("discogs_%s_%s.xml.gz", date, typ)
}

func ChecksumName(date string) string {
	return fmt.Sprintf("discogs_%s_CHECKSUM.txt", date)
}

// downloadURL builds the front-end download URL: base/?download=data%2F<year>%2F<file>
func downloadURL(baseURL, date, file string) string {
	year := date[:4]
	return fmt.Sprintf("%s/?download=data%%2F%s%%2F%s", strings.TrimRight(baseURL, "/"), year, file)
}

// Download fetches the four dump files and the CHECKSUM file into outDir.
func Download(ctx context.Context, baseURL, date, outDir string, client *http.Client) error {
	if client == nil {
		client = http.DefaultClient
	}
	files := make([]string, 0, len(Types)+1)
	for _, typ := range Types {
		files = append(files, FileName(date, typ))
	}
	files = append(files, ChecksumName(date))

	for _, file := range files {
		if err := downloadOne(ctx, client, downloadURL(baseURL, date, file), filepath.Join(outDir, file)); err != nil {
			return fmt.Errorf("download %s: %w", file, err)
		}
	}
	return nil
}

func downloadOne(ctx context.Context, client *http.Client, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// VerifyChecksums recomputes SHA-256 for each file listed in the CHECKSUM file and
// compares. Returns an error on the first mismatch or missing file.
// A non-empty line that does not split into exactly two fields is an error.
// After processing all lines, every expected dump file must have been present and verified.
func VerifyChecksums(dir, date string) error {
	data, err := os.ReadFile(filepath.Join(dir, ChecksumName(date)))
	if err != nil {
		return fmt.Errorf("read checksum file: %w", err)
	}
	verified := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return fmt.Errorf("malformed checksum line: %q", line)
		}
		want, name := fields[0], fields[1]
		got, err := sha256File(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("hash %s: %w", name, err)
		}
		if !strings.EqualFold(got, want) {
			return fmt.Errorf("checksum mismatch for %s: got %s want %s", name, got, want)
		}
		verified[name] = true
	}
	for _, typ := range Types {
		name := FileName(date, typ)
		if !verified[name] {
			return fmt.Errorf("checksum file missing entry for %s", name)
		}
	}
	return nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
