package auth

import (
	"context"
	"time"

	"litepan/internal/domain"
	"litepan/internal/driver"
)

// Refresh 执行一次认证刷新（主动/被动共用入口，含 per-account 锁）。
func (s *Service) Refresh(ctx context.Context, accountID int64, caller driver.RefreshCaller) (driver.RefreshOutcome, error) {
	unlock := s.locks.Lock(accountID)
	defer unlock()
	return s.refreshUnlocked(ctx, accountID, caller)
}

func (s *Service) refreshUnlocked(ctx context.Context, accountID int64, caller driver.RefreshCaller) (driver.RefreshOutcome, error) {
	if s.drivers == nil {
		err := domain.Errorf(domain.CodeInternal, "认证服务缺少驱动管理器")
		return driver.RefreshRetryable, err
	}
	drv, err := s.drivers.Get(ctx, accountID)
	if err != nil {
		s.log.Warn("auth refresh get driver", "account", accountID, "err", err)
		return driver.RefreshRetryable, err
	}
	refresher, ok := drv.(driver.AuthRefresher)
	if !ok {
		err := domain.Errorf(domain.CodeNotImplement, "驱动不支持认证刷新")
		return driver.RefreshRetryable, err
	}

	st, err := s.loadState(ctx, accountID)
	if err != nil {
		s.log.Warn("auth refresh load state", "account", accountID, "err", err)
		return driver.RefreshRetryable, err
	}

	outcome, rerr := refresher.RefreshAuth(ctx, caller)
	if outcome == driver.RefreshSuccess {
		st, _ = s.loadState(ctx, accountID)
		s.applyTokenSchedule(drv, st)
		now := s.nowTime()
		name := s.accountName(ctx, accountID)
		if st.Status != domain.AuthActive || st.LastRefreshAt.IsZero() || now.Sub(st.LastRefreshAt) > 3*time.Second {
			s.markSuccess(ctx, accountID, st)
		} else if err := s.authStates.Upsert(ctx, st); err != nil {
			s.log.Warn("auth update schedule", "account", accountID, "err", err)
		}
		if caller == driver.CallerPassive {
			s.log.Info("账号被动认证刷新成功", "account_id", accountID, "account", name)
		}
		return outcome, nil
	}
	if rerr == nil {
		rerr = domain.Errf(domain.CodeAuthExpired)
	}
	s.handleFailure(ctx, accountID, st, outcome, caller, rerr)
	s.log.Warn("auth refresh failed", "account", accountID, "caller", caller, "outcome", outcome, "err", rerr)
	return outcome, rerr
}

// onCredentialsPersisted 驱动内联刷新写回 token 时同步认证成功态（如 123 apiCall 自动续期）。
func (s *Service) onCredentialsPersisted(ctx context.Context, accountID int64) {
	st, err := s.loadState(ctx, accountID)
	if err != nil {
		return
	}
	if s.accounts != nil {
		if acc, aerr := s.accounts.Get(ctx, accountID); aerr == nil && acc != nil {
			if drv, ok := driver.New(acc.DriverType); ok {
				s.applyTokenSchedule(drv, st)
			}
		}
	}
	name := s.accountName(ctx, accountID)
	s.log.Info("请求链路触发认证凭证回写", "account_id", accountID, "account", name)
	s.markSuccess(ctx, accountID, st)
	if s.gate != nil {
		s.gate.InvalidateStateCache(accountID)
	}
}

func (s *Service) applyTokenSchedule(drv driver.Driver, st *domain.AuthState) {
	cfg := drv.Config()
	now := s.nowTime()
	switch cfg.AuthType {
	case driver.AuthToken:
		if cfg.TokenLifetime > 0 {
			st.TokenExpires = now.Add(cfg.TokenLifetime)
		}
	case driver.AuthCookie:
		if cfg.HealthCheckInterval > 0 {
			st.CookieExpires = now.Add(cfg.HealthCheckInterval)
		}
	}
}
