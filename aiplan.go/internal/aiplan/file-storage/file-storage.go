// Пакет предоставляет интерфейс и реализации для работы с файловым хранилищем, включая локальное хранилище и Minio. Он обеспечивает операции сохранения, загрузки, удаления и копирования файлов, а также поддержку метаданных.
package filestorage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/exp/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/config"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3config "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/tus/tusd/v2/pkg/s3store"

	tusd "github.com/tus/tusd/v2/pkg/handler"
)

const (
	UploadTries = 20
)

type Metadata struct {
	WorkspaceId string
	DocId       string
	FormId      string
	ProjectId   string
	IssueId     string
	CommentId   string
}

type FileInfo struct {
	Name        string
	Size        int64
	ContentType string
	CreatedAt   time.Time
}

func (m Metadata) GetMap() map[string]string {
	meta := make(map[string]string)
	if m.WorkspaceId != "" {
		meta["workspaceId"] = m.WorkspaceId
	}
	if m.ProjectId != "" {
		meta["projectId"] = m.ProjectId
	}
	if m.IssueId != "" {
		meta["issueId"] = m.IssueId
	}
	if m.CommentId != "" {
		meta["commentId"] = m.CommentId
	}
	return meta
}

type FileStorage interface {
	GetTUSHandler(
		cfg *config.Config,
		baseUrl string,
		uploadValidator func(hook tusd.HookEvent) (tusd.HTTPResponse, tusd.FileInfoChanges, error),
		postUploadHook func(event tusd.HookEvent)) echo.HandlerFunc
	Save(data []byte, name uuid.UUID, contentType string, metadata *Metadata) error
	SaveReader(reader io.Reader, fileSize int64, name uuid.UUID, contentType string, metadata *Metadata) error
	SaveReaderWithBuf(reader io.Reader, fileSize int64, name uuid.UUID, contentType string, metadata *Metadata) error
	Load(name uuid.UUID) ([]byte, error)
	LoadReader(name uuid.UUID) (io.ReadCloser, error)
	Delete(name uuid.UUID) error
	CopyOld(name string, newName uuid.UUID, newMeta *Metadata) error
	Exist(name uuid.UUID) (bool, error)
	ListRoot(fn func(FileInfo) error) error
	Move(old string, new string) error
	GetFileInfo(name uuid.UUID) (*FileInfo, error)
}

type LocalStorage struct {
	rootDir string
}

func (s *LocalStorage) GetTUSHandler(cfg *config.Config, baseUrl string, uploadValidator func(hook tusd.HookEvent) (tusd.HTTPResponse, tusd.FileInfoChanges, error),
	postUploadHook func(event tusd.HookEvent)) echo.HandlerFunc {
	return nil
}

func (s *LocalStorage) Save(data []byte, name uuid.UUID, contentType string, metadata *Metadata) error {
	return os.WriteFile(filepath.Join(s.rootDir, name.String()), data, 0644)
}

func (s *LocalStorage) SaveReader(reader io.Reader, fileSize int64, name uuid.UUID, contentType string, metadata *Metadata) error {
	f, err := os.Create(name.String())
	if err != nil {
		return err
	}
	_, err = io.Copy(f, reader)
	return err
}

func (s *LocalStorage) SaveReaderWithBuf(reader io.Reader, fileSize int64, name uuid.UUID, contentType string, metadata *Metadata) error {
	return s.SaveReader(reader, fileSize, name, contentType, metadata)
}

func (s *LocalStorage) Load(name uuid.UUID) ([]byte, error) {
	return os.ReadFile(filepath.Join(s.rootDir, name.String()))
}

func (s *LocalStorage) LoadReader(name uuid.UUID) (io.ReadCloser, error) {
	return os.Open(name.String())
}

func (s *LocalStorage) Delete(name uuid.UUID) error {
	return os.Remove(name.String())
}

func (s *LocalStorage) Exist(name uuid.UUID) (bool, error) {
	return true, nil
}

func (s *LocalStorage) CopyOld(name string, newName uuid.UUID, newMeta *Metadata) error {
	return nil
}

func (s *LocalStorage) ListRoot(fn func(FileInfo) error) error {
	return nil
}

func (s *LocalStorage) Move(old string, new string) error {
	return nil
}

func (s *LocalStorage) GetFileInfo(name uuid.UUID) (*FileInfo, error) {
	return nil, nil
}

func NewLocalStorage(rootPath string) (FileStorage, error) {
	return &LocalStorage{rootPath}, nil
}

type MinioStorage struct {
	client     *minio.Client
	s3client   *s3.Client
	bucketName string
}

