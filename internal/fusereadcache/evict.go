package fusereadcache

import (
	"context"
	"time"
)

// maintainInterval 限制过期扫描频率：不再每次写块都全表扫一遍 created_at。
const maintainInterval = 60 * time.Second

func (s *Service) maintain(cfg Config) error {
	if s == nil || s.store == nil {
		return nil
	}
	cutoff := time.Now().Add(-time.Duration(cfg.RetentionDays) * 24 * time.Hour).Unix()
	return s.store.expireBefore(cutoff)
}

func (s *Service) ensureSpace(cfg Config, incoming int64) error {
	if s.store == nil {
		return nil
	}
	// usedBytes 是内存计数，避免每次写块都做全表 SUM/COUNT 扫描。
	used := s.store.usedBytes.Load()
	if used+incoming <= cfg.MaxBytes {
		return nil
	}
	for used+incoming > cfg.MaxBytes {
		meta, ok, err := s.pickEvict(cfg)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		if err := s.store.deleteBlock(meta); err != nil {
			return err
		}
		used = s.store.usedBytes.Load()
		if used+incoming <= cfg.MaxBytes {
			break
		}
	}
	return nil
}

func (s *Service) pickEvict(cfg Config) (blockMeta, bool, error) {
	if cfg.EvictionPolicy == PolicyLargeFile {
		return s.store.pickEvictLargeFile()
	}
	return s.store.pickEvictLRU()
}

func (s *Service) putBlockWithPolicy(ctx context.Context, accountID int64, fileID string, blockIdx int64, data []byte) error {
	cfg := LoadConfig(ctx, s.settings)
	// 过期扫描按时间节流；s.lastMaintain 在调用方持 s.mu 写锁期间访问，无需额外锁。
	if time.Since(s.lastMaintain) >= maintainInterval {
		s.lastMaintain = time.Now()
		if err := s.maintain(cfg); err != nil {
			return err
		}
	}
	if err := s.ensureSpace(cfg, int64(len(data))); err != nil {
		return err
	}
	return s.store.putBlock(accountID, fileID, blockIdx, data)
}
