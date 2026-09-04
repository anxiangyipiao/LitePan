package template

import (
	"context"
	"strings"

	"litepan/internal/domain"
	"litepan/internal/driver"
)

func (d *Driver) RefreshAuth(ctx context.Context, _ driver.RefreshCaller) (driver.RefreshOutcome, error) {
	_, err := d.doRefresh(ctx)
	if err != nil {
		return classifyRefreshError(err), err
	}
	return driver.RefreshSuccess, nil
}

func classifyRefreshError(err error) driver.RefreshOutcome {
	if ae, ok := domain.AsAppError(err); ok {
		msg := strings.ToLower(ae.Message)
		if strings.Contains(msg, "invalid") && strings.Contains(msg, "refresh") ||
			strings.Contains(msg, "revoked") ||
			strings.Contains(msg, "不能都为空") {
			return driver.RefreshFatal
		}
	}
	return driver.RefreshRetryable
}

func (d *Driver) doRefresh(ctx context.Context) (string, error) {
	d.mu.Lock()
	refresh := strings.TrimSpace(d.refresh)
	d.mu.Unlock()
	if refresh == "" {
		return "", domain.Errorf(domain.CodeAuthExpired, "缺少 refresh_token，无法刷新访问令牌")
	}

	d.mu.Lock()
	token := d.token
	d.mu.Unlock()

	if d.persist != nil {
		_ = d.persist(ctx, domain.AuthCredentials{
			AccessToken:  token,
			RefreshToken: refresh,
		})
	}
	return token, nil
}
