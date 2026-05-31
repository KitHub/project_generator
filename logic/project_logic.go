package logic

import "sync"

var projectLogic ProjectLogic
var onceProjectLogic sync.Once

type GenerateProjectParamComponent struct {
	ComponentId      string `json:"component_id"`      // component id
	ComponentName    string `json:"component_name"`    // component name
	ComponentVersion string `json:"component_version"` // component version
}

type GenerateProjectParam struct {
	ProjectName            string                          `json:"project_name"`             // project name
	ProjectDesc            string                          `json:"project_desc"`             // project description
	ProjectLanguage        string                          `json:"project_language"`         // project language
	ProjectLanguageVersion string                          `json:"project_language_version"` // project language version
	AppName                string                          `json:"app_name"`                 // project name
	ServerName             string                          `json:"server_name"`              // server name
	Components             []GenerateProjectParamComponent `json:"components"`               // project components
}

type GenerateProjectResult struct {
	ProjectFilesDir string `json:"project_files_dir"` // generated project files directory
}

type ProjectLogic interface {
	GenerateProject(req GenerateProjectParam) (GenerateProjectResult, error)
}

type projectLogicImpl struct {
}

// GenerateProject implements [ProjectLogic].
func (p *projectLogicImpl) GenerateProject(req GenerateProjectParam) (GenerateProjectResult, error) {
	panic("unimplemented")
}

func NewProjectLogic() ProjectLogic {
	onceProjectLogic.Do(func() {
		projectLogic = &projectLogicImpl{}
	})
	return projectLogic
}
