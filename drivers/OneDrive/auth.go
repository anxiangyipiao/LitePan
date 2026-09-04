package onedrive

import (
	"context"

	"litepan/internal/domain"
	"litepan/internal/driver"
)

func (d *Driver) RefreshAuth(ctx context.Context, _ driver.RefreshCaller) (driver.RefreshOutcome, error) {
	_, err := d.doRefresh(ctx)
	if err != nil {
		return driver.ClassifyOAuthRefreshError(err), err
	}
	return driver.RefreshSuccess, nil
}

func (d *Driver) doRefresh(ctx context.Context) (string, error) {
	d.refreshMu.Lock()
	defer d.refreshMu.Unlock()
	refresh := d.currentRefreshToken()
	if refresh == "" {
		return "", domain.Errorf(domain.CodeAuthExpired, "缺少 refresh_token，无法刷新 OneDrive 访问令牌")
	}
	d.mu.Lock()
	token := d.token
	nextRefresh := d.refresh
	d.mu.Unlock()
	if d.persist != nil {
		if err := d.persist(ctx, domain.AuthCredentials{AccessToken: token, RefreshToken: nextRefresh}); err != nil {
			return "", domain.Wrap(domain.CodeInternal, err)
		}
	}
	return token, nil
}
