package main

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/akamensky/argparse"
	"golang.org/x/exp/slices"
)

const timeFormat string = "2006-01-02 15:04:05"

var lineBreak string

const default_config string = `
ProjectName = "dummy_project"
Author = "May Draskovics"
DueDate = "not_set"
MainFile = "main.py"
TestScript = "test.py"
Executable = "python"
ProjFiles = ["project.toml"]
ExpectedOutPuts = ["hello", "world"]
RequiredPasses = 3
TimeOutAfterSeconds = 2
CountExecutionAsPass = true
`

// TODO: eventually make this a hidden file
const default_projectFileName string = "project.toml"
const IS_DEBUG bool = false

type projectConfig struct {
	ProjectName          string
	Author               string
	ProjectPath          string
	DueDate              string
	MainFile             string
	TestScript           string
	Executable           string
	ProjFiles            []string
	ExpectedOutPuts      []string
	RequiredPasses       int
	TimeOutAfterSeconds  int
	CountExecutionAsPass bool
}

func eCheck(e error) {
	if e != nil {
		panic(e)
	}
}

func writeToml(tomlFileName string, tomlData *projectConfig) error {
	f, err := os.OpenFile(tomlFileName, os.O_WRONLY, 0644)
	defer f.Close()
	eCheck(err)

	if err := toml.NewEncoder(f).Encode(*tomlData); err != nil {
		return err
	}

	return nil
}

func printPassFail(hasPassed bool, timeTaken string, testName string, message ...string) {
	if hasPassed {
		fmt.Printf("%s: \x1b[1;32mPassed✅ %s\x1b[0m %s\n", testName, timeTaken, message)
		return
	}
	fmt.Printf("%s: \x1b[1;32mFailed❌ %s\x1b[0m %s\n", testName, timeTaken, message)
}

func isOnTime(dueDate string) bool {
	dueTime, err := time.Parse(timeFormat, dueDate)
	eCheck(err)

	return time.Now().Before(dueTime)
}

func test_project(tomlData *projectConfig) bool {
	fmt.Printf("Testing project %s by %s\n", tomlData.ProjectName, tomlData.Author)
	fmt.Printf("Constraints:\n\tRequired Passes = %d\n\tMust execute within %d seconds\n\tExpected outputs in order:\n\t\t%s\n\n", tomlData.RequiredPasses, tomlData.TimeOutAfterSeconds, strings.Join(tomlData.ExpectedOutPuts, "\n\t\t"))
	fmt.Println(strings.Repeat("=", 100) + "\n")
	// first thing check time
	if tomlData.DueDate == "not_set" || tomlData.DueDate == "no due date" {
		// this check passes
		printPassFail(true, "0", "On time")
	} else if isOnTime(tomlData.DueDate) {
		printPassFail(true, "0", "On time")
	} else {
		printPassFail(false, "0", "On time")
	}

	type output struct {
		out []byte
		err error
	}

	ch := make(chan output)
	timeStart := time.Now()
	currentPasses := 0

	go func() {
		cmd := exec.Command(tomlData.Executable, tomlData.TestScript)
		out, err := cmd.CombinedOutput()
		ch <- output{out, err}
	}()

	select {
	case <-time.After(time.Duration(tomlData.TimeOutAfterSeconds) * time.Second):
		fmt.Print("timed out: ")
		printPassFail(false, strconv.Itoa(tomlData.TimeOutAfterSeconds)+"s", tomlData.TestScript)
		return false
	case x := <-ch:
		timeTaken := time.Since(timeStart)
		if x.err != nil {
			fmt.Printf("program errored: %s\n", x.err)
			printPassFail(false, strconv.FormatInt(timeTaken.Milliseconds(), 10)+"ms", tomlData.TestScript, "Failed Execution")
			return false
		}

		printPassFail(true, strconv.FormatInt(timeTaken.Milliseconds(), 10)+"ms", tomlData.TestScript, "Finished Execution")

		if tomlData.CountExecutionAsPass {
			currentPasses++
		}

		// handle output data
		outSplit := strings.Split(string(x.out), lineBreak)

		for ind, elm := range outSplit {
			if elm == "" {
				break // done
			}
			if ind >= len(tomlData.ExpectedOutPuts) {
				break // were done
			}

			if strings.ToLower(tomlData.ExpectedOutPuts[ind]) == strings.ToLower(elm) {
				currentPasses++
				printPassFail(true, "", tomlData.TestScript, fmt.Sprintf("Got the expected result '%s'!", elm))
			} else {
				printPassFail(false, "", tomlData.TestScript, fmt.Sprintf("Got '%s' expected '%s' as a result", elm, tomlData.ExpectedOutPuts[ind]))
			}

		}
	}

	return tomlData.RequiredPasses <= currentPasses
}

