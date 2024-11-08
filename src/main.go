package main

import (
	"github.com/yosuke-furukawa/json5/encoding/json5"
	"log"
	"main/src/sources"
	"main/src/utils"
	"net/url"
)

func main() {
	file, err := utils.OpenConfigFile()
	if err != nil {
		log.Fatal("Could not open configuration file:", err)
	}

	var config Configuration
	err = json5.Unmarshal(file, &config)
	if err != nil {
		log.Fatal("Could not parse configuration file:", err)
	}

	config.PopulateDefault()

	if err := config.Validate(); err != nil {
		log.Fatal("Configuration error:", err)
	}

	gitlabUrl, _ := url.Parse(*config.Gitlab.URL)
	gitlab := NewGitLab(*gitlabUrl, *config.Gitlab.Token)

	dufsUrl, _ := url.Parse(*config.Dufs.URL)
	dufs := NewDufs(*dufsUrl)

	var github *sources.Github
	if config.Sources.GitHub != nil {
		github = sources.NewGithub(config.Sources.GitHub.Token)
	}

	var huggingFace *sources.HuggingFace
	if config.Sources.HuggingFace != nil {
		huggingFace = sources.NewHuggingFace(config.Sources.HuggingFace.Token)
	}

	var sGitlab *sources.Gitlab
	if config.Sources.Gitlab != nil {
		sGitlab = sources.NewGitlab(config.Sources.Gitlab.Token)
	}

	for _, configRepo := range config.Groups {
		var source sources.Source

		if configRepo.Source == sources.GitHubID {
			source = github
		} else if configRepo.Source == sources.HuggingFaceID {
			source = huggingFace
		} else if configRepo.Source == sources.GitlabID {
			source = sGitlab
		} else {
			log.Fatalf("source '%s' not found in group with username '%s'", configRepo.Source, configRepo.Username)
		}

		SyncUser(gitlab, dufs, configRepo, source)
	}
}
