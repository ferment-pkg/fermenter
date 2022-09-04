/*
Copyright © 2022 NotTimIsReal
*/
package cmd

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"

	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/go-git/go-git/v5"
	"github.com/gorilla/websocket"
	"github.com/radovskyb/watcher"
	"github.com/spf13/cobra"
	"github.com/theckman/yacspin"
)

// buildCmd represents the build command
var barrellsloc string
var buildCmd = &cobra.Command{
	Use:   "build <package>",
	Short: "Build and upload prebuilds",
	Long:  `Build and upload prebuilds to the server holding other prebuilds`,
	Run: func(cmd *cobra.Command, args []string) {
		dualarch, err := cmd.Flags().GetBool("dual-arch")
		if err != nil {
			panic(err)
		}
		barrellsLoc, err := cmd.Flags().GetString("barrells")
		barrellsloc = barrellsLoc
		if err != nil {
			panic(err)
		}
		useExisting, err := cmd.Flags().GetBool("use-existing")
		if err != nil {
			panic(err)
		}
		noupload, err := cmd.Flags().GetBool("no-upload")
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
		if !useExisting {
			dep := getDependencies(pkg, args[0])
			installDependencies(dep, pkg, barrellsLoc)
			if !dualarch && !checkIfPackageIsDualArch(pkg) {
				downloadsource(args[0], barrellsLoc)
				doneBuilding := make(chan bool)
				go magicWatcher(args[0], doneBuilding)
				runBuildCommand(pkg, args[0], "")
				doneBuilding <- true
				os.Exit(0)
			} else {
				for _, arch := range []string{"amd64", "arm64"} {
					fmt.Println("Building for arch:", arch)
					downloadsource(args[0], barrellsLoc)
					doneBuilding := make(chan bool)
					go magicWatcher(args[0], doneBuilding)
					runBuildCommand(pkg, args[0], arch)

					installPKG(args[0], barrellsLoc)
					doneBuilding <- true
					if !noupload {
						uploadtoapi(args[0], arch)
					}
					//clena up

					os.Remove(fmt.Sprintf("/tmp/fermenter/%s", args[0]))
				}
				os.Exit(0)
			}

		}
		if !noupload {
			uploadtoapi(args[0], "")
		}

	},
}