func pack_project(tomlData *projectConfig) {
	// generic file handling
	f, err := os.Create(tomlData.ProjectName + ".zip")
	eCheck(err)
	defer f.Close()

	zwriter := zip.NewWriter(f)
	defer zwriter.Close()

	for _, filename := range tomlData.ProjFiles {
		fn, err := filepath.Abs(filename)
		fmt.Printf("Compressing %s\n", fn)
		eCheck(err)
		fr, err := os.Open(fn)
		eCheck(err)

		zw, err := zwriter.Create(filename)
		if _, err := io.Copy(zw, fr); err != nil {
			panic(err)
		}
		defer fr.Close()
	}
}

func unpack_project(zipFile string) {
	reader, err := zip.OpenReader(zipFile)
	eCheck(err)

	defer reader.Close()

	// get current path
	dest, err := os.Getwd()
	eCheck(err)

	dest = filepath.Join(dest, strings.Replace(zipFile, ".zip", "", -1))
	if err := os.MkdirAll(dest, os.ModePerm); err != nil {
		panic(err)
	}

	for _, f := range reader.File {
		filePath := filepath.Join(dest, f.Name)
		fmt.Println("unzipping file ", filePath)

		if !strings.HasPrefix(filePath, filepath.Clean(dest)+string(os.PathSeparator)) {
			fmt.Println("invalid file path")
			return
		}

		if f.FileInfo().IsDir() {
			fmt.Println("creating directory")
			os.MkdirAll(filePath, os.ModePerm)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
			panic(err)
		}
		dstFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			panic(err)
		}

		fileInArchive, err := f.Open()
		if err != nil {
			panic(err)
		}

		if _, err := io.Copy(dstFile, fileInArchive); err != nil {
			panic(err)
		}
		dstFile.Close()
		fileInArchive.Close()
	}
}

func addFileToProject(tomlData *projectConfig, tomlFile string, fileName string) {
	tomlData.ProjFiles = append(tomlData.ProjFiles, fileName)
	if err := writeToml(tomlFile, tomlData); err != nil {
		panic("There was an issue writing data!")
	}
}

func indexOf(arr []string, lookFor string) int {
	for i, elm := range arr {
		if elm == lookFor || filepath.Base(elm) == lookFor {
			return i
		}
	}
	return -1
}

func removeFile(tomlData *projectConfig, tomlFile string, fileName string, deleteFile bool) {
	i := indexOf(tomlData.ProjFiles, fileName)
	if i == -1 {
		panic("You do not have a file called " + fileName)
	}
	tomlData.ProjFiles[i] = tomlData.ProjFiles[len(tomlData.ProjFiles)-1]
	tomlData.ProjFiles = tomlData.ProjFiles[:len(tomlData.ProjFiles)-1]
	if err := writeToml(tomlFile, tomlData); err != nil {
		panic("There was an issue writing data!")
	}
	if deleteFile {
		if err := os.Remove(fileName); err != nil {
			panic(err)
		}
	}
}

func createFile(tomlData *projectConfig, conf *app_config, tomlFile, newFile string) {
	if strings.Contains(newFile, "template:") {
		tempName := strings.Split(newFile, ":")[1]
		// create a template

		if !templateExists(conf, tempName) {
			panic(fmt.Sprintf("The template %s does not exist!\n", tempName))
		}

		temp := getTemplate(conf, tempName)
		// check if the path exists
		if _, err := os.Stat(temp.Path); err != nil {
			panic(fmt.Sprintf("The template %s does not exist!\n", temp.Path))
		}

		// copy file
		p, _ := os.Getwd()
		templatedFile := filepath.Join(p, filepath.Base(temp.Path))
		copy(temp.Path, templatedFile)
		addFileToProject(tomlData, tomlFile, strings.ReplaceAll(templatedFile, "\\", "/"))
	}
	if slices.Contains(tomlData.ProjFiles, newFile) {
		fmt.Printf("The file %s already is being tracked!\n", newFile)
	} else {
		if err := os.MkdirAll(filepath.Dir(newFile), 0770); err != nil {
			panic(err)
		}
		f, err := os.Create(newFile)
		eCheck(err)
		defer f.Close()
		addFileToProject(tomlData, tomlFile, newFile)
	}
}

