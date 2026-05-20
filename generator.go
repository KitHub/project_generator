package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type ProjectConfig struct {
	ProjectName  string
	ModuleName   string
	WebFramework string // gin / echo / native
	DBType       string // mysql / postgres / none
	UseGorm      bool
	UseViper     bool
	UseZapLog    bool
}

var cfg ProjectConfig

func main() {
	fmt.Println("===== Go project generator =====")
	scanInput("Input project name", &cfg.ProjectName)
	scanInput("Input go module name (e.g., github.com/xxx/xxx)", &cfg.ModuleName)

	selectOption("Please select Web framework", []string{"gin", "echo", "native"}, &cfg.WebFramework)
	selectOption("Please select database type", []string{"mysql", "postgres", "none"}, &cfg.DBType)

	cfg.UseGorm = cfg.DBType != "none"
	cfg.UseViper = true
	cfg.UseZapLog = true

	// start generating project
	genProject()
	fmt.Printf("\n[%s] project generated successfully!\n", cfg.ProjectName)
}

func scanInput(desc string, val *string) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print(desc + ": ")
	res, _ := reader.ReadString('\n')
	*val = strings.TrimSpace(res)
}

func selectOption(desc string, opts []string, res *string) {
	fmt.Println("\n" + desc)
	for i, v := range opts {
		fmt.Printf("  %d. %s\n", i+1, v)
	}
	fmt.Print("Please enter the number: ")
	var idx int
	fmt.Scan(&idx)
	*res = opts[idx-1]
}

// dir structure
var dirs = []string{
	"cmd/api",
	"internal/handler",
	"internal/service",
	"internal/repository",
	"internal/model",
	"internal/global",
	"config",
	"pkg/logger",
	"pkg/utils",
	"scripts",
}

func genProject() {
	root := cfg.ProjectName
	// create dirs
	for _, d := range dirs {
		_ = os.MkdirAll(root+"/"+d, 0755)
	}

	// generate go.mod
	writeFile(root+"/go.mod", fmt.Sprintf("module %s\n\ngo 1.21\n", cfg.ModuleName))

	// generate global package
	genGlobal(root)
	// generate config file
	genConfig(root)
	// generate logger
	genLogger(root)
	// generate database connection
	genDB(root)
	// generate main entry
	genMain(root)
	// generate routes
	genRouter(root)
	// generate basic demo
	genDemoHandler(root)
	// generate helper files
	genExtra(root)
}

func writeFile(path, content string) {
	_ = os.WriteFile(path, []byte(content), 0644)
}

func genGlobal(root string) {
	code := `package global

import (
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var (
	Config *ConfigStruct
	Log    *zap.Logger
	DB     *gorm.DB
	Vp     *viper.Viper
)

type ConfigStruct struct {
	Server ServerConfig ` + "`" + `yaml:"server"` + "`" + `
	DB     DBConfig     ` + "`" + `yaml:"db"` + "`" + `
}

type ServerConfig struct {
	Port string ` + "`" + `yaml:"port"` + "`" + `
}

type DBConfig struct {
	DSN  string ` + "`" + `yaml:"dsn"` + "`" + `
	Type string ` + "`" + `yaml:"type"` + "`" + `
}
`
	writeFile(root+"/internal/global/global.go", code)
}

func genConfig(root string) {
	// yaml content
	yaml := `server:
  port: ":8080"
db:
  dsn: "root:123456@tcp(127.0.0.1:3306)/test?charset=utf8mb4&parseTime=True&loc=Local"
  type: "mysql"
`
	writeFile(root+"/config/config.yaml", yaml)

	// load config
	code := `package global

import (
	"fmt"
	"github.com/spf13/viper"
)

func InitConfig() error {
	vp := viper.New()
	vp.SetConfigFile("config/config.yaml")
	if err := vp.ReadInConfig(); err != nil {
		return err
	}
	var c ConfigStruct
	if err := vp.Unmarshal(&c); err != nil {
		return err
	}
	Config = &c
	Vp = vp
	fmt.Println("Configuration file loaded successfully")
	return nil
}
`
	writeFile(root+"/internal/global/config.go", code)
}

func genLogger(root string) {
	code := `package global

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func InitLogger() {
	conf := zap.NewDevelopmentConfig()
	conf.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05")
	logger, _ := conf.Build()
	zap.ReplaceGlobals(logger)
	Log = logger
	Log.Info("Logger initialized successfully")
}
`
	writeFile(root+"/internal/global/logger.go", code)
}

