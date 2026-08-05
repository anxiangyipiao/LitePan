package strmscrape

import (
	"context"
	"strconv"
	"strings"

	"litepan/internal/mediaorganize"
	"litepan/internal/settings"
)

func (s *Service) GetSettings() Settings {
	writeMode := WriteModeMissingOnly
	if s.settings != nil {
		if v := strings.TrimSpace(s.settings.String(settings.KeyStrmScrapeWriteMode)); v != "" {
			writeMode = normalizeWriteMode(v)
		}
	}
	out := Settings{WriteMode: writeMode, Source: SourceTMDB}
	if s.settings == nil {
		return out
	}
	out.Source = normalizeSource(s.settings.String(settings.KeyStrmScrapeSource))
	out.MetaTubeURL = strings.TrimSpace(s.settings.String(settings.KeyStrmScrapeMetaTubeURL))
	enriched := mediaorganize.EnrichPlannerSettings(s.settings, nil)
	out.TmdbAPIKey = mediaorganize.PlannerTMDBAPIKey(enriched)
	out.TmdbLanguage = mediaorganize.PlannerTMDBLanguage(enriched)
	out.TmdbRequestIntervalMS = s.settings.Int(settings.KeyMOTmdbRequestIntervalMS)
	proxy := mediaorganize.TmdbProxyFromSettings(enriched)
	out.ProxyEnabled = proxy.Enabled
	out.ProxyURL = proxy.URL
	out.ProxyUsername = proxy.Username
	out.ProxyPassword = proxy.Password
	return out
}

func (s *Service) UpdateSettings(ctx context.Context, in Settings) error {
	if s.settings == nil {
		return nil
	}
	payload := map[string]string{
		settings.KeyStrmScrapeWriteMode: normalizeWriteMode(in.WriteMode),
		settings.KeyStrmScrapeSource:    normalizeSource(in.Source),
	}
	if url := strings.TrimSpace(in.MetaTubeURL); url != "" {
		payload[settings.KeyStrmScrapeMetaTubeURL] = url
	}
	if lang := strings.TrimSpace(in.TmdbLanguage); lang != "" {
		payload[settings.KeyMOTmdbLanguage] = lang
	}
	if key := strings.TrimSpace(in.TmdbAPIKey); key != "" {
		payload[settings.KeyMOTmdbAPIKey] = key
	}
	if in.TmdbRequestIntervalMS > 0 {
		payload[settings.KeyMOTmdbRequestIntervalMS] = strconv.Itoa(in.TmdbRequestIntervalMS)
	}
	if in.ProxyEnabled {
		payload[settings.KeyMOProxyEnabled] = "true"
	} else {
		payload[settings.KeyMOProxyEnabled] = "false"
	}
	payload[settings.KeyMOProxyURL] = strings.TrimSpace(in.ProxyURL)
	payload[settings.KeyMOProxyUsername] = strings.TrimSpace(in.ProxyUsername)
	if pwd := strings.TrimSpace(in.ProxyPassword); pwd != "" {
		payload[settings.KeyMOProxyPassword] = pwd
	}
	return s.settings.Update(ctx, payload)
}

func normalizeWriteMode(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case WriteModeOverwrite:
		return WriteModeOverwrite
	default:
		return WriteModeMissingOnly
	}
}

func normalizeSource(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case SourceMetaTube:
		return SourceMetaTube
	default:
		return SourceTMDB
	}
}
