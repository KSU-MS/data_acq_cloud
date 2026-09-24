package s3

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestSeparateObjectAndPresignEndpoints(t *testing.T) {
	const bucket = "vehicle-runs"
	const key = "run-id/session 123.mcap"
	const publicOrigin = "https://mcap.example.ts.net:8443"
	repo := NewS3Session("test-access-key", "test-secret-key", "us-east-1",
		bucket, "http://minio:9000", publicOrigin)

	// Capture the actual SDK request without connecting to Docker or MinIO.
	var objectURL *url.URL
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		objectURL = r.URL
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    r,
		}, nil
	})}
	_, err := repo.s3_session.client.PutObject(context.Background(), &awss3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   strings.NewReader("test recording"),
	}, func(o *awss3.Options) {
		o.HTTPClient = client
	})
	if err != nil {
		t.Fatal(err)
	}
	if objectURL == nil {
		t.Fatal("object client did not send a request")
	}
	if objectURL.Scheme != "http" || objectURL.Host != "minio:9000" {
		t.Fatalf("object operation used unexpected endpoint: %s", objectURL)
	}
	wantPath := "/" + bucket + "/" + key
	if objectURL.Path != wantPath {
		t.Fatalf("object path = %q, want %q", objectURL.Path, wantPath)
	}

	signedURL, err := url.Parse(repo.GetSignedUrl(context.Background(), bucket, key))
	if err != nil {
		t.Fatal(err)
	}
	if signedURL.Scheme+"://"+signedURL.Host != publicOrigin {
		t.Fatalf("signed URL used unexpected endpoint: %s", signedURL)
	}
	if signedURL.Path != wantPath {
		t.Fatalf("signed path = %q, want %q", signedURL.Path, wantPath)
	}
	query := signedURL.Query()
	if query.Get("X-Amz-SignedHeaders") != "host" || query.Get("X-Amz-Signature") == "" {
		t.Fatal("download URL must contain a signature covering the public host")
	}
	if query.Get("X-Amz-Expires") != "600" {
		t.Fatalf("expiry = %q, want 600 seconds", query.Get("X-Amz-Expires"))
	}
}
