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
		log.Fatal(err)
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

	for _, configRepo := range config.Groups {
		fmt.Println("\n==========================")
		fmt.Printf("Evaluating group %s\n", configRepo.Username)
		fmt.Println("==========================")

		var source sources.Source
		var configSource ConfigRepo

		if github.GetID() == configRepo.Source {
			source = github
			configSource = config.Sources.GitHub.Config
		} else if huggingFace.GetID() == configRepo.Source {
			source = huggingFace
			configSource = config.Sources.HuggingFace.Config
		} else {
			log.Fatalf("source %s not found", configRepo.Source)
		}

		if github.GetID() == configRepo.Source && config.Sources.GitHub == nil {
			log.Fatal("source github missing configuration")
		}

		if huggingFace.GetID() == configRepo.Source && config.Sources.HuggingFace == nil {
			log.Fatal("source github missing configuration")
		}

		SyncUser(gitlab, dufs, configSource, configRepo, source)
	}
}