func (s *MinioStorage) GetTUSHandler(
	cfg *config.Config,
	baseUrl string,
	uploadValidator func(hook tusd.HookEvent) (tusd.HTTPResponse, tusd.FileInfoChanges, error),
	postUploadHook func(event tusd.HookEvent),
) echo.HandlerFunc {
	store := s3store.New(s.bucketName, s.s3client)
	composer := tusd.NewStoreComposer()
	store.UseIn(composer)

	basePath, _ := url.Parse(baseUrl)
	handler, err := tusd.NewHandler(tusd.Config{
		BasePath:                cfg.WebURL.ResolveReference(basePath).String(),
		StoreComposer:           composer,
		DisableDownload:         true,
		NotifyCompleteUploads:   true,
		PreUploadCreateCallback: uploadValidator,
		Logger:                  slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		fmt.Println(err)
	}

	go func() {
		for {
			event := <-handler.CompleteUploads
			postUploadHook(event)
		}
	}()

	return echo.WrapHandler(http.StripPrefix(basePath.String(), handler))
}

func (s *MinioStorage) Save(data []byte, name uuid.UUID, contentType string, metadata *Metadata) error {
	putOptions := minio.PutObjectOptions{ContentType: contentType}
	if metadata != nil {
		putOptions.UserTags = metadata.GetMap()
	}

	var err error
	for i := range UploadTries {
		_, err = s.client.PutObject(context.Background(),
			s.bucketName,
			name.String(),
			bytes.NewReader(data),
			int64(len(data)),
			putOptions,
		)
		if err != nil {
			resp := minio.ToErrorResponse(err)
			slog.Error("Upload file to minio", "try", i+1, "code", resp.StatusCode, "msg", resp.Message)
			time.Sleep(time.Second * 20)
			continue
		}
		break
	}
	return err
}

func (s *MinioStorage) SaveReader(reader io.Reader, fileSize int64, name uuid.UUID, contentType string, metadata *Metadata) error {
	putOptions := minio.PutObjectOptions{ContentType: contentType}
	if metadata != nil {
		putOptions.UserTags = metadata.GetMap()
	}

	var err error
	for i := range UploadTries {
		_, err = s.client.PutObject(context.Background(),
			s.bucketName,
			name.String(),
			reader,
			fileSize,
			putOptions,
		)
		if err != nil {
			resp := minio.ToErrorResponse(err)
			slog.Error("Upload file to minio", "name", name, "try", i+1, "code", resp.StatusCode, "msg", resp.Message, "err", err)
			time.Sleep(time.Minute)
			continue
		}
		break
	}
	return err
}

func (s *MinioStorage) SaveReaderWithBuf(reader io.Reader, fileSize int64, name uuid.UUID, contentType string, metadata *Metadata) error {
	tmp, err := os.CreateTemp("", name.String()+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := io.Copy(tmp, reader); err != nil {
		return err
	}
	tmp.Seek(0, io.SeekStart)

	return s.SaveReader(tmp, fileSize, name, contentType, metadata)
}

func (s *MinioStorage) Load(name uuid.UUID) ([]byte, error) {
	obj, err := s.LoadReader(name)
	if err != nil {
		return nil, err
	}
	defer obj.Close()

	return io.ReadAll(obj)
}

func (s *MinioStorage) LoadReader(name uuid.UUID) (io.ReadCloser, error) {
	return s.client.GetObject(context.Background(),
		s.bucketName,
		name.String(),
		minio.GetObjectOptions{},
	)
}

func (s *MinioStorage) Delete(name uuid.UUID) error {
	return s.client.RemoveObject(
		context.Background(),
		s.bucketName,
		name.String(),
		minio.RemoveObjectOptions{},
	)
}

func (s *MinioStorage) CopyOld(name string, newName uuid.UUID, newMeta *Metadata) error {
	dstOptions := minio.CopyDestOptions{
		Bucket: s.bucketName,
		Object: newName.String(),
	}
	if newMeta != nil {
		dstOptions.UserTags = newMeta.GetMap()
	}
	_, err := s.client.CopyObject(context.Background(),
		dstOptions,
		minio.CopySrcOptions{
			Bucket: s.bucketName,
			Object: name,
		})
	return err
}

func (s *MinioStorage) Exist(name uuid.UUID) (bool, error) {
	_, err := s.client.StatObject(
		context.Background(),
		s.bucketName,
		name.String(),
		minio.StatObjectOptions{},
	)
	if err != nil {
		errResponse := minio.ToErrorResponse(err)
		if errResponse.Code == "NoSuchKey" {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *MinioStorage) ListRoot(fn func(info FileInfo) error) error {
	for obj := range s.client.ListObjects(context.Background(), s.bucketName, minio.ListObjectsOptions{Recursive: true}) {
		if err := fn(FileInfo{
			Name:        obj.Key,
			Size:        obj.Size,
			ContentType: obj.ContentType,
			CreatedAt:   obj.LastModified,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *MinioStorage) Move(old string, new string) error {
	if _, err := s.client.CopyObject(context.Background(),
		minio.CopyDestOptions{
			Bucket: s.bucketName,
			Object: new,
		},
		minio.CopySrcOptions{
			Bucket: s.bucketName,
			Object: old,
		},
	); err != nil {
		return err
	}
	return s.client.RemoveObject(context.Background(), s.bucketName, old, minio.RemoveObjectOptions{})
}

func (s *MinioStorage) GetFileInfo(name uuid.UUID) (*FileInfo, error) {
	stat, err := s.client.StatObject(context.Background(), s.bucketName, name.String(), minio.StatObjectOptions{})
	if err != nil {
		return nil, err
	}

	return &FileInfo{
		Name:        name.String(),
		Size:        stat.Size,
		ContentType: stat.ContentType,
		CreatedAt:   stat.LastModified,
	}, nil
}

func NewMinioStorage(endpoint string, accessKeyID string, secretAccessKey string, useSSL bool, bucketName string) (FileStorage, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, err
	}

	s3cfg, _ := s3config.LoadDefaultConfig(context.Background())

	s3client := s3.NewFromConfig(s3cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String("http://" + endpoint)
		o.Region = "ru"
		o.UsePathStyle = true
	})

	exists, err := client.BucketExists(context.Background(), bucketName)
	if err != nil {
		return nil, err
	}

	if !exists {
		// Create bucket if not exist
		if err := client.MakeBucket(context.Background(), bucketName, minio.MakeBucketOptions{}); err != nil {
			return nil, err
		}
	}

	return &MinioStorage{client, s3client, bucketName}, nil
}
