package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/oklog/run"
	"github.com/pkg/errors"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	klogv1 "k8s.io/klog"
	klogv2 "k8s.io/klog/v2"

	// Blank import required to register GCP auth handlers to talk to GKE clusters.
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/google/gpe-collector/pkg/operator"
)

// The valid levels for the --log-level flag.
const (
	logLevelDebug = "debug"
	logLevelInfo  = "info"
	logLevelWarn  = "warn"
	logLevelError = "error"
)

var (
	validLogLevels = []string{
		logLevelDebug,
		logLevelInfo,
		logLevelWarn,
		logLevelError,
	}
)

func main() {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	apiserverURL := flag.String("apiserver", "", "help")
	logLevel := flag.String("log-level", logLevelInfo, fmt.Sprintf("Log level to use. Possible values: %s", strings.Join(validLogLevels, ", ")))

	flag.Parse()

	logger, err := setupLogger(*logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Creating logger failed: %s", err)
		os.Exit(2)
	}

	cfg, err := clientcmd.BuildConfigFromFlags(*apiserverURL, *kubeconfig)
	if err != nil {
		level.Error(logger).Log("msg", "building kubeconfig failed", "err", err)
		os.Exit(1)
	}
	op, err := operator.New(logger, cfg, operator.DefaultNamespace)
	if err != nil {
		level.Error(logger).Log("msg", "instantiating operator failed", "err", err)
		os.Exit(1)
	}

	var g run.Group
	{
		// Termination handler.
		term := make(chan os.Signal, 1)
		cancel := make(chan struct{})
		signal.Notify(term, os.Interrupt, syscall.SIGTERM)

		g.Add(
			func() error {
				select {
				case <-term:
					level.Info(logger).Log("msg", "received SIGTERM, exiting gracefully...")
				case <-cancel:
				}
				return nil
			},
			func(err error) {
				close(cancel)
			},
		)
	}
	{
		ctx, cancel := context.WithCancel(context.Background())
		g.Add(
			func() error {
				return op.Run(ctx)
			},
			func(err error) {
				cancel()
			},
		)
	}
	if err := g.Run(); err != nil {
		level.Error(logger).Log("msg", "exit with error", "err", err)
		os.Exit(1)
	}
}

func setupLogger(lvl string) (log.Logger, error) {
	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))

	switch lvl {
	case logLevelDebug:
		logger = level.NewFilter(logger, level.AllowDebug())
	case logLevelInfo:
		logger = level.NewFilter(logger, level.AllowInfo())
	case logLevelWarn:
		logger = level.NewFilter(logger, level.AllowWarn())
	case logLevelError:
		logger = level.NewFilter(logger, level.AllowError())
	default:
		return nil, errors.Errorf("log level %q unknown, must be one of (%s)", lvl, strings.Join(validLogLevels, ", "))
	}
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)

	// Set caller to one function higher up in the stack as it will just reference the
	// klog code with the default.
	klogv1.SetLogger(log.With(logger, "component", "k8s_client_runtime", "caller", log.Caller(4)))
	klogv2.SetLogger(log.With(logger, "component", "k8s_client_runtime", "caller", log.Caller(4)))
	// Limit log level to address CVE-2019-11250, which would cause bearer tokens to be logged.
	klogv1.ClampLevel(6)
	klogv2.ClampLevel(6)

	logger = log.With(logger, "caller", log.DefaultCaller)

	return logger, nil
}
