// Command devidp is a minimal OpenID Connect provider for local development
// and CI only. It stands in for Google so that sign-in works end to end on
// a laptop and in the Playwright tests, with no account anywhere.
//
// It is NOT an identity provider. Anyone who reaches it signs in as any of
// its test users (Mestre Teste, Jogador Teste...) with one click and no
// password. That is why it can never be deployed:
//
//   - backend/Dockerfile, the production image, builds only cmd/api and
//     cmd/migrate; devidp has its own image (deploy/local/devidp.Dockerfile),
//     used only by deploy/local/compose.yaml;
//   - it refuses to start unless its issuer is on a loopback host
//     (localhost, *.localhost, 127.0.0.1 or ::1), which a real deployment
//     cannot use, and it refuses to start on Cloud Run (K_SERVICE is set);
//   - it prints a warning banner when it starts.
//
// The provider itself lives in internal/identity/oidctest, shared with the
// identity package's tests. Configuration comes from flags, or from the
// environment when a flag is not given:
//
//	-issuer         DEVIDP_ISSUER          issuer URL (default http://localhost:9090)
//	-listen         DEVIDP_LISTEN          listen address (default 127.0.0.1:9090)
//	-client-id      DEVIDP_CLIENT_ID       the client ID (default meurpg-local)
//	-client-secret  DEVIDP_CLIENT_SECRET   the client secret (default meurpg-local-secret)
//	-redirect-uris  DEVIDP_REDIRECT_URIS   comma-separated redirect URIs
//	                                       (default http://localhost:8080/auth/callback)
//	-healthcheck                           check that a running devidp answers, then exit
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/PuraFome/meuRPG/backend/internal/identity/oidctest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/config"
	"github.com/PuraFome/meuRPG/backend/internal/platform/safego"
)

const banner = `
################################################################################
#  devidp: DEVELOPMENT OpenID Connect provider.                                #
#  Anyone who can reach it signs in as any test user, WITHOUT A PASSWORD.      #
#  It exists for your machine and for CI only. NEVER DEPLOY IT.                #
################################################################################
`

// settings is devidp's configuration.
type settings struct {
	issuer       string
	listen       string
	clientID     string
	clientSecret string
	redirectURIs []string
	healthcheck  bool
}

func main() {
	s, err := parseSettings(os.Args[1:], os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "devidp:", err)
		os.Exit(2)
	}
	if s.healthcheck {
		if err := healthcheck(s); err != nil {
			fmt.Fprintln(os.Stderr, "devidp healthcheck:", err)
			os.Exit(1)
		}
		return
	}

	fmt.Fprint(os.Stderr, banner)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err = run(ctx, s, logger)
	stop() // not deferred: os.Exit below would skip a deferred call
	if err != nil {
		logger.Error("devidp stopped with an error", "error", err)
		os.Exit(1)
	}
}

// parseSettings reads flags, then the environment for anything not given
// as a flag, and refuses a configuration that could be a deployment.
func parseSettings(args []string, getenv func(string) string) (settings, error) {
	fromEnv := func(name, fallback string) string {
		if v := strings.TrimSpace(getenv(name)); v != "" {
			return v
		}
		return fallback
	}

	var s settings
	var redirectURIs string
	fs := flag.NewFlagSet("devidp", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&s.issuer, "issuer", fromEnv("DEVIDP_ISSUER", "http://localhost:9090"), "issuer URL")
	fs.StringVar(&s.listen, "listen", fromEnv("DEVIDP_LISTEN", "127.0.0.1:9090"), "listen address")
	fs.StringVar(&s.clientID, "client-id", fromEnv("DEVIDP_CLIENT_ID", "meurpg-local"), "client ID")
	fs.StringVar(&s.clientSecret, "client-secret", fromEnv("DEVIDP_CLIENT_SECRET", "meurpg-local-secret"), "client secret")
	fs.StringVar(&redirectURIs, "redirect-uris", fromEnv("DEVIDP_REDIRECT_URIS", "http://localhost:8080/auth/callback"), "comma-separated redirect URIs")
	fs.BoolVar(&s.healthcheck, "healthcheck", false, "check that a running devidp answers, then exit")
	if err := fs.Parse(args); err != nil {
		return settings{}, err
	}
	for uri := range strings.SplitSeq(redirectURIs, ",") {
		if uri = strings.TrimSpace(uri); uri != "" {
			s.redirectURIs = append(s.redirectURIs, uri)
		}
	}

	if err := refuseDeployment(s.issuer, getenv); err != nil {
		return settings{}, err
	}
	return s, nil
}

// refuseDeployment is the guard that keeps devidp off any real server: it
// only accepts an issuer on a loopback host, which no deployment can use,
// and it refuses to run on Cloud Run at all.
func refuseDeployment(issuer string, getenv func(string) string) error {
	if getenv("K_SERVICE") != "" || getenv("K_REVISION") != "" {
		return errors.New("refusing to start on Cloud Run (K_SERVICE is set): devidp signs anyone in without a password and must never be deployed")
	}
	u, err := url.Parse(issuer)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("the issuer must be an absolute http(s) URL, got %q", issuer)
	}
	if !config.IsLoopbackHost(u.Hostname()) {
		return fmt.Errorf("refusing to start with issuer %q: devidp only runs on a loopback host (localhost, *.localhost, 127.0.0.1 or ::1), because it signs anyone in without a password and must never be deployed", issuer)
	}
	return nil
}

// newProvider builds the provider devidp serves.
func newProvider(s settings, logger *slog.Logger) (*oidctest.Provider, error) {
	return oidctest.New(oidctest.Config{
		Issuer:       s.issuer,
		ClientID:     s.clientID,
		ClientSecret: s.clientSecret,
		RedirectURIs: s.redirectURIs,
		Users:        oidctest.TestUsers(),
		Logger:       logger,
	})
}

// run serves the provider until ctx is canceled.
func run(ctx context.Context, s settings, logger *slog.Logger) error {
	provider, err := newProvider(s, logger)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              s.listen,
		Handler:           provider,
		ReadHeaderTimeout: 10 * time.Second,
	}
	serveErr := make(chan error, 1)
	safego.Go(logger, "serve the development provider", func() { serveErr <- srv.ListenAndServe() }, func() { serveErr <- errors.New("devidp: the server panicked") })
	logger.WarnContext(ctx, "devidp is running: a development-only OpenID Connect provider; never deploy it",
		"issuer", s.issuer, "listen", s.listen, "client_id", s.clientID, "redirect_uris", s.redirectURIs)

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// healthcheck asks the devidp listening on s.listen for its discovery
// document. The container image has no shell or curl, so compose's
// healthcheck runs the binary itself: devidp -healthcheck.
func healthcheck(s settings) error {
	host, port, err := net.SplitHostPort(s.listen)
	if err != nil {
		return err
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	u, err := url.Parse(s.issuer)
	if err != nil {
		return err
	}
	target := "http://" + net.JoinHostPort(host, port) + strings.TrimSuffix(u.Path, "/") + "/.well-known/openid-configuration"

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status %d", target, resp.StatusCode)
	}
	return nil
}
