package main

import (
	"context"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type config struct {
	Targets   []Target
	Community string
}

func main() {

	logger, _ := zap.NewProduction()
	defer logger.Sync()

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("/etc/rittal-exporter/")
	viper.AddConfigPath(".")
	err := viper.ReadInConfig()
	if err != nil {
		logger.Panic("No valid configuration found", zap.Error(err))
	}

	config := config{}

	err = viper.Unmarshal(&config)
	if err != nil {
		logger.Panic("No valid configuration found", zap.Error(err))
	}

	wg := sync.WaitGroup{}
	workerChan := make(chan string)
	devices := map[string]map[string]RittalDevice{}
	for i := 0; i < len(config.Targets); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for target := range workerChan {
				l := logger.With(zap.String("device", target))
				l.Info("Starting loading device config")
				start := time.Now()
				x, err := loadDevices(target, config.Community)
				if err != nil {
					l.Error("Error loading config", zap.Error(err))
					return
				}
				devices[target] = x
				duration := time.Since(start).Seconds()
				l.Info("Finished scrape", zap.Float64("duration_seconds", duration))
			}
		}()
	}

	for _, target := range config.Targets {
		workerChan <- target.Host
	}
	close(workerChan)
	wg.Wait()
	logger.Info("Ready for processing")
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		target := query.Get("target")
		if len(query["target"]) != 1 || target == "" {
			http.Error(w, "'target' parameter must be specified once", http.StatusBadRequest)
			return
		}

		registry := prometheus.NewRegistry()
		h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})

		deviceType := ""
		ip := ""
		for _, x := range config.Targets {
			if x.Host == target || x.Alias == target {
				deviceType = x.Type
				ip = x.Host
				break
			}
		}

		if ip == "" {
			http.Error(w, "Not found", 404)
			return
		}

		c := Collector{Devices: devices[ip], Ip: ip, DeviceType: deviceType, Community: config.Community}
		registry.MustRegister(c)
		h.ServeHTTP(w, r)
	})
	server := &http.Server{Addr: ":9191", Handler: nil}
	go func() {
		err := server.ListenAndServe()
		if err != nil {
			logger.Error("Error starting server", zap.Error(err))
		}
	}()

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, os.Kill, syscall.SIGTERM, syscall.SIGKILL)

	<-interrupt
	logger.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown:", zap.Error(err))
	}
	logger.Info("Server stopped")

}