func init() {
	rootCmd.AddCommand(buildCmd)

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
	buildCmd.Flags().String("barrells", fmt.Sprintf("%s/Barrells", location), "Path for the barrells")
	buildCmd.Flags().BoolP("use-existing", "E", false, "Use existing build")
	buildCmd.Flags().BoolP("no-upload", "n", false, "Build but do not upload to the server")
	buildCmd.Flags().BoolP("dual-arch", "D", false, "Build for both arches seperately and upload twice to the server")
}
func compress(outputPath string, inputPath string) {
	cmd := exec.Command("tar", "-czf", outputPath, "-C/tmp/fermenter", inputPath)
	cmd.Dir = "/tmp"
	cmd.Env = []string{"GZIP=-9", "GZIP_OPT=-9"}
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		color.Red("ERROR - COMPRESS: %s", err)
		os.Exit(1)
	}
}
func isDir(path string) (bool, error) {
	fi, err := os.Stat(path)
	if err != nil {
		fmt.Println(err)
		return false, err
	}
	switch mode := fi.Mode(); {
	case mode.IsDir():
		return true, nil
	case mode.IsRegular():
		return false, nil
	default:
		return false, errors.New("unknown file type")
	}
}
func convertToReadableString(pkg string) string {
	pkg = strings.Replace(pkg, "-", "", -1)
	pkg = strings.Replace(pkg, "_", "", -1)
	pkg = strings.Replace(pkg, ".", "", -1)
	pkg = strings.Replace(pkg, " ", "", -1)
	return pkg
}
func getFileContent(file string) ([]byte, error) {
	content, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return content, nil
}
func runBuildCommand(path string, pkg string, arch string) {
	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[14],
		Suffix:            " Build",
		SuffixAutoColon:   true,
		StopCharacter:     "✓",
		StopColors:        []string{"fgGreen"},
		StopMessage:       " Complete",
		StopFailMessage:   " Failed",
		StopFailCharacter: "✗",
		StopFailColors:    []string{"fgRed"},
	}

	spinner, err := yacspin.New(cfg)
	if err != nil {
		color.Red("ERROR - SPINNER INIT: %s", err)
		os.Exit(1)
	}
	spinner.Start()
	spinner.Message("Building")
	if !build(pkg, path, arch) {
		spinner.StopFail()
		os.Exit(1)
	}
	spinner.Stop()
}
func doesExist(file string) bool {
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return false
	}
	return true
}
func build(pkg string, path string, arch string) bool {
	if arch == "" {
		arch = "universal"
	}
	content, err := getFileContent(path)
	if err != nil {
		return false
	}
	cmd := exec.Command("python3")
	closer, err := cmd.StdinPipe()
	if err != nil {
		return false
	}
	defer closer.Close()
	r, w, _ := os.Pipe()
	cmd.Stdout = w
	cmd.Stderr = w
	cmd.Dir = path[:len(path)-len(pkg)-3]
	defer r.Close()
	defer w.Close()
	err = cmd.Start()
	if err != nil {
		os.WriteFile(fmt.Sprintf("/tmp/fermenter/%s/build.log", pkg), content, 0644)
		return false
	}
	closer.Write(content)
	closer.Write([]byte("\n"))
	io.WriteString(closer, fmt.Sprintf("pkg=%s()\n", convertToReadableString(strings.ToLower(pkg))))
	io.WriteString(closer, fmt.Sprintf(`pkg.cwd="/tmp/fermenter/%s"`, pkg)+"\n")
	io.WriteString(closer, fmt.Sprintf(`pkg.arch="%s"`, arch)+"\n")
	io.WriteString(closer, "pkg.build()\n")
	closer.Close()
	w.Close()
	cmd.Wait()
	f, err := os.OpenFile("/tmp/fermenter.log", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return false
	}
	_, err = io.Copy(f, r)
	if err != nil {
		os.WriteFile("/tmp/fermenter.log", []byte(err.Error()), 0644)
		return false
	}
	return true
}
func downloadsource(pkg string, path string) bool {
	path = path + "/" + pkg + ".py"
	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[14],
		Suffix:            " Download",
		SuffixAutoColon:   true,
		StopCharacter:     "✓",
		StopColors:        []string{"fgGreen"},
		StopMessage:       " Complete",
		StopFailMessage:   " Failed",
		StopFailCharacter: "✗",
		StopFailColors:    []string{"fgRed"},
	}

	spinner, err := yacspin.New(cfg)
	if err != nil {
		color.Red("ERROR - SPINNER INIT: %s", err)
		os.Exit(1)
	}
	spinner.Start()
	spinner.Message("Downloading")
	if UsingGit(pkg, path) {
		url := GetGitURL(pkg, path)
		err := DownloadFromGithub(url, pkg)
		if err != nil {
			spinner.StopFailMessage(err.Error())
			spinner.StopFail()
			return false
		}
	} else {
		GetDownloadUrl(pkg, path)
	}
	spinner.Stop()
	return true
}
func Untar(dst string, downloadedFile string, pkg string) error {
	os.Mkdir(dst, 0777)
	//list dst
	oldentries, err := os.ReadDir(dst)
	if err != nil {
		return err
	}
	cmd := exec.Command("tar", "-xvf", downloadedFile, "--directory", dst)

	var bytes bytes.Buffer
	cmd.Stderr = &bytes
	err = cmd.Run()

	if err != nil {
		return errors.New(bytes.String())
	}
	newentries, err := os.ReadDir(dst)
	if err != nil {
		return err
	}
	//find the difference between the two
	if len(oldentries) == 0 && len(newentries) > 0 {
		os.Rename(fmt.Sprintf("%s/%s", dst, newentries[0].Name()), fmt.Sprintf("%s/%s", dst, pkg))
	} else {
		//Using the old entries, find the first one that is not in the old entries
		for _, entry := range newentries {
			found := false
			for _, oldentry := range oldentries {
				if entry.Name() == oldentry.Name() {
					found = true
					break
				}
			}
			if !found {
				os.Rename(fmt.Sprintf("%s/%s", dst, entry.Name()), fmt.Sprintf("%s/%s", dst, pkg))
				break
			}
		}
	}

	return nil

}
func DownloadFromGithub(url string, pkg string) error {
	_, err := git.PlainClone(fmt.Sprintf("/tmp/fermenter/%s", pkg), false, &git.CloneOptions{
		URL: url,
	})
	if err != nil {
		if strings.Contains(err.Error(), "exists") {
			return fmt.Errorf("package already exists")
		}
		panic(err)
	}
	return nil
}
func UsingGit(pkg string, path string) bool {
	content, err := getFileContent(path)
	if err != nil {
		return false
	}
	cmd := exec.Command("python3")
	closer, err := cmd.StdinPipe()
	if err != nil {
		panic(err)
	}
	defer closer.Close()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = path[:len(path)-len(pkg)-3]
	cmd.Start()
	closer.Write(content)
	closer.Write([]byte("\n"))
	io.WriteString(closer, fmt.Sprintf("pkg=%s()\n", convertToReadableString(strings.ToLower(pkg))))
	io.WriteString(closer, "print(pkg.git)\n")
	closer.Close()
	w.Close()
	cmd.Wait()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String() == "True\n"
	//fmt.Println(out)

}
func GetGitURL(pkg string, path string) string {
	content, err := getFileContent(path)
	if err != nil {
		if strings.Contains(err.Error(), "no such file or directory") {
			fmt.Println(color.RedString("Reinstall ferment, Barrells is missing"))
			os.Exit(1)
		}
		panic(err)
	}
	cmd := exec.Command("python3")
	closer, err := cmd.StdinPipe()
	if err != nil {
		panic(err)
	}
	defer closer.Close()
	r, w, _ := os.Pipe()
	cmd.Stdout = w
	cmd.Stderr = os.Stderr
	cmd.Dir = path[:len(path)-len(pkg)-3]
	cmd.Start()
	closer.Write(content)
	closer.Write([]byte("\n"))
	io.WriteString(closer, fmt.Sprintf("pkg=%s()\n", convertToReadableString(strings.ToLower(pkg))))
	io.WriteString(closer, "print(pkg.url)\n")
	closer.Close()
	w.Close()
	cmd.Wait()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()

}
func DownloadFromTar(pkg string, url string) string {
	resp, err := http.Get(url)
	if err != nil {
		fmt.Println(color.RedString("Unable to download %s", pkg))
		panic(err)
	}
	if err != nil {
		fmt.Println(color.RedString("Unable to download %s", pkg))
		panic(err)
	}
	defer resp.Body.Close()
	pkg = convertToReadableString(strings.ToLower(pkg))
	fileName := strings.Split(url, "/")[len(strings.Split(url, "/"))-1]
	file, err := os.OpenFile(fmt.Sprintf("/tmp/%s", fileName), os.O_CREATE|os.O_WRONLY, 0777)
	if err != nil {
		panic(err)
	}
	io.Copy(file, resp.Body)
	err = Untar("/tmp/fermenter/", fmt.Sprintf("/tmp/%s", fileName), convertToReadableString(strings.ToLower(pkg)))
	if err != nil {
		fmt.Println(color.RedString("Unable to extract %s", pkg))
		panic(err)
	}
	return fmt.Sprintf("/tmp/fermenter/%s", pkg)
}
func GetDownloadUrl(pkg string, path string) string {
	content, err := getFileContent(path)
	if err != nil {
		if strings.Contains(err.Error(), "no such file or directory") {
			fmt.Println(color.RedString("Reinstall ferment, Barrells is missing"))
			os.Exit(1)
		}
		panic(err)
	}
	cmd := exec.Command("python3")
	closer, err := cmd.StdinPipe()
	if err != nil {
		panic(err)
	}
	defer closer.Close()
	r, w, _ := os.Pipe()
	cmd.Stdout = w
	cmd.Stderr = os.Stderr
	cmd.Dir = path[:len(path)-len(pkg)-3]
	err = cmd.Start()
	if err != nil {
		panic(err)
	}
	closer.Write(content)
	closer.Write([]byte("\n"))
	io.WriteString(closer, fmt.Sprintf("pkg=%s()\n", convertToReadableString(strings.ToLower(pkg))))
	io.WriteString(closer, "print(pkg.url)\n")
	closer.Close()
	w.Close()
	err = cmd.Wait()
	if err != nil {
		fmt.Println(color.RedString("Unable to get url %s", pkg))
		panic(err)
	}
	var buf bytes.Buffer
	io.Copy(&buf, r)
	path = DownloadFromTar(convertToReadableString(strings.ToLower(pkg)), strings.Replace(buf.String(), "\n", "", -1))
	return path
}

