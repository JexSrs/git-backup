package main

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
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

type Response struct {
	Status int
	Body   []byte
}

func (g *GitLab) Request(method, path string, data []byte) (*Response, error) {
	pathQuery := strings.Split(path, "?")

	urlPath := g.URL.JoinPath(pathQuery[0])
	if len(pathQuery) > 1 {
		urlPath.RawQuery = pathQuery[1]
	}

	var req *http.Request
	var err error

	if data != nil {
		req, err = http.NewRequest(method, urlPath.String(), bytes.NewBuffer(data))
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req, err = http.NewRequest(method, urlPath.String(), nil)
	}

	if err != nil {
		return nil, err
	}

	req.Header.Add("Private-Token", g.APIToken)
	req.Header.Add("Accept", "*/*")

	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	return &Response{
		Status: res.StatusCode,
		Body:   body,
	}, nil
}

func (g *GitLab) isReservedName(name string) bool {
	rn := []string{
		".github",
		"badges",
		"blame",
		"blob",
		"builds",
		"commits",
		"create",
		"create_dir",
		"edit",
		"environments/folders",
		"files",
		"find_file",
		"gitlab-lfs/objects",
		"info/lfs/objects",
		"new",
		"preview",
		"raw",
		"refs",
		"tree",
		"update",
		"wikis",
	}

	for _, rn := range rn {
		if name == rn {
			return true
		}
	}
	return false
}
