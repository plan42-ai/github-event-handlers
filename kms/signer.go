package kms

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/plan42-ai/openid/jwt"
)

type Client interface {
	Sign(ctx context.Context, params *kms.SignInput, optFns ...func(*kms.Options)) (*kms.SignOutput, error)
}

type Signer struct {
	client Client
}

func NewSigner(cfg aws.Config) *Signer {
	return &Signer{client: kms.NewFromConfig(cfg)}
}

func NewSignerWithClient(client Client) *Signer {
	return &Signer{client: client}
}

func (s *Signer) SignGithubJWT(ctx context.Context, token *jwt.Token, keyAlias string) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("kms signer not configured")
	}
	if token == nil {
		return fmt.Errorf("jwt token is nil")
	}
	alias := keyAlias
	if alias == "" {
		return fmt.Errorf("kms key alias is required")
	}
	digest := sha256.Sum256([]byte(token.StringToSign()))
	resp, err := s.client.Sign(
		ctx, &kms.SignInput{
			KeyId:            aws.String(alias),
			Message:          digest[:],
			SigningAlgorithm: kmstypes.SigningAlgorithmSpecRsassaPkcs1V15Sha256,
			MessageType:      kmstypes.MessageTypeDigest,
		},
	)
	if err != nil {
		return fmt.Errorf("sign github jwt: %w", err)
	}
	if len(resp.Signature) == 0 {
		return fmt.Errorf("kms returned empty signature")
	}
	token.Signature = resp.Signature
	token.RawSignature = base64.RawURLEncoding.EncodeToString(resp.Signature)
	return nil
}