func getDependencies(path string, pkg string) []string {
	content, err := getFileContent(path)
	if err != nil {
		panic(err)
	}
	cmd := exec.Command("python3")
	closer, err := cmd.StdinPipe()
	if err != nil {
		panic(err)
	}
	defer closer.Close()
	r, w, _ := os.Pipe()
	cmd.Stdout = w
	cmd.Stderr = w
	cmd.Dir = path[:len(path)-len(pkg)-3]
	err = cmd.Start()
	if err != nil {
		panic(err)
	}
	closer.Write(content)
	closer.Write([]byte("\n"))
	io.WriteString(closer, fmt.Sprintf("pkg=%s()\n", convertToReadableString(strings.ToLower(pkg))))
	io.WriteString(closer, "print(pkg.dependencies)\n")
	closer.Close()
	w.Close()
	cmd.Wait()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	c := strings.Replace(buf.String(), " ", "", -1)
	c = strings.Replace(c, "\n", "", -1)
	c = strings.Replace(c, "[", "", -1)
	c = strings.Replace(c, "]", "", -1)
	c = strings.Replace(c, "\"", "", -1)
	c = strings.Replace(c, "'", "", -1)
	if strings.Contains(buf.String(), "AttributeError") {
		return []string{}
	}
	return strings.Split(c, ",")
}
func installDependencies(dependencies []string, path string, barrellsLoc string) {
	fmt.Println(color.GreenString("Installing dependencies"))
	//check if already installed by using which command
	if len(dependencies) == 0 {
		return
	}
	for _, dependency := range dependencies {
		//check if you can split dep by :
		var command = dependency
		if strings.Contains(dependency, ":") {
			command = strings.Split(dependency, ":")[0]
			dependency = strings.Split(dependency, ":")[1]

		}
		color.Yellow("Installing %s as dependency", dependency)
		cmd := exec.Command("which", strings.ReplaceAll(command, "'", ""))
		err := cmd.Run()
		if err == nil {
			color.Yellow("%s already installed", dependency)
			continue
		}
		if IsLib(dependency, barrellsLoc) && checkIfPackageExists(dependency) {
			color.Yellow("%s is a lib and already installed", dependency)
			continue
		}
		_, err = os.Stat(fmt.Sprintf("%s/%s.py", barrellsLoc, convertToReadableString(strings.ToLower(dependency))))
		if os.IsNotExist(err) {
			color.Yellow("%s is not downloadable by ferment, skipping...", dependency)
			continue
		}
		fmt.Printf(color.YellowString("Now Installing %s\n"), dependency)
		cmd = exec.Command("ferment", "install", dependency)
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stderr
		cmd.Stdin = os.Stdin
		err = cmd.Run()
		if err != nil {
			panic(err)
		}
	}
}
func IsLib(pkg string, location string) bool {
	content, err := os.ReadFile(fmt.Sprintf("%s/%s.py", location, convertToReadableString(strings.ToLower(pkg))))
	if err != nil {
		panic(err)
	}
	cmd := exec.Command("python3")
	closer, err := cmd.StdinPipe()
	if err != nil {
		panic(err)
	}
	defer closer.Close()
	r, w, _ := os.Pipe()
	cmd.Stdout = w
	cmd.Stderr = w
	cmd.Dir = location
	cmd.Start()
	closer.Write(content)
	closer.Write([]byte("\n"))
	io.WriteString(closer, fmt.Sprintf("pkg=%s()\n", convertToReadableString(strings.ToLower(pkg))))
	io.WriteString(closer, "print(pkg.lib)\n")
	closer.Close()
	w.Close()
	cmd.Wait()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	if strings.Contains(buf.String(), "no attribute") {
		return false
	} else {
		return strings.Contains(buf.String(), "True")
	}

}
func uploadtoapi(pkg string, arch string) {

	f, _ := os.OpenFile("/tmp/fermenter.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	l := log.New(f, "UPLOAD: ", log.Ltime)
	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[14],
		Suffix:            " Upload",
		SuffixAutoColon:   true,
		StopCharacter:     "✓",
		StopColors:        []string{"fgGreen"},
		StopMessage:       " Complete",
		StopFailMessage:   " Failed",
		StopFailCharacter: "✗",
		StopFailColors:    []string{"fgRed"},
	}

	spinner, err := yacspin.New(cfg)
	if err != nil {
		color.Red("ERROR - SPINNER INIT: %s", err)
		os.Exit(1)
	}
	spinner.Start()
	spinner.Message("Initializing...")
	compress(fmt.Sprintf("/tmp/%s.tar.gz", pkg), pkg)
	u := url.URL{Scheme: "wss", Host: "upload.fermentpkg.tech"}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		color.Red("ERROR - DIAL: %s", err)
		os.Exit(1)
	}
	keepAlive(c, time.Hour/2)
	interrupt := make(chan os.Signal, 1)
	done := make(chan bool)
	replied := make(chan bool)
	signal.Notify(interrupt, os.Interrupt)
	defer c.Close()
	go func() {
		for {
			select {
			case <-interrupt:
				color.Yellow("\nInterrupted Now Cleaning Up...")
				c.WriteMessage(websocket.CloseMessage, []byte{})
				os.Exit(1)
			case done := <-done:
				if done {
					return
				}
			}
		}
	}()
	go func() {
		for {
			//check if connection is closed
			_, content, err := c.ReadMessage()
			if err != nil {
				l.Fatal(err)
				done <- true
				break
			}
			l.Println(string(content))
			if strings.Contains(strings.ToLower(string(content)), "uploaded") {
				replied <- true
			}
		}
	}()
	split(fmt.Sprintf("/tmp/%s.tar.gz", pkg))
	if err != nil {
		spinner.StopFailMessage("Failed Line:699 - " + err.Error())
		spinner.StopFail()
	}

	type Data struct {
		File string `json:"file"`
		Part int    `json:"part"`
		Name string `json:"name"`
		Of   int    `json:"of"`
		Data string `json:"data"`
	}
	var data Data
	stat, err := os.Stat(fmt.Sprintf("/tmp/%s.tar.gz", pkg))
	if err != nil {
		spinner.StopFailMessage("Failed - " + err.Error())
		spinner.StopFail()
		os.Exit(1)
	}
	megabytes := math.Round((float64)(stat.Size() / 1e6))
	data.Of = int(megabytes / 90)
	if data.Of == 0 {
		data.Of++
	}
	data.Name = pkg
	version, err := executeQuickPython(fmt.Sprintf("import %s;pkg=%s.%s();print(pkg.version)", pkg, pkg, pkg), barrellsloc)
	if err != nil {
		spinner.StopFailMessage("Failed:VersionRetrieve - " + err.Error())
		spinner.StopFail()
		os.Exit(1)
	}
	if arch != "" {
		data.File = fmt.Sprintf("%s-%s@%s.tar.gz", pkg, arch, strings.Replace(version, "\n", "", -1))
	} else {
		data.File = fmt.Sprintf("%s@%s.tar.gz", pkg, strings.Replace(version, "\n", "", -1))
	}

	data.Part = 1
	for i := 1; i <= data.Of; i++ {
		spinner.Message(fmt.Sprintf("Uploading Part %d of %d... (%fmb)", i, data.Of, megabytes))
		data.Part = i
		//list all files in /tmp
		files, err := os.ReadDir("/tmp")
		if err != nil {
			spinner.StopFailMessage("Failed - " + err.Error())
			spinner.StopFail()
			os.Exit(1)
		}
		var f string
		for _, fi := range files {
			if strings.Contains(fi.Name(), "tar.gz") && strings.Contains(fi.Name(), fmt.Sprintf("%d", i-1)) && strings.Contains(fi.Name(), pkg) {
				f = fi.Name()
				break
			}
		}
		content, err := os.ReadFile(fmt.Sprintf("/tmp/%s", f))
		if err != nil {
			spinner.StopFailMessage("Failed - " + err.Error())
			spinner.StopFail()
			os.Exit(1)
		}
		encoded := base64Encode(content)
		data.Data = encoded
		en, err := json.Marshal(data)
		if err != nil {
			spinner.StopFailMessage("Failed - " + err.Error())
			spinner.StopFail()
			os.Exit(1)
		}
		c.EnableWriteCompression(true)
		spinner.Message("Waiting...")
		err = c.WriteMessage(websocket.TextMessage, en)
		if err != nil {
			spinner.StopFailMessage("Failed - " + err.Error())
			spinner.StopFail()
			os.Exit(1)
		}
		r := <-replied

		//wait till r is true
		for !r {
			r = <-replied
		}
		spinner.Message(fmt.Sprintf("Uploaded Part %d of %d", i, data.Of))
		os.WriteFile("test.json", en, 0644)

	}
	spinner.Message("Uploading Complete")
	spinner.Stop()
	done <- true

}
func checkIfPackageExists(pkg string) bool {
	pkg = convertToReadableString(strings.ToLower(pkg))
	_, err := os.ReadDir(fmt.Sprintf("/usr/local/ferment/Installed/%s", pkg))
	return err == nil
}
func base64Encode(str []byte) string {
	return base64.StdEncoding.EncodeToString(str)
}

