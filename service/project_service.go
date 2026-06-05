package service

import (
	"archive/zip"
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/KitHub/project_generator/config"
	"github.com/KitHub/project_generator/entity"
	"github.com/KitHub/project_generator/logic"
	"github.com/KitHub/protocols/projectgeneratorapi"
	"github.com/google/uuid"
	"google.golang.org/genproto/googleapis/api/httpbody"
	"google.golang.org/grpc"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var projectService *ProjectService
var onceProjectService sync.Once

type ProjectService struct {
	projectgeneratorapi.UnimplementedProjectGeneratorAPIServer
	ProjectLogic    *logic.ProjectLogic
	generateSeqsMap *sync.Map // map[seq]projectDir, used for downloading generated project files
}

// DownloadGeneratedProject implements [projectgeneratorapi.ProjectGeneratorAPIServer].
func (p *ProjectService) DownloadGeneratedProject(ctx context.Context, req *projectgeneratorapi.DownloadGeneratedProjectRequest) (*httpbody.HttpBody, error) {
	slog.InfoContext(ctx, "received download generated project request", slog.Any("request", req.String()))
	if req.Validate() != nil {
		errMsg := "invalid request parameters: " + req.Validate().Error()
		slog.ErrorContext(ctx, errMsg, slog.Any("request", req.String()))
		return nil, status.Errorf(codes.InvalidArgument, "invalid request parameters")
	}

	value, ok := p.generateSeqsMap.Load(req.GetSeq())
	if !ok {
		errMsg := "invalid seq, no generated project found, maybe deleted before, please generate project again"
		slog.ErrorContext(ctx, errMsg, slog.String("seq", req.GetSeq()))
		return nil, status.Errorf(codes.Internal, "server error: %s", errMsg)
	}

	projectFilesDirStr, ok := value.(string)
	if !ok {
		errMsg := "invalid project files directory"
		slog.ErrorContext(ctx, errMsg, slog.String("seq", req.GetSeq()), slog.Any("projectFilesDir", value))
		return nil, status.Errorf(codes.Internal, "server error")
	}

	content, err := readProjectFilesDir(ctx, projectFilesDirStr)
	if err != nil {
		errMsg := "failed to read project files directory"
		slog.ErrorContext(ctx, errMsg, slog.String("seq", req.GetSeq()), slog.Any("projectFilesDirStr", projectFilesDirStr), slog.Any("error", err))
		return nil, status.Errorf(codes.Internal, "server error")
	}

	md := metadata.Pairs("download-filename", req.GetSeq()+".zip")
	err = grpc.SendHeader(ctx, md)
	if err != nil {
		slog.ErrorContext(ctx, "failed to send header with filename metadata", slog.String("seq", req.GetSeq()), slog.Any("error", err))
		return nil, status.Errorf(codes.Internal, "server error")
	}

	rsp := &httpbody.HttpBody{
		ContentType: "application/zip",
		Data:        content,
	}

	slog.InfoContext(ctx, "project files read successfully", slog.String("seq", req.GetSeq()), slog.Any("projectFilesDirStr", projectFilesDirStr))
	return rsp, nil
}

// GenerateProject implements [projectgeneratorapi.ProjectGeneratorAPIServer].
func (p *ProjectService) GenerateProject(ctx context.Context, req *projectgeneratorapi.GenerateProjectRequest) (*projectgeneratorapi.GenerateProjectResponse, error) {
	seq := strings.ReplaceAll(uuid.New().String(), "-", "")
	slog.InfoContext(ctx, "received generate project request", slog.String("seq", seq), slog.Any("request", req.String()))

	if req.Validate() != nil {
		errMsg := "invalid request parameters: " + req.Validate().Error()
		slog.ErrorContext(ctx, errMsg, slog.Any("request", req.String()))
		return nil, status.Errorf(codes.InvalidArgument, "invalid request parameters")
	}

	param := composeProjectServiceParam(ctx, req)

	result, err := p.ProjectLogic.GenerateProject(ctx, param, config.GetConfig(ctx).Templates.TemplatesDir)
	if err != nil {
		slog.ErrorContext(ctx, "failed to generate project", slog.Any("error", err))
		return nil, err
	}

	rsp := &projectgeneratorapi.GenerateProjectResponse{
		ErrCode: 0,
		ErrMsg:  "ok",
		Data: &projectgeneratorapi.GenerateProjectResponseData{
			Seq: seq,
		},
	}
	p.generateSeqsMap.Store(seq, result.ProjectFilesDir)

	slog.InfoContext(ctx, "project generated successfully", slog.Any("result", result), slog.String("seq", seq))
	return rsp, nil
}

func NewProjectService(projectLogic *logic.ProjectLogic) *ProjectService {
	onceProjectService.Do(func() {
		projectService = &ProjectService{
			ProjectLogic:    projectLogic,
			generateSeqsMap: &sync.Map{},
		}

		go func() {
			// clear generated project files every hour, the max lifetime of generated project files is 24 hours
			maxLifetime := int64(24 * 3600)
			ticker := time.NewTicker(1 * time.Hour)
			for range ticker.C {
				slog.Info("start to clear generated project files")
				clearGeneratedProjectFiles(context.Background(), maxLifetime)
			}
		}()

	})
	return projectService
}

func composeProjectServiceParam(ctx context.Context, req *projectgeneratorapi.GenerateProjectRequest) entity.GenerateProjectParam {
	result := entity.GenerateProjectParam{
		ProjectName:            req.GetProjectName(),
		ProjectDesc:            req.GetProjectDescription(),
		ProjectLanguage:        req.GetProjectLanguage(),
		ProjectLanguageVersion: req.GetProjectLanguageVersion(),
		ProjectCvsUrl:          req.GetProjectCvsUrl(),
		AppName:                req.GetProjectAppName(),
		ServerName:             req.GetProjectServerName(),
	}
	return result
}

func readProjectFilesDir(ctx context.Context, projectFilesDir string) ([]byte, error) {
	slog.InfoContext(ctx, "start to read project files directory", slog.String("projectFilesDir", projectFilesDir))

	zipBuf := &bytes.Buffer{}
	zw := zip.NewWriter(zipBuf)

	err := filepath.Walk(projectFilesDir, func(path string, info os.FileInfo, err error) error {
		slog.DebugContext(ctx, "walking project files directory", slog.String("projectFilesDir", projectFilesDir), slog.String("currentPath", path))

		if err != nil {
			slog.ErrorContext(ctx, "failed to walk project files directory", slog.String("projectFilesDir", projectFilesDir), slog.Any("error", err))
			return err
		}

		relPath, err := filepath.Rel(projectFilesDir, path)
		if err != nil {
			slog.ErrorContext(ctx, "failed to get relative path for directory", slog.String("projectFilesDir", projectFilesDir), slog.String("path", path), slog.Any("error", err))
			return err
		}
		if relPath == "." {
			return nil
		}

		if info.IsDir() {
			_, err = zw.Create(relPath + string(os.PathSeparator))
			if err != nil {
				slog.ErrorContext(ctx, "failed to create directory entry in zip", slog.String("projectFilesDir", projectFilesDir), slog.String("relPath", relPath), slog.Any("error", err))
				return err
			}
			return nil
		}

		f, err := zw.Create(relPath)
		if err != nil {
			slog.ErrorContext(ctx, "failed to create file in zip", slog.String("projectFilesDir", projectFilesDir), slog.String("relPath", relPath), slog.Any("error", err))
			return err
		}

		b, err := os.ReadFile(path)
		if err != nil {
			slog.ErrorContext(ctx, "failed to read file", slog.String("projectFilesDir", projectFilesDir), slog.String("path", path), slog.Any("error", err))
			return err
		}
		_, err = f.Write(b)
		if err != nil {
			slog.ErrorContext(ctx, "failed to write file in zip", slog.String("projectFilesDir", projectFilesDir), slog.String("relPath", relPath), slog.Any("error", err))
			return err
		}
		return nil
	})

	if err != nil {
		slog.ErrorContext(ctx, "failed to zip project files directory", slog.String("projectFilesDir", projectFilesDir), slog.Any("error", err))
		return nil, err
	}

	err = zw.Close()
	if err != nil {
		slog.ErrorContext(ctx, "failed to close zip writer", slog.String("projectFilesDir", projectFilesDir), slog.Any("error", err))
	}

	slog.InfoContext(ctx, "project files directory read and zipped successfully", slog.String("projectFilesDir", projectFilesDir))
	return zipBuf.Bytes(), nil
}

func deleteProjectFilesByDir(ctx context.Context, projectFilesDir string) error {
	err := os.RemoveAll(projectFilesDir)
	if err != nil {
		slog.ErrorContext(ctx, "failed to delete project files directory", slog.String("projectFilesDir", projectFilesDir), slog.Any("error", err))
		return err
	}
	slog.InfoContext(ctx, "project files directory deleted successfully", slog.String("projectFilesDir", projectFilesDir))
	return nil
}

func clearGeneratedProjectFiles(ctx context.Context, maxLifetime int64) {
	clearSeq := strings.ReplaceAll(uuid.New().String(), "-", "")
	slog.InfoContext(ctx, "start to clear generated project files", slog.String("clear_seq", clearSeq))

	// iterate generateSeqsMap and delete project files, then delete the entry in the map
	projectService.generateSeqsMap.Range(func(key, value any) bool {
		seq, ok := key.(string)
		if !ok {
			slog.ErrorContext(ctx, "invalid seq in generateSeqsMap", slog.Any("key", key))
			return true
		}

		projectFilesDir, ok := value.(string)
		if !ok {
			slog.ErrorContext(ctx, "invalid project files directory in generateSeqsMap", slog.String("seq", seq), slog.Any("value", value))
			return true
		}

		ostat, err := os.Stat(projectFilesDir)
		if err != nil {
			slog.ErrorContext(ctx, "failed to stat project files directory", slog.String("seq", seq), slog.String("projectFilesDir", projectFilesDir), slog.Any("error", err))
			return true
		}
		if ostat.IsDir() {
			modTime := ostat.ModTime().Unix()
			currentTime := time.Now().Unix()
			if currentTime-modTime < maxLifetime {
				// not expired yet, skip
				return true
			}
		} else {
			// not a directory, skip
			slog.ErrorContext(ctx, "project files path is not a directory", slog.String("seq", seq), slog.String("projectFilesDir", projectFilesDir))
			return true
		}

		// delete project files by projectFilesDir
		err = deleteProjectFilesByDir(ctx, projectFilesDir)
		if err != nil {
			slog.ErrorContext(ctx, "failed to delete project files", slog.String("seq", seq), slog.String("projectFilesDir", projectFilesDir), slog.Any("error", err))
			return true
		}

		// delete entry in the map
		projectService.generateSeqsMap.Delete(seq)
		slog.InfoContext(ctx, "generated project files cleared successfully", slog.String("seq", seq), slog.String("projectFilesDir", projectFilesDir))
		return true
	})

	slog.InfoContext(ctx, "clear generated project files done", slog.String("clear_seq", clearSeq))
}
