package utils

import (
	"bytes"
	"fmt"
	"image/png"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"

	ico "github.com/biessek/golang-ico"
)

// OpenConfigFile reads the configuration file.
//
// When path is non-empty, exactly that file is read and any failure is reported
// for that path; there is no fallback. When path is empty, the well-known file
// names are tried in order in the current working directory.
func OpenConfigFile(path string) ([]byte, error) {
	if path != "" {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read configuration file %q: %w", path, err)
		}
		return content, nil
	}

	filenames := []string{"config.json", "configuration.json", "config.jsonc", "configuration.jsonc"}
	for _, filename := range filenames {
		content, err := os.ReadFile(filename)
		if err == nil {
			return content, nil
		}
	}
	return nil, fmt.Errorf("none of the configuration files were found")
}

func GetFileSize(filename string) (int64, error) {
	fileInfo, err := os.Stat(filename)
	if err != nil {
		return 0, err
	}
	return fileInfo.Size(), nil
}

// ConvertToBytes converts size strings like "2KB", "5MB", or "3GB" to an integer representing bytes.
func ConvertToBytes(sizeString string) int64 {
	// Extract numeric part
	size, _ := strconv.ParseInt(strings.TrimRight(sizeString, "KMGTB"), 10, 64)

	// Extract unit part
	unit := sizeString[len(sizeString)-2:]

	// Convert size to bytes based on the unit
	switch unit {
	case "KB":
		return size * 1024
	case "MB":
		return size * 1024 * 1024
	case "GB":
		return size * 1024 * 1024 * 1024
	default: // Assume bytes if no unit is specified
		return size
	}
}

// ConvertFromBytes converts an integer byte count into a human-readable string with units.
func ConvertFromBytes(bytes int) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%dB", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	value := float64(bytes) / float64(div)
	units := []string{"KB", "MB", "GB", "TB", "PB", "EB", "ZB", "YB"}
	return fmt.Sprintf("%.1f%s", value, units[exp])
}

func ExtractExtension(fpath string) string {
	parsedURL, err := url.Parse(fpath)
	if err != nil {
		return ""
	}

	filePath := parsedURL.Path
	ext := path.Ext(filePath)
	return strings.TrimPrefix(ext, ".")
}

func ConvertICOtoPNG(icoBuffer *bytes.Buffer) (*bytes.Buffer, error) {
	img, err := ico.Decode(icoBuffer)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ICO: %v", err)
	}

	pngBuffer := &bytes.Buffer{}
	err = png.Encode(pngBuffer, img)
	if err != nil {
		return nil, fmt.Errorf("failed to encode PNG: %v", err)
	}

	return pngBuffer, nil
}
