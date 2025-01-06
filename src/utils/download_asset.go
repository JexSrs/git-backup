package utils

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
)

func DownloadAsset(srcUrl string) (*bytes.Buffer, error) {
	response, err := http.Get(srcUrl)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Failed to download asset. HTTP status code: %d\n", response.StatusCode)
	}

	var buffer bytes.Buffer
	_, err = io.Copy(&buffer, response.Body)
	if err != nil {
		return nil, err
	}

	return &buffer, nil
}
