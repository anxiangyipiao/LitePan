package guangya

import (
	"context"
	"strings"

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

func (d *Driver) currentToken() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.token
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

	return token, nil
}

func (d *Driver) validateToken(ctx context.Context) error {
	var out struct {
		Sub string `json:"sub"`
	}
	if err := d.accountGET(ctx, pathUserMe, &out); err != nil {
		return err
	}
	if strings.TrimSpace(out.Sub) == "" {
		return domain.Errf(domain.CodeAuthExpired)
	}
	return nil
}
