package entity

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
	ProjectCvsUrl          string                          `json:"project_cvs_url"`          // project cvs url
	AppName                string                          `json:"app_name"`                 // project name
	ServerName             string                          `json:"server_name"`              // server name
	Components             []GenerateProjectParamComponent `json:"components"`               // project components
}

type GenerateProjectResult struct {
	ProjectFilesDir string `json:"project_files_dir"` // generated project files directory
}
