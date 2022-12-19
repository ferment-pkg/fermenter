/*
Copyright © 2022 Nottimisreal
*/
package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/theckman/yacspin"
)

type pkg struct {
	name         string
	version      string
	alias        []string
	desc         string
	dependencies []string
	Dbuild       []string
	arch         []string
	source       []string
	build        string
	install      string
	test         string
	caveats      string
	license      string
}

var config yacspin.Config = yacspin.Config{
	Frequency:         100 * time.Millisecond,
	CharSet:           yacspin.CharSets[57],
	Suffix:            color.GreenString(" Pre-Build"),
	SuffixAutoColon:   true,
	StopCharacter:     "✓",
	StopColors:        []string{"fgGreen"},
	StopFailCharacter: "✗",
	StopFailColors:    []string{"fgRed"},
}

// buildCmd represents the build command
var buildCmd = &cobra.Command{
	Use:   "build <path_to_fpkg>",
	Short: "Builds and uploads a fpkg",
	Long:  `Builds and uploads the fpkg.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {

		s, err := yacspin.New(config)

		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		s.Start()
		location, err := cmd.Flags().GetString("ferment")
		if err != nil {
			s.StopFailMessage(err.Error())
			s.StopFail()
			os.Exit(1)
		}
		s.Message(fmt.Sprintf("Ferment Path: %s", location))
		pkg := parseFpkg(args[0])
		s.Message(fmt.Sprintf("Building %s...", pkg.name))
		//print the version
		s.Message(fmt.Sprintf("Version: %s\n", pkg.version))
		//check if ferment binary works
		s.Message("Checking if ferment binary works...")
		var ferment string = location + "ferment"
		out, err := exec.Command(ferment, "-v").Output()
		if err != nil {
			s.StopFailMessage(err.Error())
			s.StopFail()
			os.Exit(1)
		}
		s.StopMessage(strings.Split(string(out), "\n")[0])
		s.Stop()
		dependencies := []string{}
		dependencies = append(dependencies, pkg.dependencies...)
		dependencies = append(dependencies, pkg.Dbuild...)
		fmt.Printf("Dependencies: %s\n", dependencies)
		for _, dep := range dependencies {
			installDependency(ferment, dep)
		}

	},
}

func init() {
	buildCmd.Flags().String("ferment", "/usr/local/ferment/", "Path to ferment")
	rootCmd.AddCommand(buildCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// buildCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// buildCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

func runFpkgCommand(pkgName string, version string, command string, action string, location string) {
	fmt.Printf("Parsing %s Command...\n", action)
	type Var struct {
		Name  string
		Value string
		Bool  bool
	}
	vars := []Var{}
	//set the variables
	vars = append(vars, Var{Name: "version", Value: version})
	code := strings.Split(command, "\n")
	var index int
	for i, line := range code {
		if index != i {
			continue
		}
		//remove indent until first character
		line = strings.TrimLeft(line, "\t")
		//check if line starts with $
		if strings.HasPrefix(line, "$") {
			//get the variable name
			varName := strings.Replace(strings.Split(line, "=")[0], "$", "", -1)
			//get the variable value
			varValue := strings.Join(strings.Split(line, "=")[1:], "=")
			if strings.HasPrefix(varValue, "$") {
				varValue = strings.Replace(varValue, "$", "", -1)
				for _, v := range vars {
					if v.Name == varValue {
						varValue = v.Value
					}
				}
			}
			//add the variable to the vars array
			vars = append(vars, Var{Name: varName, Value: varValue})
			index++
			continue
		}
		//check if line starts with @
		if strings.HasPrefix(line, "@") {
			//get the variable name
			varName := strings.Replace(strings.Split(line, "=")[0], "@", "", -1)
			//get the variable value
			varValue := strings.Split(line, "=")[1]
			//remove the quotes at the index 0 and at the last index but not the ones in between

			args := strings.Split(varValue, " ")
			var command []string
			for _, arg := range args {
				if strings.HasPrefix(arg, "$") {
					arg = strings.Replace(arg, "$", "", -1)
					for _, v := range vars {
						if v.Name == arg {
							arg = v.Value
						}
					}
				}

				command = append(command, arg)
			}
			command[0] = strings.Replace(command[0], "\"", "", 1)
			//remove quote mark at the end of the string
			command[len(command)-1] = strings.TrimSuffix(command[len(command)-1], "\"")
			//run the command
			fmt.Println(fmt.Sprintf("Running Command: %s", strings.Join(command, " ")))
			//check if the command is a built in command and if it is not run the code below
			if command[0] == "match" {
				identifier := []string{"==", "<", ">", "!="}
				var identifierFound int
				for i, id := range identifier {
					if command[2] == id {
						identifierFound = i
						break
					}
				}
				comparedTo := strings.Join(command[3:], " ")
				comparedTo = strings.Replace(comparedTo, "\"", "", -1)
				if identifierFound == 0 {
					//compare the values of index 2 and 3
					vars = append(vars, Var{Name: varName, Bool: command[1] == comparedTo})
					index++
					continue
				}
				if identifierFound == 1 {
					//compare the values of index 2 and 3
					vars = append(vars, Var{Name: varName, Bool: command[1] < comparedTo})
					index++
					continue
				}
				if identifierFound == 2 {
					//compare the values of index 2 and 3
					vars = append(vars, Var{Name: varName, Bool: command[1] > comparedTo})
					index++
					continue
				}
				if identifierFound == 3 {
					//compare the values of index 2 and 3
					vars = append(vars, Var{Name: varName, Bool: command[1] != comparedTo})
					index++
					continue
				}
				fmt.Println("Identifier for match invalid")
				os.Exit(1)

			}
			cmd := exec.Command(command[0], command[1:]...)
			cmd.Dir = fmt.Sprintf("%s/Installed/%s", location, pkgName)
			//save stdout as buff
			var buff bytes.Buffer
			cmd.Stdout = &buff
			err := cmd.Run()
			if err != nil {
				fmt.Println(err.Error())
				os.Exit(1)
			}
			//add the variable to the vars array
			vars = append(vars, Var{Name: varName, Value: strings.TrimSuffix(buff.String(), "\n")})
			index++
			continue

		}
		if strings.HasPrefix(line, "arm64: ") || strings.HasPrefix(line, "amd64: ") {
			if strings.HasPrefix(line, "arm64: ") && runtime.GOARCH == "arm64" {
				index++
				continue

			}
			if strings.HasPrefix(line, "amd64: ") && runtime.GOARCH == "amd64" {
				index++
				continue
			}
			//find where double indent ends
			for i, line := range code[index:] {
				if !strings.HasPrefix(line, "\t\t") {
					index = i
					break
				}
			}
			continue

		}
		//run the command
		fmt.Println(fmt.Sprintf("Running Command: %s", line))
		args := strings.Split(line, " ")
		var command []string
		for _, arg := range args {
			if strings.HasPrefix(arg, "$") {
				arg = strings.Replace(arg, "$", "", -1)
				for _, v := range vars {
					if v.Name == arg {
						arg = v.Value
					}
				}
			}
			command = append(command, arg)
		}
		if command[0] == "match" && strings.ToLower(action) == "test" {
			identifier := []string{"==", "<", ">", "!="}
			var identifierFound int
			for i, id := range identifier {
				if command[2] == id {
					identifierFound = i
					break
				}

			}
			comparedTo := strings.Join(command[3:], " ")
			comparedTo = strings.Replace(comparedTo, "\"", "", -1)
			if identifierFound == 0 && command[1] != comparedTo {
				//compare the values of index 2 and 3
				message := fmt.Sprintf("Test Failed: %s %s %s", command[1], command[2], command[3])
				fmt.Println(message)
				os.Exit(1)

			}
			if identifierFound == 1 && command[1] >= comparedTo {
				//compare the values of index 2 and 3
				message := fmt.Sprintf("Test Failed: %s %s %s", command[1], command[2], command[3])
				fmt.Println(message)
				os.Exit(1)
			}
			if identifierFound == 2 && command[1] <= comparedTo {
				//compare the values of index 2 and 3
				message := fmt.Sprintf("Test Failed: %s %s %s", command[1], command[2], command[3])
				fmt.Println(message)

				os.Exit(1)

			}
			if identifierFound == 3 && command[1] == comparedTo {
				//compare the values of index 2 and 3
				message := fmt.Sprintf("Test Failed: %s %s %s", command[1], command[2], command[3])
				fmt.Println(message)

				os.Exit(1)
			}
			if identifierFound > len(identifier) {
				fmt.Println("Identifier for match invalid")

				os.Exit(1)
			}
			break

		}
		if command[0] == "fileMan" {
			mode := command[1]
			if mode != "append" && mode != "write" {
				fmt.Println("Invalid fileMan mode")

				os.Exit(1)
			}
			file := command[2]
			contents := command[3:]
			content := strings.Join(contents, " ")
			var f *os.File
			var err error
			if mode == "append" {
				f, err = os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			} else {
				f, err = os.OpenFile(file, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
			}
			//remove any literal \n
			content = strings.Replace(content, `\n`, "\n", -1)
			if err != nil {
				fmt.Println(err.Error())

				os.Exit(1)
			}
			if _, err := f.WriteString(content); err != nil {
				fmt.Println(err.Error())

				os.Exit(1)
			}
			if err := f.Close(); err != nil {
				fmt.Println(err.Error())

				os.Exit(1)
			}
			break

		}
		cmd := exec.Command(command[0], command[1:]...)
		cmd.Dir = fmt.Sprintf("%s/Installed/%s", location, pkgName)
		err := cmd.Run()
		if err != nil {
			fmt.Println(err.Error())

			os.Exit(1)
		}
		index++
	}
	fmt.Println(fmt.Sprintf("%s Complete!", action))
}
func parseFpkg(file string) pkg {
	c, err := os.ReadFile(file)
	if err != nil {
		panic(err)
	}
	//parse the file

	content := string(c)
	var pkg pkg
	arrayContent := strings.Split(content, "\n")
	var skipToIndex int = 0
	for i, line := range arrayContent {
		if i < skipToIndex {
			continue
		}
		lineCut := strings.Split(line, "=")
		if len(lineCut) > 1 {
			lineCut[1] = strings.ReplaceAll(lineCut[1], "\"", "")
			switch lineCut[0] {
			case "pkgname":
				pkg.name = lineCut[1]
			case "version":
				pkg.version = lineCut[1]
			case "desc":
				pkg.desc = lineCut[1]
			case "alias":
				pkg.alias = strings.Split(lineCut[1], ",")
			case "arch":
				pkg.arch = strings.Split(lineCut[1], ",")
			case "dependencies":
				pkg.dependencies = strings.Split(lineCut[1], ",")
			case "Dbuild":
				pkg.Dbuild = strings.Split(lineCut[1], ",")
			case "source":
				pkg.source = strings.Split(lineCut[1], ",")
			case "license":
				pkg.license = lineCut[1]
			case "caveats":
				pkg.caveats = lineCut[1]

			}
		} else if strings.Contains(line, "()") {
			//starting from index i look for a }
			var indexOfEnd int
			for i, line := range arrayContent[i:] {
				if strings.Contains(line, "}") {
					indexOfEnd = i
					break
				}

			}
			//get the function name
			functionName := strings.Split(line, "(")[0]
			//get the function body
			functionBody := strings.Join(arrayContent[i:i+indexOfEnd], "\n")
			//add the function to the pkg
			switch functionName {
			case "build":
				pkg.build = functionBody
			case "install":
				pkg.install = functionBody
			case "test":
				pkg.test = functionBody

			}
		}
		skipToIndex++
	}
	//check if every field in pkg is filled
	if pkg.name == "" || pkg.version == "" || pkg.desc == "" || pkg.arch == nil || pkg.source == nil || pkg.build == "" || pkg.install == "" || pkg.test == "" {
		panic("Invalid fpkg file")
	}
	return pkg
}
func installDependency(ferment string, dependency string) {
	//run ferment install dependency
	config.Suffix = color.GreenString(" Dependency " + dependency)
	s, err := yacspin.New(config)
	s.Start()
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	cmd := exec.Command(ferment, "install", dependency)
	out, err := cmd.Output()
	if err != nil {
		s.StopFailMessage(string(out))
		s.StopFail()
		os.Exit(1)

	}
	lastLineArr := strings.Split(string(out), "\n")
	//remove any element that is empty
	for i, line := range lastLineArr {
		if line == "" {
			lastLineArr = append(lastLineArr[:i], lastLineArr[i+1:]...)
		}
	}
	lastLine := lastLineArr[len(lastLineArr)-1]
	if strings.Contains(lastLine, "✓ Testing") {
		lastLine = "Installed"

	}
	s.StopMessage(color.YellowString(lastLine))
	s.Stop()

}
