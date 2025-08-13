package main

import (
	"github.com/muhammadmuzzammil1998/jsonc"
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
	err = jsonc.Unmarshal(file, &config)
	if err != nil {
		log.Fatal("Could not parse configuration file:", err)
	}

	config.PopulateDefault()

	if err := config.Validate(); err != nil {
		log.Fatal("Configuration error:", err)
	}

	srcs := mapSources(config)
	dsts := mapDestinations(config)

	for _, configRepo := range config.Groups {
		source := srcs[configRepo.Source]
		if source == nil {
			log.Fatalf("source '%s' not found in group with username '%s'", configRepo.Source, configRepo.Username)
		}

		SyncUser(dsts, configRepo, source)
	}
}

func mapSources(config configuration.Configuration) map[string]sources.Source {
	ret := map[string]sources.Source{}
	for _, source := range config.Sources {
		if strings.HasPrefix(source.ID, "github") {
			ret[source.ID] = sources.NewGithub(source.Token)
		} else if strings.HasPrefix(source.ID, "huggingface") {
			ret[source.ID] = sources.NewHuggingFace(source.Token)
		} else if strings.HasPrefix(source.ID, "gitlab") {
			ret[source.ID] = sources.NewGitlab(source.BaseURL, source.Token)
		}
	}

	return ret
}

func mapDestinations(config configuration.Configuration) map[string]dest.Destination {
	ret := map[string]dest.Destination{}
	for _, dst := range config.Destinations {
		if strings.HasPrefix(dst.ID, "gitlab") {
			ret[dst.ID] = dest.NewGitLab(dst.ID, dst.URL, dst.Token)
		} else if strings.HasPrefix(dst.ID, "dufs") {
			ret[dst.ID] = dest.NewDufs(dst.ID, dst.URL)
		}
	}

	return ret
}
