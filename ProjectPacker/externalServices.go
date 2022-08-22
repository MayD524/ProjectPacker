package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const packerConfPath string = ".project_packer/"
const configPath string = packerConfPath + "config.toml"

const defaultAppConfig string = `
AppVersion = "v1.0"
AppAuthor = "May Draskovics"
AppAPI = "something.com"
AutoUpdate = false
UserName = ""
AuthCode = ""
[Projects]
[Modules]
[Templates]
`

type (
	app_config struct {
		// what does this need?
		AppVersion string
		AppAuthor  string
		AppAPI     string
		UserName   string
		AuthCode   string
		AutoUpdate bool
		Projects   map[string]tomlProject
		Modules    map[string]tomlModule
		Templates  map[string]tomlTemplate
	}

	tomlModule struct {
		Name    string // name just to have it
		Version string
		Path    string
		Farg    string // formated args
	}

	tomlProject struct {
		Name     string // name just to have it
		Owner    string
		TomlPath string
	}

	tomlTemplate struct {
		Name     string
		Language string
		Path     string
	}
)

func getPackerPath(file string) string {
	p, _ := os.UserConfigDir()
	return filepath.Join(p, packerConfPath+file)
}

func copy(src, dst string) (int64, error) {
	srcFileStat, err := os.Stat(src)
	if err != nil {
		return 0, err
	}

	if !srcFileStat.Mode().IsRegular() {
		return 0, fmt.Errorf("%s is not a regulare file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer destination.Close()
	nBytes, err := io.Copy(destination, source)
	return nBytes, err

}

func getConfigPath() string {
	var p string
	if !IS_DEBUG {
		p, _ = os.UserConfigDir()
	} else {
		p, _ = os.Getwd()
	}
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
	conf.Projects[proj.ProjectName] = tomlProject{
		Name:     proj.ProjectName,
		Owner:    proj.Author,
		TomlPath: strings.Replace(proj.ProjectPath, "\\", "/", -1),
	}
}

func listTemplates(conf *app_config) {
	for name, mod := range conf.Templates {
		fmt.Printf("[%s] %s - %s\n", mod.Language, name, mod.Path)
	}
}

func templateExists(conf *app_config, name string) bool {
	_, found := conf.Templates[name]
	return found
}

func getTemplate(conf *app_config, name string) *tomlTemplate {
	if !templateExists(conf, name) {
		return nil
	}
	// so i can't ref a map? interesting
	temp := conf.Templates[name]
	return &temp
}

func addTemplate(conf *app_config, name string, lang string, path string) {
	if templateExists(conf, name) {
		return // the template exists just *stop*
	}

	endPath := getPackerPath(filepath.Base(path))

	// copy data
	b, err := copy(path, endPath)

	if err != nil {
		panic(err)
	}
	fmt.Printf("Copied %d bytes from %s to %s\n", b, path, endPath)

	conf.Templates[name] = tomlTemplate{
		Name:     name,
		Language: lang,
		Path:     strings.ReplaceAll(endPath, "\\", "/"),
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
		os.MkdirAll(getPackerPath("templates/"), 0770)
	}
	return conf
}

func CloseConfig(conf *app_config) {
	err := writeConfig(getConfigPath(), conf)
	eCheck(err)
}