func genDB(root string) {
	if cfg.DBType == "none" {
		writeFile(root+"/internal/global/db.go", `package global

// Database not enabled
func InitDB() error {
	return nil
}`)
		return
	}

	var dialector string
	if cfg.DBType == "mysql" {
		dialector = `mysql.Open(Config.DB.DSN)`
	} else {
		dialector = `postgres.Open(Config.DB.DSN)`
	}

	importStr := `"gorm.io/driver/` + cfg.DBType + `"`
	code := fmt.Sprintf(`package global

import (
	%s
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func InitDB() error {
	db, err := gorm.Open(%s, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return err
	}
	DB = db
	Log.Info("Database connection established")
	return nil
}
`, importStr, dialector)
	writeFile(root+"/internal/global/db.go", code)
}

func genRouter(root string) {
	if cfg.WebFramework == "gin" {
		code := `package handler

import (
	"github.com/gin-gonic/gin"
)

func RegisterRouter(r *gin.Engine) {
	r.GET("/ping", Ping)
}
`
		writeFile(root+"/internal/handler/router.go", code)
	} else if cfg.WebFramework == "echo" {
		code := `package handler

import "github.com/labstack/echo/v4"

func RegisterRouter(e *echo.Echo) {
	e.GET("/ping", Ping)
}
`
		writeFile(root+"/internal/handler/router.go", code)
	} else {
		code := `package handler

import "net/http"

func RegisterRouter() {
	http.HandleFunc("/ping", Ping)
}
`
		writeFile(root+"/internal/handler/router.go", code)
	}
}

func genDemoHandler(root string) {
	if cfg.WebFramework == "gin" {
		code := `package handler

import "github.com/gin-gonic/gin"

func Ping(c *gin.Context) {
	c.JSON(200, gin.H{
		"msg": "pong",
	})
}
`
		writeFile(root+"/internal/handler/demo.go", code)
	} else if cfg.WebFramework == "echo" {
		code := `package handler

import "github.com/labstack/echo/v4"

func Ping(c echo.Context) error {
	return c.JSON(200, map[string]string{"msg":"pong"})
}
`
		writeFile(root+"/internal/handler/demo.go", code)
	} else {
		code := `package handler

import (
	"net/http"
	"encoding/json"
)

func Ping(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]string{"msg":"pong"})
}
`
		writeFile(root+"/internal/handler/demo.go", code)
	}
}

func genMain(root string) {
	var code string

	if cfg.WebFramework == "gin" {
		code = `package main

import (
	"` + cfg.ModuleName + `/internal/global"
	"` + cfg.ModuleName + `/internal/handler"
	"github.com/gin-gonic/gin"
	"log"
)

func main() {
	// init
	global.InitLogger()
	if err := global.InitConfig(); err != nil {
		log.Fatal(err)
	}
	if err := global.InitDB(); err != nil {
		log.Fatal(err)
	}

	r := gin.Default()
	handler.RegisterRouter(r)
	global.Log.Info("Service started successfully, listening on port " + global.Config.Server.Port)
	log.Fatal(r.Run(global.Config.Server.Port))
}
`
	} else if cfg.WebFramework == "echo" {
		code = `package main

import (
	"` + cfg.ModuleName + `/internal/global"
	"` + cfg.ModuleName + `/internal/handler"
	"github.com/labstack/echo/v4"
	"log"
)

func main() {
	global.InitLogger()
	if err := global.InitConfig(); err != nil {
		log.Fatal(err)
	}
	if err := global.InitDB(); err != nil {
		log.Fatal(err)
	}

	e := echo.New()
	handler.RegisterRouter(e)
	global.Log.Info("Service started successfully, listening on port " + global.Config.Server.Port)
	log.Fatal(e.Start(global.Config.Server.Port))
}
`
	} else {
		code = `package main

import (
	"` + cfg.ModuleName + `/internal/global"
	"` + cfg.ModuleName + `/internal/handler"
	"log"
	"net/http"
)

func main() {
	global.InitLogger()
	if err := global.InitConfig(); err != nil {
		log.Fatal(err)
	}
	if err := global.InitDB(); err != nil {
		log.Fatal(err)
	}

	handler.RegisterRouter()
	global.Log.Info("Service started successfully, listening on port " + global.Config.Server.Port)
	log.Fatal(http.ListenAndServe(global.Config.Server.Port, nil))
}
`
	}

	writeFile(root+"/cmd/api/main.go", code)
}

func genExtra(root string) {
	// gitignore
	gitignore := `*.log
.DS_Store
.idea/
.vscode/
bin/
tmp/
dist/
*.env
`
	writeFile(root+"/.gitignore", gitignore)

	// Makefile
	makefile := `.PHONY: run build clean

run:
	go run ./cmd/api

build:
	go build -o bin/app ./cmd/api

clean:
	rm -rf bin
`
	writeFile(root+"/Makefile", makefile)

	// readme
	readme := `# ` + cfg.ProjectName + `
Automatically generated Go layered architecture project

## Run
make run

## Build
make build
`
	writeFile(root+"/README.md", readme)
}
