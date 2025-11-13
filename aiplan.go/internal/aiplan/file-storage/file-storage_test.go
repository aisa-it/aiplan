package filestorage

import (
	"testing"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/config"
	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/assert"
)

func TestGetChecksum(t *testing.T) {
	cfg := config.ReadConfig()

	s, err := NewMinioStorage(cfg.AWSEndpoint, cfg.AWSAccessKey, cfg.AWSSecretKey, false, cfg.AWSBucketName)
	assert.NoError(t, err)

	_, err = s.GetFileInfo(uuid.Must(uuid.FromString("d5b1a9d6-27bd-4ab4-9db4-65798b9ef115")))
	assert.NoError(t, err)
}
