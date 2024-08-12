package main

import (
	"fmt"
	"github.com/yosuke-furukawa/json5/encoding/json5"
	"log"
	"main/src/sources"
	"main/src/utils"
	"net/url"
)

func main() {
	file, err := utils.OpenConfigFile()
	if err != nil {
		log.Fatal(err)
	}

	var config Configuration
	err = json5.Unmarshal(file, &config)
	if err != nil {
		log.Fatal(err)
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

	var huggingFaceModel *sources.HuggingFace
	if config.Sources.HuggingFace != nil {
		huggingFaceModel = sources.NewHuggingFace(config.Sources.HuggingFace.Token)
	}

	for _, configRepo := range config.Groups {
		var source sources.Source
		var configSource ConfigRepo

		if configRepo.Source == sources.GitHubID {
			source = github
			configSource = config.Sources.GitHub.Config
		} else if configRepo.Source == sources.HuggingFaceID {
			source = huggingFaceModel
			configSource = config.Sources.HuggingFace.Config
		} else {
			log.Fatalf("source %s not found", configRepo.Source)
		}

		fmt.Println("\n================================================")
		fmt.Printf("Evaluating group %s from %s\n", configRepo.Username, configRepo.Source)
		fmt.Println("================================================")

		SyncUser(gitlab, dufs, configSource, configRepo, source)
	}
}
