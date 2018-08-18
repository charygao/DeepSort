package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/labstack/gommon/color"
	"github.com/savaki/jq"
	filetype "gopkg.in/h2non/filetype.v1"
)

var replaceSpace = strings.NewReplacer(" ", "_")
var replaceComma = strings.NewReplacer(",", "")

const charset = "abcdefghijklmnopqrstuvwxyz" +
	"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var seededRand = rand.New(rand.NewSource(time.Now().UnixNano()))

func randString(length int, charset string) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

func renameFile(path string, name string, response string) {
	originalName := filepath.Base(path)
	absPath, _ := filepath.Abs(path)
	dirPath := filepath.Dir(absPath)
	newName := response + "_" + originalName
	if len(path) > 100 {
		extension := path[len(path)-4:]
		newName = response + "_" + randString(11, charset) + extension
	}
	err := os.Rename(path, dirPath+"/"+newName)

	if err != nil {
		fmt.Println(err)
		stopDeepDetect(name)
		return
	}
}

func parseResponse(rawResponse []byte) string {
	op, _ := jq.Parse(".body.predictions.[0].classes.[0].cat")
	value, _ := op.Apply(rawResponse)
	class := strings.Split(string(value), " ")
	class = class[1:len(class)]
	result := strings.Join(class, " ")
	result = replaceSpace.Replace(result)
	result = replaceComma.Replace(result)
	result = result[:len(result)-1]
	return (result)
}

func getClass(path string, name string) {
	url := "http://localhost:8080/predict"
	var jsonStr = []byte(`{"service":"imageserv","parameters":{"input":{"width":224,"height":224},"output":{"best":1},"mllib":{"gpu":false}},"data":["` + "/" + path + `"]}`)
	// DEBUG
	//fmt.Println("Request: " + string(jsonStr))
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	req.Close = true
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		stopDeepDetect(name)
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	parsedResponse := parseResponse(body)
	color.Println(color.Yellow("[") + color.Cyan(path) + color.Yellow("]") + color.Yellow(" Response: ") + color.Green(parsedResponse))
	renameFile(path, name, parsedResponse)
}

func runRecursively(name string) ([]string, error) {
	searchDir := "Images"

	fileList := make([]string, 0)
	e := filepath.Walk(searchDir, func(path string, f os.FileInfo, err error) error {
		fileList = append(fileList, path)
		return err
	})

	if e != nil {
		stopDeepDetect(name)
		panic(e)
	}

	for _, file := range fileList {
		buf, _ := ioutil.ReadFile(file)
		if filetype.IsImage(buf) {
			getClass(file, name)
		}
	}

	return fileList, nil
}

func startDeepDetect(path string) string {
	// Starting the DeepDetect docker image
	name := randString(11, charset)
	path, _ = filepath.Abs(path)
	cmd := "docker"
	args := []string{"run", "-d", "-p", "8080:8080", "-v", path + ":/Images", "--name", name, "beniz/deepdetect_cpu"}
	if err := exec.Command(cmd, args...).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	color.Println(color.Yellow("[") + color.Cyan("CONTAINER: "+name) + color.Yellow("] ") + color.Green("Successfully started DeepDetect. "))
	// Starting the image classification service
	time.Sleep(time.Second)
	url := "http://localhost:8080/services/imageserv"
	var jsonStr = []byte(`{"mllib":"caffe","description":"image classification service","type":"supervised","parameters":{"input":{"connector":"image"},"mllib":{"nclasses":1000}},"model":{"repository":"/opt/models/ggnet/"}}`)
	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(jsonStr))
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	if resp.Status != "201 Created" {
		stopDeepDetect(name)
		os.Exit(1)
	}
	color.Println(color.Yellow("[") + color.Cyan("CONTAINER: "+name) + color.Yellow("] ") + color.Green("Successfully started the image classification service. "))
	return name
}

func stopDeepDetect(name string) {
	// Stopping the DeepDetect docker image
	cmd := "docker"
	args := []string{"stop", name}
	if err := exec.Command(cmd, args...).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func main() {
	start := time.Now()
	name := startDeepDetect(os.Args[1])
	color.Println(color.Yellow("[") + color.Cyan("CONTAINER: "+name) + color.Yellow("] ") + color.Yellow("Starting image classification.. "))
	runRecursively(name)
	stopDeepDetect(name)
	color.Println(color.Yellow("[") + color.Cyan("CONTAINER: "+name) + color.Yellow("] ") + color.Green("Successfully stopped DeepDetect. "))
	color.Println(color.Cyan("Done in ") + color.Yellow(time.Since(start)) + color.Cyan("!"))
}
