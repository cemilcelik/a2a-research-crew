package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

// ServiceParamAuthorization, A2A transportlarının taşıdığı kimlik doğrulama
// parametresinin anahtarıdır.
const ServiceParamAuthorization = "authorization"

// ServerInterceptor, gelen A2A çağrılarındaki Bearer token'ı doğrular ve
// kullanıcıyı CallContext'e yerleştirir.
type ServerInterceptor struct {
	a2asrv.PassthroughCallInterceptor

	manager  *Manager
	required bool
}

// NewServerInterceptor, bir sunucu interceptor'ı oluşturur. required true ise
// token bulunmayan çağrılar reddedilir.
func NewServerInterceptor(manager *Manager, required bool) *ServerInterceptor {
	return &ServerInterceptor{manager: manager, required: required}
}

// Before implements a2asrv.CallInterceptor.
func (i *ServerInterceptor) Before(ctx context.Context, callCtx *a2asrv.CallContext, _ *a2asrv.Request) (context.Context, any, error) {
	token := bearerToken(callCtx.ServiceParams())
	if token == "" {
		if i.required {
			return ctx, nil, a2a.NewError(a2a.ErrUnauthenticated, "missing bearer token")
		}
		return ctx, nil, nil
	}

	claims, err := i.manager.ParseAccess(token)
	if err != nil {
		return ctx, nil, a2a.NewError(a2a.ErrUnauthenticated, err.Error())
	}

	callCtx.User = a2asrv.NewAuthenticatedUser(claims.Subject, map[string]any{
		"roles": claims.Roles,
	})
	return ctx, nil, nil
}

// TokenSource, istemci tarafında giden çağrıya eklenecek token'ı üretir.
type TokenSource func(ctx context.Context) (string, error)

// StaticTokenSource, sabit bir token döndüren TokenSource üretir.
func StaticTokenSource(token string) TokenSource {
	return func(context.Context) (string, error) { return token, nil }
}

// ClientInterceptor, giden A2A çağrılarına Bearer token ekler.
type ClientInterceptor struct {
	a2aclient.PassthroughInterceptor

	TokenSource TokenSource
}

// NewClientInterceptor, verilen token kaynağıyla bir istemci interceptor'ı
// oluşturur.
func NewClientInterceptor(source TokenSource) *ClientInterceptor {
	return &ClientInterceptor{TokenSource: source}
}

// Before implements a2aclient.CallInterceptor.
func (i *ClientInterceptor) Before(ctx context.Context, req *a2aclient.Request) (context.Context, any, error) {
	if i.TokenSource == nil {
		return ctx, nil, nil
	}
	token, err := i.TokenSource(ctx)
	if err != nil {
		return ctx, nil, fmt.Errorf("auth: resolve token: %w", err)
	}
	if token == "" {
		return ctx, nil, nil
	}
	if req.ServiceParams == nil {
		req.ServiceParams = a2aclient.ServiceParams{}
	}
	req.ServiceParams.Append(ServiceParamAuthorization, BearerPrefix+token)
	return ctx, nil, nil
}

func bearerToken(params *a2asrv.ServiceParams) string {
	values, ok := params.Get(ServiceParamAuthorization)
	if !ok || len(values) == 0 {
		return ""
	}
	raw := strings.TrimSpace(values[0])
	if len(raw) >= len(BearerPrefix) && strings.EqualFold(raw[:len(BearerPrefix)], BearerPrefix) {
		return strings.TrimSpace(raw[len(BearerPrefix):])
	}
	return raw
}
