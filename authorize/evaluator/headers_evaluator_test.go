package evaluator

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v3"
	"github.com/go-jose/go-jose/v3/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/pomerium/pomerium/config"
	"github.com/pomerium/pomerium/pkg/cryptutil"
	"github.com/pomerium/pomerium/pkg/grpc/session"
)

func TestHeadersEvaluator(t *testing.T) {
	type A = []interface{}
	type M = map[string]interface{}

	signingKey, err := cryptutil.NewSigningKey()
	require.NoError(t, err)
	encodedSigningKey, err := cryptutil.EncodePrivateKey(signingKey)
	require.NoError(t, err)
	privateJWK, err := cryptutil.PrivateJWKFromBytes(encodedSigningKey, jose.ES256)
	require.NoError(t, err)
	publicJWK, err := cryptutil.PublicJWKFromBytes(encodedSigningKey, jose.ES256)
	require.NoError(t, err)

	eval := func(t *testing.T, data []proto.Message, input *HeadersRequest) (*HeadersResponse, error) {
		store := NewStoreFromProtos(math.MaxUint64, data...)
		store.UpdateIssuer("authenticate.example.com")
		store.UpdateJWTClaimHeaders(config.NewJWTClaimHeaders("email", "groups", "user", "CUSTOM_KEY"))
		store.UpdateSigningKey(privateJWK)
		e, err := NewHeadersEvaluator(context.Background(), store)
		require.NoError(t, err)
		return e.Evaluate(context.Background(), input)
	}

	t.Run("jwt", func(t *testing.T) {
		output, err := eval(t,
			[]proto.Message{
				&session.Session{Id: "s1", ImpersonateSessionId: proto.String("s2")},
				&session.Session{Id: "s2"},
			},
			&HeadersRequest{
				FromAudience: "from.example.com",
				ToAudience:   "to.example.com",
				Session: RequestSession{
					ID: "s1",
				},
			})
		require.NoError(t, err)

		rawJWT, err := jwt.ParseSigned(output.Headers.Get("X-Pomerium-Jwt-Assertion"))
		require.NoError(t, err)

		var claims M
		err = rawJWT.Claims(publicJWK, &claims)
		require.NoError(t, err)

		assert.Equal(t, claims["exp"], math.Round(claims["exp"].(float64)))
		assert.LessOrEqual(t, claims["exp"], float64(time.Now().Add(time.Minute*6).Unix()),
			"JWT should expire within 5 minutes, but got: %v", claims["exp"])
		assert.Equal(t, "s1", claims["sid"], "should set session id to input session id")
	})
}
