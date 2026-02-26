// Package aws provides a stub AWS Secrets Manager provider.
// To activate: add github.com/aws/aws-sdk-go-v2/service/secretsmanager as a dependency
// and implement fetchFromASM and rotateInASM below.
package aws

import (
	"context"
	"fmt"

	"github.com/hstores/keysmith/internal/provider"
)

// AWS fetches and rotates secrets stored in AWS Secrets Manager.
// This is a stub implementation — see the TODOs below to wire up the real SDK.
type AWS struct{}

// New returns a new AWS Secrets Manager provider.
func New() *AWS { return &AWS{} }

func (a *AWS) Name() string { return "aws" }

func (a *AWS) Validate(params map[string]string) error {
	if params["secretId"] == "" {
		return fmt.Errorf("aws provider: secretId param is required")
	}
	return nil
}

// FetchSecret retrieves the current value of a secret from AWS Secrets Manager.
//
// TODO: replace stub with real implementation:
//
//	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(params["region"]))
//	client := secretsmanager.NewFromConfig(cfg)
//	out, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
//	    SecretId: aws.String(params["secretId"]),
//	})
//	// parse out.SecretString as JSON into provider.Secret
func (a *AWS) FetchSecret(_ context.Context, _ map[string]string) (provider.Secret, error) {
	return nil, fmt.Errorf("aws provider: not yet implemented — add aws-sdk-go-v2 and implement FetchSecret")
}

// RotateSecret triggers rotation in AWS Secrets Manager and returns the new value.
//
// TODO: replace stub with real implementation:
//
//	_, err := client.RotateSecret(ctx, &secretsmanager.RotateSecretInput{
//	    SecretId: aws.String(params["secretId"]),
//	})
//	// then call GetSecretValue to retrieve the new value
func (a *AWS) RotateSecret(_ context.Context, _ map[string]string) (provider.Secret, error) {
	return nil, fmt.Errorf("aws provider: not yet implemented — add aws-sdk-go-v2 and implement RotateSecret")
}
