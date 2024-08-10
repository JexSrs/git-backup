package utils

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

func DownloadAsset(srcUrl, dstPath string) error {
	response, err := http.Get(srcUrl)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Failed to download asset. HTTP status code: %d\n", response.StatusCode)
	}

	// Extract the directory part from the destination path which includes filename
	dir := filepath.Dir(dstPath)
	// Ensures that the directory exists. If not, it creates it.
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	file, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, response.Body)
	if err != nil {
		return err
	}

	return nil
}
