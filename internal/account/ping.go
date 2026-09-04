package account

import (
	"context"
	"strings"

	"litepan/internal/domain"
	"litepan/internal/driver"
)

func (s *Service) pingDriver(ctx context.Context, driverType, configJSON string, saving bool) error {
	drv, release, err := driver.OpenEphemeral(ctx, driverType, configJSON, driver.EphemeralConfig{})
	if err != nil {
		return err
	}
	defer release(ctx)

	if err := drv.Init(ctx); err != nil {
		return connectionTestError(drv, driverType, err, saving)
	}
	if err := drv.Ping(ctx); err != nil {
		return connectionTestError(drv, driverType, err, saving)
	}
	return nil
}

func connectionTestError(drv driver.Driver, driverType string, err error, saving bool) error {
	technical := err.Error()
	friendly := ""
	if e, ok := drv.(driver.ConnectionErrorExplainer); ok {
		friendly = e.ExplainConnectionError(technical, saving)
	}
	if strings.TrimSpace(friendly) == "" {
		friendly = domain.FriendlyConnectionError(driverType, technical, saving)
	}
	return domain.Errorf(domain.CodeDriverError, "%s", friendly).
		WithDetails(map[string]any{"technical": technical})
}
