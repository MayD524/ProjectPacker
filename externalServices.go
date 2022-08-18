package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const configPath string = "project_packer/config.toml"

const defaultAppConfig string = `
AppVersion = "v1.0"
AppAuthor = "May Draskovics"
AppAPI = "something.com"
AutoUpdate = false
[Projects]
[Modules]
`

type (
	app_config struct {
		// what does this need?
		AppVersion string
		AppAuthor  string
		AppAPI     string
		AutoUpdate bool
		Projects   map[string]project
		Modules    map[string]module
	}

	module struct {
		Name    string // name just to have it
		Version string
		Path    string
		Farg    string // formated args
	}

	project struct {
		Name     string // name just to have it
		Owner    string
		TomlPath string
	}
)

func getConfigPath() string {
	//p, _ := os.UserConfigDir()
	p, _ := os.Getwd()
	return filepath.Join(p, configPath)
}

func getConfig() *app_config {
	var data app_config = app_config{}
	toml.DecodeFile(getConfigPath(), &data)
	return &data
}

func listModules(conf *app_config) {
	for name, mod := range conf.Modules {
		fmt.Printf("%s (%s) - %s\n", name, mod.Version, mod.Path)
	}
}

func listProjects(conf *app_config) {
	for name, proj := range conf.Projects {
		fmt.Printf("%s - %s\n", name, proj.TomlPath)
	}
}

func writeConfig(configFile string, data *app_config) error {
	f, err := os.OpenFile(configFile, os.O_WRONLY, 0644)
	defer f.Close()
	eCheck(err)

	if err := toml.NewEncoder(f).Encode(&data); err != nil {
		return err
	}

	return nil
}

func addProject(conf *app_config, proj *projectConfig) {
	if _, found := conf.Projects[proj.ProjectName]; found {
		return // already exists
	}
	conf.Projects[proj.ProjectName] = project{
		Name:     proj.ProjectName,
		Owner:    proj.Author,
		TomlPath: strings.Replace(proj.ProjectPath, "\\", "/", -1),
	}
}

func createConfig() *app_config {
	cfgPath := getConfigPath()
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0770); err != nil {
		panic(err)
	}
	f, err := os.Create(cfgPath)
	eCheck(err)
	defer f.Close()

	var config app_config
	toml.Decode(defaultAppConfig, &config)
	return &config
}

func InitConfig() *app_config {
	var conf *app_config
	if _, err := os.Stat(getConfigPath()); err == nil {
		conf = getConfig()
	} else {
		conf = createConfig()
	}
	return conf
}

func CloseConfig(conf *app_config) {
	err := writeConfig(getConfigPath(), conf)
	eCheck(err)
}
