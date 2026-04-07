package service

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	appconfig "github.io/webpoint-solutions-llc/uploader/internal/config"
	apptypes "github.io/webpoint-solutions-llc/uploader/internal/types"
)

type S3Service struct {
	client    *s3.Client
	presigner *s3.PresignClient
	bucket    string
	partSize  int64
}

func NewS3Service(cfg *appconfig.Config) (*S3Service, error) {
	// Build MinIO endpoint URL
	protocol := "http"
	if cfg.MinioUseSSL {
		protocol = "https"
	}
	endpoint := fmt.Sprintf("%s://%s", protocol, cfg.MinioEndpoint)

	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.MinioRegion),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.MinioAccessKey,
			cfg.MinioSecretKey,
			"",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client with MinIO-specific options
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true // Required for MinIO
	})

	return &S3Service{
		client:    client,
		presigner: s3.NewPresignClient(client),
		bucket:    cfg.MinioBucket,
		partSize:  cfg.PartSize,
	}, nil
}

func (s *S3Service) InitiateUpload(ctx context.Context, key, contentType string) (string, error) {
	input := &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}

	result, err := s.client.CreateMultipartUpload(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to initiate multipart upload: %w", err)
	}

	return *result.UploadId, nil
}

func (s *S3Service) SignPart(ctx context.Context, key, uploadID string, partNumber int) (string, error) {
	input := &s3.UploadPartInput{
		Bucket:     aws.String(s.bucket),
		Key:        aws.String(key),
		UploadId:   aws.String(uploadID),
		PartNumber: aws.Int32(int32(partNumber)),
	}

	presignedReq, err := s.presigner.PresignUploadPart(ctx, input, func(opts *s3.PresignOptions) {
		opts.Expires = 15 * time.Minute
	})
	if err != nil {
		return "", fmt.Errorf("failed to presign upload part: %w", err)
	}

	return presignedReq.URL, nil
}

func (s *S3Service) ListParts(ctx context.Context, key, uploadID string) ([]apptypes.PartInfo, error) {
	var parts []apptypes.PartInfo
	var partMarker *string

	for {
		input := &s3.ListPartsInput{
			Bucket:           aws.String(s.bucket),
			Key:              aws.String(key),
			UploadId:         aws.String(uploadID),
			PartNumberMarker: partMarker,
		}

		result, err := s.client.ListParts(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to list parts: %w", err)
		}

		for _, part := range result.Parts {
			parts = append(parts, apptypes.PartInfo{
				PartNumber: int(*part.PartNumber),
				ETag:       *part.ETag,
				Size:       *part.Size,
			})
		}

		if !*result.IsTruncated {
			break
		}
		partMarker = result.NextPartNumberMarker
	}

	return parts, nil
}

func (s *S3Service) CompleteUpload(ctx context.Context, key, uploadID string, parts []apptypes.CompletedPart) (string, error) {
	var completedParts []types.CompletedPart
	for _, p := range parts {
		completedParts = append(completedParts, types.CompletedPart{
			PartNumber: aws.Int32(int32(p.PartNumber)),
			ETag:       aws.String(p.ETag),
		})
	}

	input := &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(s.bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	}

	result, err := s.client.CompleteMultipartUpload(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to complete multipart upload: %w", err)
	}

	return *result.Location, nil
}

func (s *S3Service) AbortUpload(ctx context.Context, key, uploadID string) error {
	input := &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(s.bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
	}

	_, err := s.client.AbortMultipartUpload(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to abort multipart upload: %w", err)
	}

	return nil
}

func (s *S3Service) GetPartSize() int64 {
	return s.partSize
}
