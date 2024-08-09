package utils

import (
	"fmt"
	"io"
	"net/http"
	"os"
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
