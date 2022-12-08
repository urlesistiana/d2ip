package main

import (
	"flag"
	"fmt"
	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
)

var version = "dev"

var (
	domains     = flag.String("d", "", "root domain(s), split multiple domains using ','")
	listenAddr  = flag.String("l", ":53", "listen address")
	metricsAddr = flag.String("m", "", "prometheus metrics server")
)
var logger = mustInitLogger()

func main() {
	flag.Parse()

	// Check args
	if len(*domains) == 0 {
		logger.Fatal("missing -d arg")
	}

	logger.Info("d2ip is starting", zap.String("version", version))

	suffixes := make(map[string]struct{})
	for _, s := range strings.Split(*domains, ",") {
		s = dns.Fqdn(s)
		suffixes[s] = struct{}{}
	}
	h := &d2ip{
		domains: suffixes,
		queryCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "d2ip_query_total",
			Help: "total number of incoming queries",
		}),
		errCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "d2ip_err_total",
			Help: "total number of invalid queries",
		}),
	}
	reg := prometheus.NewRegistry()
	reg.MustRegister(h.queryCounter, h.errCounter)

	var closed atomic.Bool

	// Starting metrics server.
	if len(*metricsAddr) > 0 {
		l, err := net.Listen("tcp", *metricsAddr)
		if err != nil {
			logger.Fatal("failed to start metrics end point", zap.Error(err))
		}
		logger.Info("starting metrics end point", zap.Stringer("addr", l.Addr()))
		defer l.Close()
		go func() {
			err := http.Serve(l, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
			if err != nil && !closed.Load() {
				logger.Fatal("metrics end point exited", zap.Error(err))
			}
		}()
	}

	// Listen udp.
	c, err := net.ListenPacket("udp", *listenAddr)
	if err != nil {
		logger.Fatal("failed to listen udp socket", zap.Error(err))
	}
	logger.Info("listening udp", zap.Stringer("addr", c.LocalAddr()))
	defer c.Close()

	// Listen tcp.
	l, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		logger.Fatal("failed to listen tcp socket", zap.Error(err))
	}
	logger.Info("listening tcp", zap.Stringer("addr", l.Addr()))
	defer l.Close()

	// Starting udp & tcp servers.
	go func() {
		server := dns.Server{
			PacketConn: c,
			Net:        "udp",
			Handler:    h,
		}
		if err := server.ActivateAndServe(); err != nil && !closed.Load() {
			logger.Fatal("udp server exited", zap.Error(err))
		}
	}()
	go func() {
		server := dns.Server{
			Listener: l,
			Net:      "tcp",
			Handler:  h,
		}
		if err := server.ActivateAndServe(); err != nil && !closed.Load() {
			logger.Fatal("tcp server exited", zap.Error(err))
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan
	closed.Store(true)
	logger.Info("exiting", zap.Stringer("signal", sig))
}

func mustInitLogger() *zap.Logger {
	l, err := zap.NewProduction(zap.WithCaller(false))
	if err != nil {
		panic(fmt.Sprintf("init logger: %s", err))
	}
	return l
}
