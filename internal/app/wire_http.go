package app

import (
	"net/http"
	"time"

	"litepan/internal/adminauth"
	"litepan/internal/api"
	"litepan/internal/apikey"
	"litepan/internal/cache"
	"litepan/internal/config"
	"litepan/internal/logx"
	"litepan/internal/notification"
	"litepan/internal/settings"
)

func wireHTTPServer(cfg config.Config, logs *logx.Manager, st *storeBundle, core *coreBundle, svc *servicesBundle) (*http.Server, error) {
	notifySvc := notification.NewService(notification.Options{
		Repo:     st.store.Notifications,
		Accounts: st.store.Accounts,
		Log:      logs.For(logx.ModuleSystem),
	})
	notifySvc.Register(core.bus)

	apiKeySvc := apikey.New(apikey.Options{
		Repo:     st.store.ApiKeys,
		Settings: st.settings,
		Strm:     svc.strm,
		StrmDir:  cfg.StrmDir,
		Secret:   core.secret,
	})
	if svc.automation != nil {
		svc.automation.SetApiKeys(apiKeySvc)
	}
	router := api.NewRouter(api.Deps{
		Logs:              logs,
		AccountSvc:        svc.account,
		Accounts:          st.store.Accounts,
		Configs:           st.store.Configs,
		Settings:          st.settings,
		Cache:             core.cache,
		ListHitTracker:    core.listHits,
		Files:             svc.files,
		Favorites:         svc.favorites,
		Uploads:           svc.uploads,
		OfflineDownloads:  svc.offlineDownloads,
		Playback:          svc.playback,
		Strm:              svc.strm,
		CacheRetention:    svc.cacheRetention,
		MediaOrganize:     svc.mediaOrganize,
		StrmScrape:        svc.strmScrape,
		MediaLibrary:      svc.mediaLibrary,
		Automation:        svc.automation,
		Backup:            svc.backup,
		Fuse:              svc.fuse,
		CrossTransfer:     svc.crossTransfer,
		EmbyProxy:         svc.embyProxy,
		FnosProxy:         svc.fnosProxy,
		RSS:               svc.rss,
		MagnetFavorites:   svc.magnetFavorites,
		ApiKeys:           apiKeySvc,
		Auth:              core.auth,
		AuthSched:         core.sched,
		AdminAuth:         adminauth.New(st.store.Configs, core.secret, logs.For(logx.ModuleAPI)),
		Notifications:     notifySvc,
		DataDir:           cfg.DataDir,
		OnSettingsUpdated: cacheSettingsHook(core.cache, st.settings, cfg.DataDir),
	})

	return &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}, nil
}

func cacheSettingsHook(cacheSvc *cache.Service, settingsSvc *settings.Service, dataDir string) func(map[string]string) {
	return func(changed map[string]string) {
		if !settingsTouchesCache(changed) {
			return
		}
		applyCacheRuntime(cacheSvc, settingsSvc, dataDir)
	}
}
