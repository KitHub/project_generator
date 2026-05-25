package logic

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
	"text/template"
)

type TemplateRenderLogic interface {
	Render(ctx context.Context, templateFilePath string, data map[string]any) (string, error)
}

type templateRenderLogicImpl struct {
}

// Render implements [TemplateRenderLogic].
func (t *templateRenderLogicImpl) Render(ctx context.Context, templateFilePath string, data map[string]any) (string, error) {
	slog.InfoContext(ctx, "Rendering template", slog.String("templateFilePath", templateFilePath), slog.Any("data", data))
	tpl, err := template.ParseFiles(templateFilePath)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to parse template file", slog.String("templateFilePath", templateFilePath), slog.String("error", err.Error()))
		return "", err
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		slog.ErrorContext(ctx, "Failed to execute template", slog.String("templateFilePath", templateFilePath), slog.String("error", err.Error()))
		return "", err
	}
	slog.InfoContext(ctx, "Template rendered successfully", slog.String("templateFilePath", templateFilePath))
	return buf.String(), nil
}

var templateRenderLogicInstance TemplateRenderLogic
var onceRenderLogicInit sync.Once

func NewTemplateRenderLogic() TemplateRenderLogic {
	onceRenderLogicInit.Do(func() {
		templateRenderLogicInstance = &templateRenderLogicImpl{}
	})
	return templateRenderLogicInstance
}
