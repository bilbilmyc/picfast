package storage

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"

	"github.com/atbeta/picfast/internal/domain"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

var tosBucketName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)

func init() {
	Register(string(domain.StrategyTypeTOS), func(cfg json.RawMessage) (Storage, error) {
		return NewTOSStorage(cfg)
	})
	RegisterValidator(string(domain.StrategyTypeTOS), func(cfg json.RawMessage) error {
		_, err := parseTOSConfig(cfg)
		return err
	})
}

func parseTOSConfig(cfg json.RawMessage) (domain.S3StrategyConfig, error) {
	var c domain.S3StrategyConfig
	if err := json.Unmarshal(cfg, &c); err != nil {
		return c, err
	}
	if c.Endpoint == "" || c.Region == "" || c.Region == "auto" || c.Bucket == "" || c.AccessKeyID == "" || c.SecretAccessKey == "" {
		return c, fmt.Errorf("endpoint, region, bucket, access_key, and secret_key are required for TOS storage")
	}
	if !tosBucketName.MatchString(c.Bucket) {
		return c, fmt.Errorf("TOS bucket must contain 3-63 lowercase letters, digits, or hyphens, and start and end with a letter or digit")
	}
	u, err := url.Parse(c.Endpoint)
	if err != nil || u.Hostname() == "" || net.ParseIP(u.Hostname()) != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return c, fmt.Errorf("TOS endpoint must be an HTTP(S) S3 API endpoint without a bucket or path, e.g. https://tos-s3-cn-beijing.volces.com")
	}
	if strings.HasPrefix(u.Hostname(), c.Bucket+".") || (strings.HasPrefix(u.Hostname(), "tos-") && !strings.HasPrefix(u.Hostname(), "tos-s3-")) {
		return c, fmt.Errorf("use the TOS S3 API endpoint without the bucket, e.g. https://tos-s3-cn-beijing.volces.com")
	}
	c.Endpoint = strings.TrimRight(c.Endpoint, "/")
	// Default direct links must use the TOS bucket host, never an AWS S3 host.
	if c.URL == "" {
		u.Host = c.Bucket + "." + u.Host
		c.URL = strings.TrimRight(u.String(), "/")
	}
	return c, nil
}

// NewTOSStorage uses TOS's S3-compatible API with SigV4 and virtual hosting.
func NewTOSStorage(cfg json.RawMessage) (*S3Storage, error) {
	c, err := parseTOSConfig(cfg)
	if err != nil {
		return nil, err
	}
	return newS3Storage(c, func(o *s3.Options) {
		o.UsePathStyle = false
		// Limit optional AWS checksum extensions on the compatible endpoint.
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	}), nil
}
