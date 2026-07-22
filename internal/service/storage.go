package service

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type StorageClient struct {
	client *minio.Client
	bucket string
}

func NewStorageClient(endpoint, accessKey, secretKey, bucket string) (*StorageClient, error) {
	// minio.New expects host:port, not a full URL
	ep := strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
	secure := strings.HasPrefix(endpoint, "https://")
	client, err := minio.New(ep, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: secure,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}

	// Ensure bucket exists
	ctx := context.Background()
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("check bucket: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("create bucket: %w", err)
		}
		slog.Info("created bucket", "bucket", bucket)
	}

	return &StorageClient{client: client, bucket: bucket}, nil
}

func (s *StorageClient) Upload(ctx context.Context, key string, data []byte, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	return err
}


