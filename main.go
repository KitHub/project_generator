package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// ProjectRequest project generation request struct
type ProjectRequest struct {
	ProjectName string `json:"project_name"` // project name
	ModuleName  string `json:"module_name"`  // go module name
	Port        string `json:"port"`         // server port
}

// templates files
var templateFiles = map[string]string{
	"main":   "templates/main.go.tmpl",
	"mod":    "templates/go.mod.tmpl",
	"router": "templates/router.go.tmpl",
	"config": "templates/config.go.tmpl",
	"readme": "templates/README.md.tmpl",
}

// renderTemplate read template file and render with data
func renderTemplate(tplPath string, data ProjectRequest) (string, error) {
	tpl, err := template.ParseFiles(tplPath)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// generateProject generate project files and return zip data
func generateProject(req ProjectRequest) ([]byte, error) {
	// create temp dir
	tmpDir, err := os.MkdirTemp("", "gen-project-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	// project root and dirs
	root := filepath.Join(tmpDir, req.ProjectName)
	dirs := []string{
		root,
		filepath.Join(root, "router"),
		filepath.Join(root, "config"),
		filepath.Join(root, "api"),
		filepath.Join(root, "service"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return nil, err
		}
	}

	// render templates
	mainContent, _ := renderTemplate(templateFiles["main"], req)
	modContent, _ := renderTemplate(templateFiles["mod"], req)
	routerContent, _ := renderTemplate(templateFiles["router"], req)
	configContent, _ := renderTemplate(templateFiles["config"], req)
	readmeContent, _ := renderTemplate(templateFiles["readme"], req)

	// define files to write
	files := map[string]string{
		filepath.Join(root, "main.go"):          mainContent,
		filepath.Join(root, "go.mod"):           modContent,
		filepath.Join(root, "README.md"):        readmeContent,
		filepath.Join(root, "router/router.go"): routerContent,
		filepath.Join(root, "config/config.go"): configContent,
	}

	// write files
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return nil, err
		}
	}

	// zip project
	zipBuf := &bytes.Buffer{}
	zw := zip.NewWriter(zipBuf)
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, _ := filepath.Rel(tmpDir, path)
		f, err := zw.Create(relPath)
		if err != nil {
			return err
		}

		b, _ := os.ReadFile(path)
		_, _ = f.Write(b)
		return nil
	})
	if err != nil {
		return nil, err
	}
	_ = zw.Close()

	return zipBuf.Bytes(), nil
}

// generateHandler HTTP api handler for project generation
func generateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.ProjectName == "" || req.ModuleName == "" || req.Port == "" {
		http.Error(w, "project_name, module_name, port are required", http.StatusBadRequest)
		return
	}

	zipData, err := generateProject(req)
	if err != nil {
		http.Error(w, "generate project failed: "+err.Error(), http.StatusInternalServerError)
		log.Printf("generate project failed: %v", err)
		return
	}

	filename := fmt.Sprintf("%s_%d.zip", req.ProjectName, time.Now().Unix())
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	_, _ = w.Write(zipData)

	log.Printf("generate project succeeded: %s | module: %s | port: %s", req.ProjectName, req.ModuleName, req.Port)
}

func main() {
	http.HandleFunc("/generate", generateHandler)

	log.Println("project generation service started, port: 8080")
	log.Println("endpoint: POST http://localhost:8080/generate")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
