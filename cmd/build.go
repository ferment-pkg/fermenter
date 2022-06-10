/*
Copyright © 2022 NotTimIsReal

*/
package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/go-git/go-git/v5"
	"github.com/spf13/cobra"
	"github.com/theckman/yacspin"
	"github.com/xi2/xz"
)

// buildCmd represents the build command
var buildCmd = &cobra.Command{
	Use:   "build <package>",
	Short: "Build and upload prebuilds",
	Long:  `Build and upload prebuilds to the server holding other prebuilds`,
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
		runBuildCommand(pkg, args[0])
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
}
func compress(outputPath string, inputPath string) {
	var file *os.File
	var err error
	var writer *gzip.Writer
	var body []byte

	if file, err = os.OpenFile(outputPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644); err != nil {
		log.Fatalln(err)
	}
	defer file.Close()

	if writer, err = gzip.NewWriterLevel(file, gzip.BestCompression); err != nil {
		log.Fatalln(err)
	}
	defer writer.Close()

	tw := tar.NewWriter(writer)
	defer tw.Close()

	if body, err = ioutil.ReadFile(inputPath); err != nil {
		log.Fatalln(err)
	}

	if body != nil {
		hdr := &tar.Header{
			Name: path.Base(inputPath),
			Mode: int64(0644),
			Size: int64(len(body)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			println(err)
		}
		if _, err := tw.Write(body); err != nil {
			println(err)
		}
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
func runBuildCommand(path string, pkg string) {
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
	if !build(pkg, path) {
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
func build(pkg string, path string) bool {
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
			return false
		}
	} else {
		GetDownloadUrl(pkg, path)
	}
	spinner.Stop()
	return true
}
func Untar(dst string, r io.Reader, pkg string, isGz bool) (string, error) {
	if !isGz {
		untarxz(r, pkg, dst)
		return "", nil
	}
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return "", err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()

		switch {

		// if no more files are found return
		case err == io.EOF:
			return "", nil

		// return any other error
		case err != nil:
			return "", err

		// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			continue
		}

		// the target location where the dir/file should be created
		header.Name = fmt.Sprintf("%s/%s", pkg, strings.Join(strings.Split(header.Name, "/")[1:], "/"))
		target := filepath.Join(dst, header.Name)

		// the following switch could also be done using fi.Mode(), not sure if there
		// a benefit of using one vs. the other.
		// fi := header.FileInfo()

		// check the file type
		switch header.Typeflag {

		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return "", err
				}
			}

		// if it's a file create it
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return "", err
			}
			// copy over contents
			if _, err := io.Copy(f, tr); err != nil {
				return "", err
			}

			// manually close here after each file operation; defering would cause each file close
			// to wait until all operations have completed.
			f.Close()
		}
	}

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
	var isGZ bool
	if strings.Contains(url, ".gz") {
		isGZ = true
	} else {
		isGZ = false
	}
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
	path, err := Untar("/tmp/fermenter/", resp.Body, convertToReadableString(strings.ToLower(pkg)), isGZ)
	if err != nil {
		fmt.Println(color.RedString("Unable to extract %s", pkg))
		panic(err)
	}
	return path
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
func untarxz(r io.Reader, pkg string, dst string) {
	// Create an xz Reader
	r, err := xz.NewReader(r, 0)
	if err != nil {
		log.Fatal(err)
	}
	// Create a tar Reader
	tr := tar.NewReader(r)
	// Iterate through the files in the archive.

	for {
		header, err := tr.Next()

		switch {

		// if no more files are found return
		case err == io.EOF:
			break

		// return any other error
		case err != nil:
			break

		// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			continue
		}

		// the target location where the dir/file should be created
		header.Name = fmt.Sprintf("%s/%s", pkg, strings.Join(strings.Split(header.Name, "/")[1:], "/"))
		target := filepath.Join(dst, header.Name)

		// the following switch could also be done using fi.Mode(), not sure if there
		// a benefit of using one vs. the other.
		// fi := header.FileInfo()

		// check the file type
		switch header.Typeflag {

		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					panic(err)
				}
			}

		// if it's a file create it
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				panic(err)
			}
			// copy over contents
			if _, err := io.Copy(f, tr); err != nil {
				panic(err)
			}

			// manually close here after each file operation; defering would cause each file close
			// to wait until all operations have completed.
			f.Close()
		}
	}
}
