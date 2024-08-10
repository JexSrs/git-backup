package utils

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func OpenConfigFile() ([]byte, error) {
	filenames := []string{"config.json5", "configuration.json", "config.json5", "configuration.json5"}
	for _, filename := range filenames {
		content, err := os.ReadFile(filename)
		if err == nil {
			return content, nil
		}
	}
	return nil, fmt.Errorf("none of the configuration files were found.")
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
func ConvertFromBytes(bytes int64) string {
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
