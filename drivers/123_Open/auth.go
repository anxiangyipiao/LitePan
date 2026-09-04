package pan123open

import (
	"context"

	"litepan/internal/domain"
	"litepan/internal/driver"
)

// RefreshAuth 主动/被动认证刷新入口。
func (d *Driver) RefreshAuth(ctx context.Context, _ driver.RefreshCaller) (driver.RefreshOutcome, error) {
	_, err := d.doRefresh(ctx)
	if err != nil {
		return driver.ClassifyOAuthRefreshError(err), err
	}
	return driver.RefreshSuccess, nil
}

func (d *Driver) currentToken() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.token
}

func (d *Driver) doRefresh(ctx context.Context) (string, error) {
	d.mu.Lock()
	refresh := d.refresh
	d.mu.Unlock()
	if refresh == "" {
		return "", domain.Errorf(domain.CodeAuthExpired, "缺少 refresh_token，无法刷新访问令牌")
	}

	d.mu.Lock()
	token := d.token
	d.mu.Unlock()

	return token, nil
}
