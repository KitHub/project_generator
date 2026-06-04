package logic

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"text/template"

	"github.com/KitHub/project_generator/entity"
	"golang.org/x/sys/unix"
)

var projectLogic *ProjectLogic
var onceProjectLogic sync.Once

type ProjectLogic struct {
}

// GenerateProject implements [ProjectLogic].
func (p *ProjectLogic) GenerateProject(ctx context.Context, param entity.GenerateProjectParam, templateDir string) (entity.GenerateProjectResult, error) {
	slog.InfoContext(ctx, "generating project", slog.Any("param", param))

	// create temp dir
	projectDir, err := os.MkdirTemp("", "ProjectGenerator-"+param.ProjectName+"-*")
	if err != nil {
		slog.ErrorContext(ctx, "failed to create temp dir", slog.Any("error", err))
		return entity.GenerateProjectResult{}, err
	}
	slog.InfoContext(ctx, "created temp dir", slog.String("dir", projectDir))

	if templateDir == "" {
		templateDir, err = getDefaultTemplateDirPath(ctx)
	}
	if err != nil {
		slog.ErrorContext(ctx, "failed to get default template directory path", slog.Any("error", err))
		return entity.GenerateProjectResult{}, err
	}

	// create project files
	err = createProjectFiles(ctx, projectDir, templateDir)
	if err != nil {
		slog.ErrorContext(ctx, "failed to create project files", slog.Any("error", err))
		return entity.GenerateProjectResult{}, err
	}

	// render templates with project info
	err = filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			slog.ErrorContext(ctx, "failed to walk project directory", slog.Any("error", err), slog.String("path", path))
			return err
		}
		if !info.IsDir() {
			content, err := render(ctx, path, param)
			if err != nil {
				slog.ErrorContext(ctx, "failed to render template", slog.Any("error", err), slog.String("path", path))
				return err
			}
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				slog.ErrorContext(ctx, "failed to write rendered content to file", slog.Any("error", err), slog.String("path", path))
				return err
			}
			slog.InfoContext(ctx, "rendered and wrote file successfully", slog.String("path", path))
		}
		return nil
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to render templates", slog.Any("error", err))
		return entity.GenerateProjectResult{}, err
	}

	slog.InfoContext(ctx, "project generated successfully", slog.String("projectDir", projectDir))
	return entity.GenerateProjectResult{ProjectFilesDir: projectDir}, nil
}

func NewProjectLogic() *ProjectLogic {
	onceProjectLogic.Do(func() {
		projectLogic = &ProjectLogic{}
	})
	return projectLogic
}

func getDefaultTemplateDirPath(ctx context.Context) (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		slog.ErrorContext(ctx, "failed to get working directory", slog.Any("error", err))
		return "", err
	}
	return filepath.Join(wd, "templates"), nil
}

func createProjectFiles(ctx context.Context, root string, templateDir string) error {
	slog.InfoContext(ctx, "creating project files", slog.String("root", root), slog.String("templateDir", templateDir))

	info, err := os.Stat(root)
	if err != nil {
		slog.ErrorContext(ctx, "failed to stat root path", slog.Any("error", err), slog.String("root", root))
		return err
	}

	if !info.IsDir() {
		slog.ErrorContext(ctx, "root path is not a directory", slog.String("root", root))
		return fmt.Errorf("root path is not a directory")
	}

	// check write permission
	if unix.Access(root, unix.W_OK) != nil {
		slog.ErrorContext(ctx, "root path is not writable", slog.String("root", root))
		return fmt.Errorf("root path is not writable")
	}

	// copy template files to root
	err = os.CopyFS(root, os.DirFS(templateDir))
	if err != nil {
		slog.ErrorContext(ctx, "failed to copy template files", slog.Any("error", err), slog.String("templateDir", templateDir), slog.String("root", root))
		return err
	}

	slog.InfoContext(ctx, "project files created successfully", slog.String("root", root))
	return nil
}

func render(ctx context.Context, templateFilePath string, projectInfo entity.GenerateProjectParam) (string, error) {
	slog.InfoContext(ctx, "Rendering template", slog.String("templateFilePath", templateFilePath), slog.Any("projectInfo", projectInfo))
	tpl, err := template.ParseFiles(templateFilePath)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to parse template file", slog.String("templateFilePath", templateFilePath), slog.String("error", err.Error()))
		return "", err
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, projectInfo); err != nil {
		slog.ErrorContext(ctx, "Failed to execute template", slog.String("templateFilePath", templateFilePath), slog.String("error", err.Error()))
		return "", err
	}
	slog.InfoContext(ctx, "Template rendered successfully", slog.String("templateFilePath", templateFilePath))
	return buf.String(), nil
}
