package main

import (
	"github.com/yosuke-furukawa/json5/encoding/json5"
	"log"
	"main/src/configuration"
	"main/src/dest"
	"main/src/sources"
	"main/src/utils"
	"strings"
)

func main() {
	file, err := utils.OpenConfigFile()
	if err != nil {
		log.Fatal("Could not open configuration file:", err)
	}

	var config configuration.Configuration
	err = json5.Unmarshal(file, &config)
	if err != nil {
		log.Fatal("Could not parse configuration file:", err)
	}

	config.PopulateDefault()

	if err := config.Validate(); err != nil {
		log.Fatal("Configuration error:", err)
	}

	gitlab := dest.NewGitLab(config.Gitlab)
	dufs := dest.NewDufs(config.Dufs)

	srcs := mapSources(config)
	for _, configRepo := range config.Groups {
		source := srcs[configRepo.Source]
		if source == nil {
			log.Fatalf("source '%s' not found in group with username '%s'", configRepo.Source, configRepo.Username)
		}

		SyncUser(gitlab, dufs, configRepo, source)
	}
}

func mapSources(config configuration.Configuration) map[string]sources.Source {
	ret := map[string]sources.Source{}
	for _, source := range config.Sources {
		if strings.HasPrefix(source.Id, "github") {
			ret[source.Id] = sources.NewGithub(source.Token)
		} else if strings.HasPrefix(source.Id, "huggingface") {
			ret[source.Id] = sources.NewHuggingFace(source.Token)
		} else if strings.HasPrefix(source.Id, "gitlab") {
			ret[source.Id] = sources.NewGitlab(source.BaseURL, source.Token)
		}
	}

	return ret
}
