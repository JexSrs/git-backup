package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
)

type Dufs struct {
	URL url.URL
}

func NewDufs(url url.URL) *Dufs {
	return &Dufs{url}
}

func (d *Dufs) UploadFIle(srcPath, dstPath string) error {
	file, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("error opening file: %w", err)
	}
	defer file.Close()

	request, err := http.NewRequest(http.MethodPut, d.URL.JoinPath(dstPath).String(), file)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("error performing request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(response.Body)
		return fmt.Errorf("received non-success status code: %d, body: %s", response.StatusCode, responseBody)
	}

	return nil
}

func (d *Dufs) DeletePath(path string) error {
	req, err := http.NewRequest(http.MethodDelete, d.URL.JoinPath(path).String(), nil)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error performing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("received non-success status code: %d", resp.StatusCode)
	}

	return nil
}
