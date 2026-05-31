package servicecontext

import (
	"context"
	"log/slog"
	"sync"

	"github.com/KitHub/project_generator/config"
	"github.com/KitHub/project_generator/logic"
)

type ServiceContext struct {
	ShutdownLogic logic.ShutdownLogic
	RenderLogic   logic.TemplateRenderLogic
	ProjectLogic  logic.ProjectLogic
}

var gServiceCtx *ServiceContext
var once sync.Once

func InitServiceContext(ctx context.Context, configEntity *config.ConfigEntity) (
	serviceCtx *ServiceContext, err error) {
	slog.InfoContext(ctx, "init service context")
	once.Do(func() {

		gServiceCtx = &ServiceContext{
			ShutdownLogic: logic.NewShutdownLogic(),
			RenderLogic:   logic.NewTemplateRenderLogic(),
			ProjectLogic:  logic.NewProjectLogic(),
		}
	})
	if err != nil {
		slog.ErrorContext(ctx, "init service context failed", slog.Any("error", err))
		return nil, err

	}
	slog.InfoContext(ctx, "init service context done")
	return gServiceCtx, err

}
