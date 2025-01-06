package dest

import (
	"bytes"
	"fmt"
	"io"
	"main/src/configuration"
	"net/http"
	"net/url"
)

type Dufs struct {
	URL      url.URL
	RootPath string
}

func NewDufs(config configuration.ConfigDufs) *Dufs {
	dufsUrl, _ := url.Parse(*config.URL)
	return &Dufs{
		URL:      *dufsUrl,
		RootPath: *config.RootPath,
	}
}

func (d *Dufs) UploadFIle(buffer *bytes.Buffer, dstPath string) error {
	request, err := http.NewRequest(http.MethodPut, d.URL.JoinPath(d.RootPath, dstPath).String(), buffer)
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
