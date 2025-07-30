package main

import (
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
)

const savePath = "/tmp/vmware"

func main() {
	http.HandleFunc("/", bootstrapHandler)
	http.HandleFunc("/migrations/bootstrap", bootstrapHandler)
	http.HandleFunc("/upload", uploadHandler)
	err := os.Mkdir(savePath, os.ModePerm)
	if err != nil && !errors.Is(err, os.ErrExist) {
		panic(err)
	}
	http.Handle("/vmware/", http.StripPrefix("/vmware/", http.FileServer(http.Dir(savePath))))
	fmt.Println("Starting server on :8080...")
	http.ListenAndServe(":8080", nil)
}

func bootstrapHandler(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.New("form").Parse(indexhtml)
	if err != nil {
		http.Error(w, "Unable to load form", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	tmpl.Execute(w, nil)
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	// Parse the multipart form
	err := r.ParseMultipartForm(10 << 20) // Max file size 10 MB
	if err != nil {
		http.Error(w, "Error parsing form", http.StatusInternalServerError)
		return
	}

	// Retrieve file and other form data
	file, _, err := r.FormFile("vddk")
	if err != nil {
		http.Error(w, "Error retrieving file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	target, err := os.Create(path.Join(savePath, "vddk.tar.gz"))
	if err != nil {
		http.Error(w, "Error creating destination file", http.StatusInternalServerError)
		return
	}
	defer target.Close()

	_, err = io.Copy(target, file)
	if err != nil {
		http.Error(w, "Error writing destination file", http.StatusInternalServerError)
		return
	}

	envFile, err := os.Create(path.Join(savePath, "env"))
	if err != nil {
		http.Error(w, "Error creating destination file", http.StatusInternalServerError)
		return
	}
	defer envFile.Close()

	j := fmt.Sprintf("url=%s\nusername=%s\npassword=%s\n",
		r.FormValue("url"),
		r.FormValue("username"),
		r.FormValue("password"))
	_, err = io.Copy(envFile, strings.NewReader(j))
	if err != nil {
		http.Error(w, "Error writing destination env file", http.StatusInternalServerError)
		return
	}

	err = os.WriteFile(path.Join(savePath, "done"), nil, os.ModePerm)
	if err != nil {
		http.Error(w, "Error writing done file", http.StatusInternalServerError)
		return
	}
	// For now, just return a simple confirmatio
	fmt.Fprintf(w, "<html><body>vddk.tar.gz and vmware credentials recieved and avaiable under <a href=\"/vmware\" />/vmware</a></body></html>\n")
}

//go:embed index.html
var indexhtml string