// Split a file into smaller chunks
// Splits every 90mb to allow for uploads of more than 90mb
// Helps bypass cloudflare limit
func split(fileToBeChunked string) {
	file, err := os.Open(fileToBeChunked)

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	defer file.Close()

	fileInfo, _ := file.Stat()

	var fileSize int64 = fileInfo.Size()

	const fileChunk = 1e7 * 2 // 20 MB

	// calculate total number of parts the file will be chunked into

	totalPartsNum := uint64(math.Ceil(float64(fileSize) / float64(fileChunk)))

	for i := uint64(0); i < totalPartsNum; i++ {

		partSize := int(math.Min(fileChunk, float64(fileSize-int64(i*fileChunk))))
		partBuffer := make([]byte, partSize)

		file.Read(partBuffer)

		// write to disk
		fileName := fileToBeChunked + ".part" + strconv.FormatUint(i, 10)
		_, err := os.Create(fileName)

		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// write/save buffer to disk
		os.WriteFile(fileName, partBuffer, os.ModeAppend)

	}
}
func executeQuickPython(code string, barrellsLoc string) (string, error) {
	cmd := exec.Command("sudo", "python3", "-c", code)
	cmd.Dir = barrellsLoc
	var out bytes.Buffer
	var errPipe bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errPipe
	cmd.Run()
	if errPipe.Len() > 0 {
		return "", errors.New(errPipe.String())
	}
	return out.String(), nil

}