func fList(tomlData *projectConfig, conf *app_config) {
	fmt.Printf("\nProject Name: %s\nProject Path: %s\nProject Due:%s\nEntry Point:%s\nTest Script:%s \n",
		tomlData.ProjectName,
		tomlData.ProjectPath,
		tomlData.DueDate,
		tomlData.MainFile,
		tomlData.TestScript)
	listPackage(tomlData)
	fmt.Printf("\nConfig Path: %s\n", getConfigPath())
	println(strings.Repeat("=", 100) + "\nModules:\n")
	listModules(conf)
	println(strings.Repeat("=", 100) + "\nProjects:\n")
	listProjects(conf)
	println(strings.Repeat("=", 100) + "\nTemplates:\n")
	listTemplates(conf)
	println(strings.Repeat("=", 100))
}

func fSetDue(tomlData *projectConfig, tomlFile string, setDue int) {
	if tomlData.DueDate != "not_set" {
		panic("Cannot change the due date of a project!")
	}
	if setDue == -1 {
		tomlData.DueDate = "no due date"
	} else {
		// just gotta love time conversions
		t := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day()+1, 3, 59, 59, 0, time.UTC)
		fmt.Println(t)
		t = t.Add((time.Duration(setDue) * 24) * time.Hour)
		fmt.Println(t)
		tomlData.DueDate = t.Local().Format(timeFormat)
	}
	if err := writeToml(tomlFile, tomlData); err != nil {
		panic(err)
	}
}

func listPackage(tomlData *projectConfig) {
	fmt.Println("All files in package:")
	for i, elm := range tomlData.ProjFiles {
		fmt.Printf("\t%d - %s\n", i, elm)
	}
}

func createProject(conf *app_config) {

	tomlData := projectConfig{}
	reader := bufio.NewReader(os.Stdin)
	err := toml.Unmarshal([]byte(default_config), &tomlData)
	eCheck(err)

	fmt.Print("Project name > ")
	tomlData.ProjectName, _ = reader.ReadString('\n')
	tomlData.ProjectName = strings.Trim(tomlData.ProjectName, "\r\t\n")

	fmt.Print("Main file > ")
	tomlData.MainFile, _ = reader.ReadString('\n')
	tomlData.MainFile = strings.Trim(tomlData.MainFile, "\r\t\n")

	fmt.Print("Required passing tests > ")
	tmp, _ := reader.ReadString('\n')
	tmp = strings.Trim(tmp, "\r\t\n")
	tomlData.RequiredPasses, err = strconv.Atoi(tmp)
	eCheck(err)

	fmt.Print("Time out After x seconds > ")
	tmp, _ = reader.ReadString('\n')
	tmp = strings.Trim(tmp, "\r\t\n")
	tomlData.TimeOutAfterSeconds, err = strconv.Atoi(tmp)
	eCheck(err)

	fmt.Print("Execution counts as pass > ")
	tmp, _ = reader.ReadString('\n')
	tmp = strings.Trim(tmp, "\r\t\n")
	tomlData.CountExecutionAsPass, err = strconv.ParseBool(tmp)
	eCheck(err)

	f, err := os.Create("project.toml")
	f.Close()

	println("Now creating project.toml...")
	if err := writeToml("project.toml", &tomlData); err != nil {
		panic(err)
	}

	println("Adding main project file...")
	createFile(&tomlData, conf, "project.toml", tomlData.MainFile)

	println("Adding test file...")
	createFile(&tomlData, conf, "project.toml", "test.py")

	println("Done project created...")
}

