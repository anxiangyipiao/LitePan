package wopan

import (
	"context"
	"strings"

	"litepan/internal/domain"
	"litepan/internal/driver"
)

// RefreshAuth 主动/被动刷新令牌。SDK 内建 AppRefreshToken 直连平台换新，
// 不走 LitePan 的统一 OAuth 代理；刷新成功后通过 onSDKRefreshToken 回写。
func (d *Driver) RefreshAuth(ctx context.Context, _ driver.RefreshCaller) (driver.RefreshOutcome, error) {
	c := d.woClient()
	if c == nil {
		return driver.RefreshRetryable, domain.Errorf(domain.CodeInternal, "驱动尚未初始化")
	}
	if err := c.RefreshToken(); err != nil {
		return classifyRefreshError(err), err
	}
	return driver.RefreshSuccess, nil
}

// onSDKRefreshToken 是 SDK 在请求链路自动续期后的回调：更新运行态并回写认证状态。
func (d *Driver) onSDKRefreshToken(accessToken, refreshToken string) {
	d.mu.Lock()
	d.token = accessToken
	d.refresh = refreshToken
	d.mu.Unlock()
	if d.persist != nil {
		_ = d.persist(context.Background(), domain.AuthCredentials{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
		})
	}
}

// currentToken 返回当前 access token（下载代理模式用于上游鉴权头）。
func (d *Driver) currentToken() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.token
}

func classifyRefreshError(err error) driver.RefreshOutcome {
	if err == nil {
		return driver.RefreshSuccess
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "unauthorized") || strings.Contains(lower, "401") ||
		strings.Contains(lower, "invalid") || strings.Contains(lower, "revoked") {
		return driver.RefreshFatal
	}
	return driver.RefreshRetryable
}
