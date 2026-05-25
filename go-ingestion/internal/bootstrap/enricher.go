package bootstrap

import (
	"context"
	"io"
	"log/slog"
	"net/http"

	goredis "github.com/redis/go-redis/v9"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/user-for-download/go-dota2/internal/config"
	"github.com/user-for-download/go-dota2/internal/enrich"
	enrichgate "github.com/user-for-download/go-dota2/internal/enrich/gate"
	"github.com/user-for-download/go-dota2/internal/enrich/httpclient"
	"github.com/user-for-download/go-dota2/internal/enrich/sources/dotaconstants"
	"github.com/user-for-download/go-dota2/internal/proxy"
	"github.com/user-for-download/go-dota2/internal/storage/refdatastore"
)

type EnricherDeps struct {
	LocalRunner *enrich.Runner
	MainRunner  *enrich.Runner
	HTTPClient  io.Closer
	HasLocal   bool
}

type EnricherDepsClosable interface {
	Close() error
}

func (d *EnricherDeps) Close() error {
	if cli, ok := d.HTTPClient.(EnricherDepsClosable); ok {
		return cli.Close()
	}
	return nil
}

func BuildEnricher(
	ctx context.Context,
	cfg *config.Config,
	log *slog.Logger,
	pool proxy.Pool,
	rdb *goredis.Client,
	db *pgxpool.Pool,
) (*EnricherDeps, error) {
	repo := ReferenceWriter(db, log)

	remoteHTTP, err := httpclient.NewProxied(httpclient.ProxiedConfig{
		Pool:        pool,
		Hold:        cfg.Proxy.Hold,
		Timeout:     cfg.Enrich.HTTPTimeout,
		Fallback:    &http.Client{Timeout: cfg.Enrich.HTTPTimeout},
		AllowDirect: cfg.Enrich.AllowDirect,
		MaxRetries:  cfg.Enrich.MaxProxyRetries,
		Backoff:     cfg.Enrich.ProxyBackoff,
		Logger:      log,
	})
	if err != nil {
		return nil, err
	}

	remoteSrcs := buildSources(cfg.Enrich.DotaConstantsBaseURL, remoteHTTP, repo)

	// selectGate picks the correct gate implementation:
	//
	//   ForceBootstrap=true  →  gate.Always{}
	//     Every run unconditionally executes all sources regardless of when they
	//     last succeeded. Use this to recover from a situation where the enricher
	//     was restarted mid-interval and the gate keys in Redis are still fresh,
	//     blocking the at-start run. Set ENRICH_FORCE_BOOTSTRAP=true for one
	//     restart, then revert to false once the data is populated.
	//
	//   ForceBootstrap=false (default)  →  gate.Gate (Interval mode)
	//     Each source is skipped if its last-success key in Redis is younger than
	//     ENRICH_INTERVAL. This prevents redundant network fetches on every
	//     container restart.
	var runGate enrichgate.RunGate
	if cfg.Enrich.ForceBootstrap {
		log.Warn("enricher: ForceBootstrap enabled — gate bypassed, all sources will run unconditionally")
		runGate = enrichgate.Always{}
	} else {
		runGate = enrichgate.New(enrichgate.Config{
			Prefix: cfg.Enrich.BootstrapPrefix,
			MinAge: cfg.Enrich.Interval,
			TTL:    cfg.Enrich.Interval,
			Mode:   enrichgate.Interval,
			Client: rdb,
		})
	}

	mainRunner, err := enrich.NewRunner(enrich.RunnerOptions{
		Sources: remoteSrcs,
		HTTP:    remoteHTTP,
		Gate:    runGate,
		Logger:  log,
	})
	if err != nil {
		remoteHTTP.Close()
		return nil, err
	}

	deps := &EnricherDeps{
		MainRunner: mainRunner,
		HTTPClient: remoteHTTP,
	}

	if cfg.Enrich.LocalBootstrapDir != "" {
		localBase := "file://" + cfg.Enrich.LocalBootstrapDir
		localHTTP := httpclient.NewFileClient()
		localSrcs := buildSources(localBase, localHTTP, repo)

		localRunner, err := enrich.NewRunner(enrich.RunnerOptions{
			Sources: localSrcs,
			HTTP:    localHTTP,
			Gate:    runGate,
			Logger:  log,
		})
		if err != nil {
			log.Warn("enricher: local runner init failed", "err", err)
		} else {
			deps.LocalRunner = localRunner
			deps.HasLocal = true
		}
	}

	return deps, nil
}

func buildSources(baseURL string, http enrich.HTTPClient, writer refdatastore.RefDataWriter) []enrich.RunSource {
	return []enrich.RunSource{
		dotaconstants.NewHeroesSource(baseURL, writer, http),
		dotaconstants.NewAbilitiesSource(baseURL, writer, http),
		dotaconstants.NewAbilityIDsSource(baseURL, writer, http),
		dotaconstants.NewHeroAbilitiesSource(baseURL, writer, http),
		dotaconstants.NewGameModesSource(baseURL, writer, http),
		dotaconstants.NewLobbyTypesSource(baseURL, writer, http),
		dotaconstants.NewRegionsSource(baseURL, writer, http),
		dotaconstants.NewItemsSource(baseURL, writer, http),
		dotaconstants.NewItemIDsSource(baseURL, writer, http),
		dotaconstants.NewPatchesSource(baseURL, writer, http),
	}
}