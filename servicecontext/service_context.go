package servicecontext

import (
	"context"
	"log/slog"
	"sync"

	"github.com/KitHub/project_generator/config"
	"github.com/KitHub/project_generator/logic"
	"github.com/KitHub/project_generator/service"
)

type ServiceContext struct {
	ShutdownLogic  *logic.ShutdownLogic
	ProjectLogic   *logic.ProjectLogic
	ProjectService *service.ProjectService
}

var gServiceCtx *ServiceContext
var once sync.Once

func InitServiceContext(ctx context.Context, configEntity *config.ConfigEntity) (
	serviceCtx *ServiceContext, err error) {
	slog.InfoContext(ctx, "init service context")

	shutdownLogic := logic.NewShutdownLogic()
	projectLogic := logic.NewProjectLogic()
	projectService := service.NewProjectService(projectLogic)

	once.Do(func() {
		gServiceCtx = &ServiceContext{
			ShutdownLogic:  shutdownLogic,
			ProjectLogic:   projectLogic,
			ProjectService: projectService,
		}
	})

	slog.InfoContext(ctx, "init service context done")
	return gServiceCtx, err
}

func GetServiceContext() *ServiceContext {
	return gServiceCtx
}
