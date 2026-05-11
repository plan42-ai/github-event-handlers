package kms

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/plan42-ai/openid/jwt"
)

type fakeKMSClient struct {
	signOutput *kms.SignOutput
	signErr    error
	received   *kms.SignInput
}

func (f *fakeKMSClient) Sign(_ context.Context, input *kms.SignInput, _ ...func(*kms.Options)) (
	*kms.SignOutput,
	error,
) {
	f.received = input
	if f.signErr != nil {
		return nil, f.signErr
	}
	return f.signOutput, nil
}

func TestSignerUsesKMS(t *testing.T) {
	client := &fakeKMSClient{signOutput: &kms.SignOutput{Signature: []byte("sig")}}
	signer := NewSignerWithClient(client)
	token := jwt.Token{
		Header:  jwt.Header{Algorithm: jwt.AlgorithmRS256, Type: "JWT"},
		Payload: jwt.Payload{IssuedAt: time.Now(), Expiration: time.Now().Add(time.Minute)},
	}
	if err := signer.SignGithubJWT(context.Background(), &token, "alias/github/key"); err != nil {
		t.Fatalf("sign token: %v", err)
	}
	if token.RawSignature == "" {
		t.Fatalf("expected raw signature to be set")
	}
	if client.received == nil || client.received.SigningAlgorithm != kmstypes.SigningAlgorithmSpecRsassaPkcs1V15Sha256 {
		t.Fatalf("unexpected kms input: %#v", client.received)
	}
}

func TestSignerPropagatesError(t *testing.T) {
	client := &fakeKMSClient{signErr: errors.New("boom")}
	signer := NewSignerWithClient(client)
	if err := signer.SignGithubJWT(context.Background(), &jwt.Token{}, "alias"); err == nil {
		t.Fatalf("expected error")
	}
}
