package service

import (
	"context"
	"log/slog"
	"sync"

	"github.com/KitHub/project_generator/config"
	"github.com/KitHub/project_generator/entity"
	"github.com/KitHub/project_generator/logic"
	"github.com/KitHub/protocols/projectgeneratorapi"
)

var projectService *ProjectService
var onceProjectService sync.Once

type ProjectService struct {
	projectgeneratorapi.UnimplementedProjectGeneratorAPIServer
	ProjectLogic logic.ProjectLogic
}

// GenerateProject implements [projectgeneratorapi.ProjectGeneratorAPIServer].
func (p *ProjectService) GenerateProject(ctx context.Context, req *projectgeneratorapi.GenerateProjectRequest) (*projectgeneratorapi.GenerateProjectResponse, error) {
	slog.InfoContext(ctx, "generating project", slog.String("request", req.String()))

	param := composeProjectServiceParam(ctx, req)

	result, err := p.ProjectLogic.GenerateProject(ctx, param, config.GetConfig(ctx).Templates.TemplatesDir)
	if err != nil {
		slog.ErrorContext(ctx, "failed to generate project", slog.Any("error", err))
		return nil, err
	}
	slog.InfoContext(ctx, "project generated successfully", slog.Any("result", result))

	rsp := &projectgeneratorapi.GenerateProjectResponse{}
	return rsp, nil
}

func NewProjectService(projectLogic logic.ProjectLogic) *ProjectService {
	onceProjectService.Do(func() {
		projectService = &ProjectService{
			ProjectLogic: projectLogic,
		}
	})
	return projectService
}

func composeProjectServiceParam(ctx context.Context, req *projectgeneratorapi.GenerateProjectRequest) entity.GenerateProjectParam {
	result := entity.GenerateProjectParam{
		ProjectName:            req.GetProjectName(),
		ProjectDesc:            req.GetProjectDescription(),
		ProjectLanguage:        req.GetProjectLanguage(),
		ProjectLanguageVersion: req.GetProjectLanguageVersion(),
		AppName:                req.GetProjectAppName(),
		ServerName:             req.GetProjectServerName(),
	}
	return result
}
