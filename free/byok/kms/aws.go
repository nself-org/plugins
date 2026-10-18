package kms

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
)

// AWSKMSProvider wraps the AWS KMS service for envelope encryption.
// It uses aws-sdk-go-v2 and resolves credentials via the default credential
// chain (env vars → shared credentials file → IAM role).
type AWSKMSProvider struct {
	client *kms.Client
	region string
}

// NewAWSKMSProvider constructs an AWSKMSProvider for the given region.
// Pass an empty region to use the AWS_DEFAULT_REGION env var.
func NewAWSKMSProvider(ctx context.Context, region string) (*AWSKMSProvider, error) {
	opts := []func(*awsconfig.LoadOptions) error{}
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("aws kms: load config: %w", err)
	}
	return &AWSKMSProvider{
		client: kms.NewFromConfig(cfg),
		region: cfg.Region,
	}, nil
}

// WrapKey uses AWS KMS GenerateDataKey to encrypt plainDEK under keyRef (a
// KMS key ARN or alias). We use Encrypt rather than GenerateDataKey because
// the caller already has a freshly generated DEK.
func (p *AWSKMSProvider) WrapKey(ctx context.Context, plainDEK []byte, keyRef string) ([]byte, error) {
	out, err := p.client.Encrypt(ctx, &kms.EncryptInput{
		KeyId:     aws.String(keyRef),
		Plaintext: plainDEK,
	})
	if err != nil {
		return nil, fmt.Errorf("aws kms: encrypt DEK: %w", err)
	}
	return out.CiphertextBlob, nil
}

// UnwrapKey uses AWS KMS Decrypt to recover the plaintext DEK. If the CMK
// has been disabled or scheduled for deletion the SDK returns an error.
func (p *AWSKMSProvider) UnwrapKey(ctx context.Context, wrappedDEK []byte, keyRef string) ([]byte, error) {
	out, err := p.client.Decrypt(ctx, &kms.DecryptInput{
		KeyId:          aws.String(keyRef),
		CiphertextBlob: wrappedDEK,
	})
	if err != nil {
		return nil, fmt.Errorf("aws kms: decrypt DEK: %w", err)
	}
	return out.Plaintext, nil
}

// DescribeKey fetches metadata for the given key ARN or alias.
func (p *AWSKMSProvider) DescribeKey(ctx context.Context, keyRef string) (KeyMetadata, error) {
	out, err := p.client.DescribeKey(ctx, &kms.DescribeKeyInput{
		KeyId: aws.String(keyRef),
	})
	if err != nil {
		return KeyMetadata{}, fmt.Errorf("aws kms: describe key: %w", err)
	}
	md := out.KeyMetadata
	state := "UNKNOWN"
	if md.KeyState != "" {
		state = string(md.KeyState)
	}
	createdAt := ""
	if md.CreationDate != nil {
		createdAt = md.CreationDate.String()
	}
	return KeyMetadata{
		KeyRef:    keyRef,
		Provider:  ProviderAWS,
		Algorithm: string(md.KeySpec),
		State:     state,
		CreatedAt: createdAt,
	}, nil
}

// Verify performs a wrap+unwrap round-trip with a 32-byte test value to
// confirm the caller has encrypt+decrypt permissions on keyRef.
func (p *AWSKMSProvider) Verify(ctx context.Context, keyRef string) error {
	testPlain := make([]byte, 32)
	for i := range testPlain {
		testPlain[i] = byte(i)
	}
	wrapped, err := p.WrapKey(ctx, testPlain, keyRef)
	if err != nil {
		return fmt.Errorf("aws kms verify wrap: %w", err)
	}
	recovered, err := p.UnwrapKey(ctx, wrapped, keyRef)
	if err != nil {
		return fmt.Errorf("aws kms verify unwrap: %w", err)
	}
	for i, b := range recovered {
		if b != testPlain[i] {
			return fmt.Errorf("aws kms verify: round-trip mismatch at byte %d", i)
		}
	}
	return nil
}
