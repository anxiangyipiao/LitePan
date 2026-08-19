package auth

import (
	"context"
	"sync"
	"time"

	"litepan/internal/domain"
	"litepan/internal/driver"
)

const authStateCacheTTL = 5 * time.Second

type authStateCacheEntry struct {
	state     *domain.AuthState
	expiresAt time.Time
}

// Gate 是被动刷新闸门：请求前检查状态，401 时触发刷新。
type Gate struct {
	svc        *Service
	stateCache sync.Map // accountID -> *authStateCacheEntry
}

var _ GateChecker = (*Gate)(nil)

// Check 请求前闸门；failed/token_expired 阻断，冷却未到期阻断，冷却到期尝试被动刷新。
func (g *Gate) Check(ctx context.Context, accountID int64) error {
	if g == nil || g.svc == nil {
		return nil
	}
	st, err := g.loadStateCached(ctx, accountID)
	if err != nil {
		return err
	}
	now := g.svc.nowTime()
	switch st.Status {
	case domain.AuthFailed:
		return authBlocked(st, now)
	case domain.AuthTokenExpired:
		if err := g.HandlePassiveError(ctx, accountID); err != nil {
			return authBlocked(st, now)
		}
		return nil
	case domain.AuthCooldown:
		if passiveBypassesCooldown(st) {
			return g.HandlePassiveError(ctx, accountID)
		}
		if !st.NextRetryAt.IsZero() && now.Before(st.NextRetryAt) {
			return authBlocked(st, now)
		}
		return g.HandlePassiveError(ctx, accountID)
	default:
		return nil
	}
}

func (g *Gate) loadStateCached(ctx context.Context, accountID int64) (*domain.AuthState, error) {
	now := time.Now()
	if v, ok := g.stateCache.Load(accountID); ok {
		entry := v.(*authStateCacheEntry)
		if now.Before(entry.expiresAt) {
			return entry.state, nil
		}
		g.stateCache.Delete(accountID)
	}
	st, err := g.svc.loadState(ctx, accountID)
	if err != nil {
		return nil, err
	}
	g.stateCache.Store(accountID, &authStateCacheEntry{state: st, expiresAt: now.Add(authStateCacheTTL)})
	return st, nil
}

// InvalidateStateCache 清除指定账号的认证状态缓存（认证状态变更时调用）。
func (g *Gate) InvalidateStateCache(accountID int64) {
	if g == nil {
		return
	}
	g.stateCache.Delete(accountID)
}

// HandlePassiveError 被动刷新入口（请求遇到认证错误时调用）。
func (g *Gate) HandlePassiveError(ctx context.Context, accountID int64) error {
	if g == nil || g.svc == nil {
		return nil
	}
	unlock := g.svc.locks.Lock(accountID)
	defer unlock()

	st, err := g.svc.loadState(ctx, accountID)
	if err != nil {
		return err
	}
	now := g.svc.nowTime()
	if st.Status == domain.AuthActive && !st.LastRefreshAt.IsZero() {
		if now.Sub(st.LastRefreshAt) < passiveReuseWindow {
			return nil
		}
	}
	if st.Status == domain.AuthCooldown && !st.NextRetryAt.IsZero() && now.Before(st.NextRetryAt) {
		if !passiveBypassesCooldown(st) {
			return authBlocked(st, now)
		}
	}
	if st.Status == domain.AuthFailed || st.Status == domain.AuthTokenExpired {
		return authBlocked(st, now)
	}

	outcome, rerr := g.svc.refreshUnlocked(ctx, accountID, driver.CallerPassive)
	if outcome == driver.RefreshSuccess {
		return nil
	}
	if rerr != nil {
		return rerr
	}
	return domain.Errf(domain.CodeAuthExpired)
}
