package main

import (
	"flag"
	"log"
	"main/src/configuration"
	"main/src/dest"
	"main/src/sources"
	"main/src/utils"

	"github.com/muhammadmuzzammil1998/jsonc"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "c", "", "path to the configuration file (shorthand for --config)")
	flag.StringVar(&configPath, "config", "", "path to the configuration file; defaults to config.json(c) or configuration.json(c) in the working directory")
	flag.Parse()

	file, err := utils.OpenConfigFile(configPath)
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
		if utils.CheckPrefix(source.ID, "github") {
			ret[source.ID] = sources.NewGithub(source)
		} else if utils.CheckPrefix(source.ID, "huggingface") {
			ret[source.ID] = sources.NewHuggingFace(source)
		} else if utils.CheckPrefix(source.ID, "gitlab") {
			ret[source.ID] = sources.NewGitlab(source)
		} else if utils.CheckPrefix(source.ID, "gitea") || utils.CheckPrefix(source.ID, "lavaforge") {
			ret[source.ID] = sources.NewGitea(source)
		} else if utils.CheckPrefix(source.ID, "git") {
			ret[source.ID] = sources.NewGit(source)
		}
	}

	return ret
}

func mapDestinations(config configuration.Configuration) map[string]dest.Destination {
	ret := map[string]dest.Destination{}
	for _, dst := range config.Destinations {
		if utils.CheckPrefix(dst.ID, "gitlab") {
			ret[dst.ID] = dest.NewGitLab(dst)
		} else if utils.CheckPrefix(dst.ID, "dufs") {
			ret[dst.ID] = dest.NewDufs(dst)
		} else if utils.CheckPrefix(dst.ID, "gitea") {
			ret[dst.ID] = dest.NewGitea(dst)
		}
	}

	return ret
}