func keepAlive(c *websocket.Conn, timeout time.Duration) {
	lastResponse := time.Now()
	c.SetPongHandler(func(msg string) error {
		lastResponse = time.Now()
		return nil
	})

	go func() {
		for {
			err := c.WriteMessage(websocket.PingMessage, []byte("keepalive"))
			if err != nil {
				return
			}
			time.Sleep(timeout / 2)
			if time.Since(lastResponse) > timeout {
				c.Close()
				return
			}
		}
	}()
}
func checkIfPackageIsDualArch(pkg string) bool {
	dual, err := executeQuickPython(fmt.Sprintf("from %s import %s;pkg=%s();print(pkg.dualarch)", pkg, pkg, pkg), barrellsloc)
	if err != nil {
		return false
	}
	return dual == "True"
}
func magicWatcher(pkg string, done chan bool) {
	watch := watcher.New()
	dirsWatched := []string{"bin", "share", "include", "lib"}
	for _, dir := range dirsWatched {
		watch.Add(fmt.Sprintf("/usr/local/%s", dir))
	}
	watch.Add("/usr/local")
	go watch.Start(10 * time.Millisecond)
	watcherfile, err := os.OpenFile("/tmp/fermenter/"+pkg+"/.ferment-watcher", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0777)
	if err != nil {
		panic(err)
	}
	go func() {
		for {
			select {
			case event := <-watch.Event:
				//ger file name
				if event.Op == watcher.Create || event.Op == watcher.Write && strings.Contains(event.Path, pkg) {
					watcherfile.WriteString(event.Path + "\n")
				}
			case <-watch.Closed:
				return
			}
		}
	}()
	for {
		d := <-done
		if d {
			break
		}
	}
	watch.Close()
	watcherfile.Close()

}
