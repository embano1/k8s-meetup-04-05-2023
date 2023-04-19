package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/kelseyhightower/envconfig"
	"github.com/nats-io/nats.go"
	"go.uber.org/automaxprocs/maxprocs"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

type loggerCtxKey string

const (
	loggerKey loggerCtxKey = "logger"
)

type config struct {
	NatsURL     string `envconfig:"NATS_SERVER" required:"true"`
	Topic       string `envconfig:"NATS_TOPIC" required:"true"`
	HealthZ     string `envconfig:"HEALTHZ_ADDRESS" default:":8080"`
	HealthZPath string `envconfig:"HEALTHZ_PATH" default:"/healthz"`
}

var ready atomic.Bool

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logger, err := zap.NewDevelopment()
	if err != nil {
		panic("could not create logger: " + err.Error())
	}
	logger = logger.Named("publisher")
	ctx = context.WithValue(ctx, loggerKey, logger)

	_, err = maxprocs.Set()
	if err != nil {
		logger.Fatal("could not set maxprocs goroutine limit", zap.Error(err))
	}

	var cfg config
	err = envconfig.Process("", &cfg)
	if err != nil {
		logger.Fatal("could not create configuration", zap.Error(err))
	}

	if err = run(ctx, cfg); err != nil && !(errors.Is(err, context.Canceled) || errors.Is(err, http.ErrServerClosed)) {
		logger.Fatal("could not run publisher", zap.Error(err))
	}
	logger.Info("shutdown complete")
}

func run(ctx context.Context, cfg config) error {
	logger := ctx.Value(loggerKey).(*zap.Logger)
	eg, egCtx := errgroup.WithContext(ctx)

	logger.Info(
		"starting healthz handler",
		zap.String("address", cfg.HealthZ),
		zap.String("path", cfg.HealthZPath),
	)
	eg.Go(func() error {
		return runHealthZ(egCtx, cfg.HealthZ, cfg.HealthZPath)
	})

	logger.Info("starting nats jetstream message producer", zap.String("natsURL", cfg.NatsURL))
	eg.Go(func() error {
		return runPublisher(egCtx, cfg.NatsURL, cfg.Topic)
	})

	return eg.Wait()
}

func runPublisher(ctx context.Context, natsURL, topic string) error {
	logger := ctx.Value(loggerKey).(*zap.Logger)

	nc, err := nats.Connect(natsURL)
	if err != nil {
		return fmt.Errorf("could not connect to nats: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		return fmt.Errorf("could not create nats jetstream context: %w", err)
	}

	streamCfg := nats.StreamConfig{
		Name:     "e2e",
		Subjects: []string{topic},
	}

	logger.Info("creating nats stream", zap.String("topic", topic))
	_, err = js.AddStream(&streamCfg)
	if err != nil {
		return fmt.Errorf("could not create nats stream: %w", err)
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	counter := 0
	for {
		select {
		case <-ctx.Done():
			logger.Info("shutting down publisher", zap.Any("cause", ctx.Err()))
			return nil
		case <-ticker.C:
			msg := fmt.Sprintf("test message: %d @%s", counter, time.Now().UTC().String())
			resp, err := js.Publish(topic, []byte(msg))
			if err != nil {
				logger.Error("could not publish message", zap.Error(err))
				ready.Store(false)
				continue
			}
			logger.Info("successfully published message", zap.Uint64("sequenceID", resp.Sequence))
			ready.Store(true)
			counter++
		}
	}
}

func runHealthZ(ctx context.Context, address, path string) error {
	logger := ctx.Value(loggerKey).(*zap.Logger)

	router := httprouter.New()
	router.GET(path, func(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
		if !ready.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	})

	srv := http.Server{
		Addr:         address,
		Handler:      router,
		ReadTimeout:  time.Second * 5,
		WriteTimeout: time.Second * 5,
	}

	go func() {
		<-ctx.Done()
		logger.Info("shutting down http server", zap.Any("cause", ctx.Err()))

		timeoutCtx, cancel := context.WithTimeout(context.Background(), time.Second*3)
		defer cancel()

		if err := srv.Shutdown(timeoutCtx); err != nil {
			logger.Error("could not shut down http server", zap.Error(err))
		}
	}()

	return srv.ListenAndServe()
}
