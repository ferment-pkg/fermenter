/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"strings"
	"time"

	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/theckman/yacspin"
)

// testCmd represents the test command
var testCmd = &cobra.Command{
	Use:   "test",
	Short: "runs build and test",
	Long:  `Runs the build and test functions if they exist`,
	Run: func(cmd *cobra.Command, args []string) {
		barrellsLoc, err := cmd.Flags().GetString("barrells")
		if err != nil {
			panic(err)
		}
		if dir, err := isDir(barrellsLoc); err != nil || !dir {
			color.Red("ERROR: Barrells location is not a directory or does not exist")
			os.Exit(1)
		}
		if len(args) < 1 {
			color.Red("ERROR: Please specify a package to build")
			os.Exit(1)

		}
		color.Yellow("Looking for package in %s\n", barrellsLoc)
		args[0] = convertToReadableString(args[0])
		pkg := fmt.Sprintf("%s/%s.py", barrellsLoc, args[0])
		if !doesExist(pkg) {
			color.Red("ERROR: Package not found in %s\n", barrellsLoc)
			os.Exit(1)
		}
		color.Green("Found package %s\n", pkg)
		downloadsource(args[0], barrellsLoc)
		dep := getDependencies(pkg, args[0])
		installDependencies(dep, pkg, barrellsLoc)
		runBuildCommand(pkg, args[0])
		fmt.Println("Printing Logs From Build If Exists")
		fmt.Println(showLogs(args[0]))
		compress(fmt.Sprintf("%s.tar.gz", args[0]), args[0])
		fmt.Printf("Compress Path: /tmp/%s.tar.gz\n", args[0])
		installPKG(args[0], barrellsLoc)
		if !test(args[0], barrellsLoc) {
			executeQuickPython(fmt.Sprintf("import os;from %s import %s;pkg=%s();pkg.cwd='/tmp/fermenter/%s/';pkg.uninstall()", args[0], args[0], args[0], args[0]), barrellsLoc)
			os.Exit(1)
		}
		executeQuickPython(fmt.Sprintf("import os;from %s import %s;pkg=%s();pkg.cwd='/tmp/fermenter/%s/';pkg.uninstall()", args[0], args[0], args[0], args[0]), barrellsLoc)

	},
}

func init() {
	rootCmd.AddCommand(testCmd)
	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// buildCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	location, err := os.Executable()
	if err != nil {
		panic(err)
	}
	location = location[:len(location)-len("/fermenter")]
	testCmd.Flags().StringP("barrells", "b", fmt.Sprintf("%s/Barrells", location), "Path for the barrells")
	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// testCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// testCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
func test(pkg string, barrells string) bool {
	spinner, err := yacspin.New(yacspin.Config{
		CharSet:           yacspin.CharSets[57],
		Frequency:         time.Millisecond * 100,
		Suffix:            color.GreenString(" Testing %s", pkg),
		StopCharacter:     "✓",
		StopColors:        []string{"fgGreen"},
		StopFailCharacter: "✗",
		StopFailColors:    []string{"fgRed"},
		SuffixAutoColon:   true,
		StopMessage:       fmt.Sprintf("Successfully tested %s", pkg),
	})
	if err != nil {
		panic(err)
	}
	spinner.Start()
	path := fmt.Sprintf("%s/%s.py", barrells, pkg)
	content, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	lines := strings.Split(string(content), "\n")
	var found bool
	for _, line := range lines {
		if strings.Contains(line, "def test") {
			found = true
			break
		}

	}
	if !found {
		spinner.StopMessage(color.YellowString("No test found in %s", pkg))
		spinner.Stop()
		return true

	}
	spinner.Message("Found test")
	out, err := executeQuickPython(fmt.Sprintf("from %s import %s;pkg=%s();pkg.cwd='/tmp/fermenter/%s';pkg.test()", pkg, pkg, pkg, pkg), barrells)
	if err != nil || !strings.Contains(out, "True") {
		spinner.StopFailMessage(color.RedString("Failed Testing %s", pkg))
		spinner.StopFail()
		return false

	}
	spinner.Stop()
	return true

}
func installPKG(pkg string, barrells string) {
	spinner, err := yacspin.New(yacspin.Config{
		CharSet:           yacspin.CharSets[57],
		Frequency:         time.Millisecond * 100,
		Suffix:            color.GreenString(" Installing %s", pkg),
		StopCharacter:     "✓",
		StopColors:        []string{"fgGreen"},
		StopFailCharacter: "✗",
		StopFailColors:    []string{"fgRed"},
		SuffixAutoColon:   true,
		Message:           fmt.Sprintf("Installing %s", pkg),
		StopMessage:       fmt.Sprintf("Successfully installed %s", pkg),
	})
	if err != nil {
		panic(err)
	}
	spinner.Start()
	go func() {
		binary := checkIfBinaryRequired(pkg, barrells)
		if binary == nil {
			return
		}
		spinner.Message(fmt.Sprintf("Installing Binary %s", *binary))
		os.Symlink(fmt.Sprintf("/tmp/fermenter/%s/%s", pkg, *binary), fmt.Sprintf("/usr/local/bin/%s", *binary))
	}()
	_, err = executeQuickPython(fmt.Sprintf("from %s import %s;pkg=%s();pkg.prebuild.cwd='/tmp/fermenter/%s';pkg.prebuild.install()", pkg, pkg, pkg, pkg), barrells)
	if err != nil {
		panic(err)
	}
	spinner.StopMessage(color.GreenString("Successfully installed %s", pkg))
	spinner.Stop()
}
func checkIfBinaryRequired(pkg string, barrellsLoc string) *string {
	path := fmt.Sprintf("%s/%s.py", barrellsLoc, pkg)
	content, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	lines := strings.Split(string(content), "\n")
	var found bool
	for _, line := range lines {
		if strings.Contains(line, "self.binary") {
			found = true
			break
		}

	}
	if !found {
		return nil
	}
	out, err := executeQuickPython(fmt.Sprintf("from %s import %s;pkg=%s();print(pkg.binary)", pkg, pkg, pkg), barrellsLoc)
	if err != nil {
		panic(err)
	}
	out = strings.Replace(out, "\n", "", -1)
	return &out
}
func showLogs(pkg string) string {
	os.Chdir(fmt.Sprintf("/tmp/fermenter/%s", pkg))
	logs, err := os.ReadFile(fmt.Sprintf("%s-build.log", pkg))
	if err != nil {
		return ""
	}
	return string(logs)
}
