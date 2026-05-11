package kms

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/plan42-ai/openid/jwt"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, signer.SignGithubJWT(context.Background(), &token, "alias/github/key"))
	require.NotEmpty(t, token.RawSignature)
	require.NotNil(t, client.received)
	require.Equal(t, kmstypes.SigningAlgorithmSpecRsassaPkcs1V15Sha256, client.received.SigningAlgorithm)
}

func TestSignerPropagatesError(t *testing.T) {
	client := &fakeKMSClient{signErr: errors.New("boom")}
	signer := NewSignerWithClient(client)
	require.Error(t, signer.SignGithubJWT(context.Background(), &jwt.Token{}, "alias"))
}
