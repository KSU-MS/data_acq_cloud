package s3

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// s3Session shares clients for object operations and browser-facing signed URLs.
type s3Session struct {
	client        *s3.Client
	presignClient *s3.PresignClient
	bucket        string
}

// NewS3Session uses endpoint for server-side object operations and publicEndpoint
// for presigned download URLs. Both must address the same S3 service and bucket.
// For MinIO in Docker, endpoint can be http://minio:9000; publicEndpoint must be
// the S3 API origin reachable by users on the tailnet. "Public" does not require
// exposing the service to the internet.
func NewS3Session(accessKey string, secretKey string, region string, bucket string, endpoint string, publicEndpoint string) *S3Repository {
	staticCreds := credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
		config.WithCredentialsProvider(staticCreds))
	if err != nil {
		log.Fatalf("could not load config: %v", err)
	}

	// Keep uploads, downloads and deletes on the internal Docker network.
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		// Address objects as host/bucket/key without requiring bucket.host DNS.
		o.UsePathStyle = true
	})
	// Sign against the client-reachable origin from the start: the signature
	// includes the Host header, so replacing minio:9000 in a signed URL later
	// would invalidate it. Any reverse proxy must preserve that host and port.
	presignS3Client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(publicEndpoint)
		o.UsePathStyle = true
	})

	presignClient := s3.NewPresignClient(presignS3Client)
	session := &s3Session{
		client:        client,
		bucket:        bucket,
		presignClient: presignClient,
	}
	return &S3Repository{
		s3_session: session,
	}
}
