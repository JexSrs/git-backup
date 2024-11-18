package main

import (
	"github.com/yosuke-furukawa/json5/encoding/json5"
	"log"
	"main/src/dest"
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
	gitlab := dest.NewGitLab(*gitlabUrl, *config.Gitlab.Token)

	dufsUrl, _ := url.Parse(*config.Dufs.URL)
	dufs := dest.NewDufs(*dufsUrl)

	sGithub := sources.NewGithub(config.Sources.GitHub.Token)
	sHuggingFace := sources.NewHuggingFace(config.Sources.HuggingFace.Token)
	sGitlab := sources.NewGitlab(config.Sources.Gitlab.Token)

	for _, configRepo := range config.Groups {
		var source sources.Source

		if configRepo.Source == sources.GitHubID {
			source = sGithub
		} else if configRepo.Source == sources.HuggingFaceID {
			source = sHuggingFace
		} else if configRepo.Source == sources.GitlabID {
			source = sGitlab
		} else {
			log.Fatalf("source '%s' not found in group with username '%s'", configRepo.Source, configRepo.Username)
		}

		SyncUser(gitlab, dufs, configRepo, source)
	}
}
