package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	sharedDir := os.Getenv("CHAINCODE_SHARED_DIR")
	fs := http.FileServer(http.Dir(sharedDir))

	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		log.Printf("Url=%s Method %s", request.URL.Path, request.Method)
		if request.Method == "POST" {
			serveUpload(sharedDir, writer, request)
		} else if request.Method == "GET" {
			fs.ServeHTTP(writer, request)
		}
	})
	httpAddress := os.Getenv("HTTP_ADDRESS")
	if httpAddress == "" {
		httpAddress = ":8080"
	}
	log.Printf("Listening on %s", httpAddress)
	err := http.ListenAndServe(httpAddress, nil)
	if err != nil {
		log.Fatalf("Failed to start server")
	}
}

func serveUpload(sharedDir string, w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Println(err)
		return
	}
	completePath := fmt.Sprintf("%s%s", sharedDir, r.URL.Path)
	log.Printf("File will be uploaded to %s", completePath)
	dir := filepath.Dir(r.URL.Path)
	err = os.MkdirAll(fmt.Sprintf("%s%s", sharedDir, dir), 0755)
	if err != nil {
		fmt.Println(err)
		return
	}
	err = ioutil.WriteFile(completePath, body, 0755)
	if err != nil {
		fmt.Println(err)
		return
	}
	return
}