func main() {
	conf := InitConfig()
	defer CloseConfig(conf)

	// just for cleaning up outputs
	lineBreak = "\r\n"
	if runtime.GOOS != "windows" {
		lineBreak = "\n"
	}

	parser := argparse.NewParser("Project Packer", "Handles the boring stuff of CS class")

	// Flags
	newProj := parser.Flag("n", "new_project", &argparse.Options{Default: false, Help: "Create a new project"})
	rmDel := parser.Flag("R", "remove_and_delete", &argparse.Options{Default: false, Help: "Paired with rmFile and will delete the file"})
	listConf := parser.Flag("l", "list_config", &argparse.Options{Default: false, Help: "List all files currently in the package"})
	pack := parser.Flag("p", "pack", &argparse.Options{Default: false, Help: "Used when ready to pack a project"})
	unpack := parser.Flag("u", "unpack", &argparse.Options{Default: false, Help: "Used when needing to unpack a packed project"})
	runTest := parser.Flag("t", "run_test", &argparse.Options{Default: true, Help: "Runs a test script"})

	// string values
	newTemplate := parser.String("e", "new_template", &argparse.Options{Default: "", Help: "create a new template (format) <name>:<language>:<template_path>"})
	tomlFile := parser.String("f", "toml_file", &argparse.Options{Default: default_projectFileName, Help: "The toml file for the project"})
	rmFile := parser.String("r", "remove_file", &argparse.Options{Default: "", Help: "Remove a file from the project"})
	projZip := parser.String("z", "project_zip", &argparse.Options{Default: "project.zip", Help: "The name of the archive of the project"})
	addFile := parser.String("a", "add_file", &argparse.Options{Default: "", Help: "Add a file to be packaged"})
	crtFile := parser.String("c", "create_file", &argparse.Options{Default: "", Help: "Create a file and add it to be packaged (for a template file use template:template_name)"})

	// int values
	setDue := parser.Int("d", "due_date", &argparse.Options{Default: -2, Help: "Set how many days till a project is due (-1 for no due date and 0 for end of the day)"})

	err := parser.Parse(os.Args)
	if len(os.Args) == 1 {
		fmt.Print(parser.Usage("No arguments supplied"))
		return
	}

	if err != nil {
		fmt.Print(parser.Usage(err))
		return
	}

	if *unpack {
		// unpack the project
		if _, err := os.Stat(*projZip); err != nil {
			fmt.Print("That project file does not exist")
			fmt.Print(parser.Usage(err))
			return // exit the program we crashed
		}

		unpack_project(*projZip)

		return // exit the program
	} else if *unpack {
		fmt.Print(parser.Usage("cannot unpack 'na' please supply a project zip file"))
		return
	}

	if *newProj {
		createProject(conf)
		return
	}

	// check if the toml file exists
	if _, err := os.Stat(*tomlFile); err != nil {
		fmt.Print("That toml file does not exist")
		fmt.Print(parser.Usage(err))
		return
	}
	var tomlData projectConfig

	toml.DecodeFile(*tomlFile, &tomlData)

	tomlData.ProjectPath, _ = os.Getwd()
	tomlData.ProjectPath = strings.Replace(tomlData.ProjectPath, "\\", "/", -1)
	defer writeToml(*tomlFile, &tomlData)

	addProject(conf, &tomlData)

	if *addFile != "" {
		fmt.Printf("Adding %s to project...\n", *addFile)
		if slices.Contains(tomlData.ProjFiles, *addFile) {
			fmt.Printf("The file %s already is being tracked!\n", *addFile)
		} else {
			addFileToProject(&tomlData, *tomlFile, *addFile)
		}
	}

	if *newTemplate != "" {
		//<name>:<language>:<template_path>
		fmt.Printf("Adding %s to templates...\n", *newTemplate)
		template := strings.Split(*newTemplate, ":")
		addTemplate(conf, template[0], template[1], template[2])
	}

	if *listConf {
		fList(&tomlData, conf)
	}

	if *crtFile != "" {
		// its not a template
		createFile(&tomlData, conf, *tomlFile, *crtFile)

	}

	if *rmFile != "" {
		fmt.Printf("Removing %s...\n", *rmFile)
		removeFile(&tomlData, *tomlFile, *rmFile, *rmDel)
	}

	if *setDue > -2 {
		fSetDue(&tomlData, *tomlFile, *setDue)
	}

	if *runTest {
		if finalVerdict := test_project(&tomlData); !finalVerdict {
			fmt.Print("Project failed too many tests! Check above to see what went wrong\n")
			return
		}
		fmt.Printf("Project passes at least %d tests!\nIf there are more tests to pass try them! Otherwise submit this!\n", tomlData.RequiredPasses)
	}

	if *pack {
		pack_project(&tomlData)
	}
}
