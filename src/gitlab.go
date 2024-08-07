package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	. "main/src/sources"
	"net/http"
	"net/url"
)

type GitLab struct {
	URL      url.URL
	APIToken string
}

func NewGitLab(url url.URL, apiToken string) *GitLab {
	return &GitLab{
		URL:      url,
		APIToken: apiToken,
	}
}

func (g *GitLab) Request(method, path string, data []byte) ([]byte, error) {
	var httpReq *http.Request
	var err error

	if data != nil {
		httpReq, err = http.NewRequest(method, g.URL.JoinPath(path).String(), bytes.NewBuffer(data))
	} else {
		httpReq, err = http.NewRequest(method, g.URL.JoinPath(path).String(), nil)
	}

	if err != nil {
		return nil, err
	}

	httpReq.Header.Add("Private-Token", g.APIToken)
	httpReq.Header.Add("Content-Type", "application/json")

	client := &http.Client{}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if httpResp.StatusCode >= 300 {
		if err != nil {
			return nil, fmt.Errorf("status code: %d, failed to read response body: %w", httpResp.StatusCode, err)
		}
		return nil, errors.New(fmt.Sprintf("status code: %d, body: %s", httpResp.StatusCode, string(respBody)))
	}

	return respBody, err
}
