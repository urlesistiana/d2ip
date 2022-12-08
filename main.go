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
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

const version = "dev"

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

	// Starting metrics server.
	if len(*metricsAddr) > 0 {
		l, err := net.Listen("tcp", *metricsAddr)
		if err != nil {
			logger.Fatal("failed to start metrics end point", zap.Error(err))
		}
		defer l.Close()
		go func() {
			err := http.Serve(l, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
			logger.Fatal("metrics end point exited", zap.Error(err))
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
		if err := server.ActivateAndServe(); err != nil {
			logger.Fatal("udp server exited", zap.Error(err))
		}
	}()
	go func() {
		server := dns.Server{
			Listener: l,
			Net:      "tcp",
			Handler:  h,
		}
		if err := server.ActivateAndServe(); err != nil {
			logger.Fatal("tcp server exited", zap.Error(err))
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan
	logger.Info("exiting", zap.Stringer("signal", sig))
}

type d2ip struct {
	domains      map[string]struct{}
	queryCounter prometheus.Counter
	errCounter   prometheus.Counter
}

func (d *d2ip) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	resp := d.resp(w, r)
	_ = w.WriteMsg(resp)
}

func (d *d2ip) resp(w dns.ResponseWriter, q *dns.Msg) *dns.Msg {
	d.queryCounter.Inc()

	// Reject invalid queries.
	if len(q.Question) != 1 || q.Question[0].Qtype != dns.ClassINET {
		logger.Warn("invalid query", zap.Stringer("from", w.RemoteAddr()))
		d.errCounter.Inc()
		return reject(q, dns.RcodeRefused)
	}

	question := q.Question[0]
	// Parse ip addr from prefix.
	s, suffixOk := trimFqdn(question.Name, d.domains)
	if !suffixOk {
		logger.Warn("invalid domain", zap.String("qname", question.Name), zap.Stringer("from", w.RemoteAddr()))
		d.errCounter.Inc()
		return reject(q, dns.RcodeRefused)
	}

	if strings.ContainsRune(s, '-') {
		s = strings.ReplaceAll(s, "-", ":")
	}
	addr, err := netip.ParseAddr(s)
	if err != nil {
		logger.Warn("invalid ip", zap.String("qname", question.Name), zap.Stringer("from", w.RemoteAddr()))
		d.errCounter.Inc()
		return reject(q, dns.RcodeNameError)
	}

	logger.Info(
		"incoming query",
		zap.String("qname", question.Name),
		zap.Uint16("qtype", question.Qtype),
		zap.Stringer("ip", addr),
		zap.Stringer("from", w.RemoteAddr()),
	)

	r := new(dns.Msg)
	r.SetReply(q)
	r.Authoritative = true
	switch question.Qtype {
	case dns.TypeA:
		if !addr.Is4() {
			break
		}
		r.Answer = append(r.Answer, &dns.A{
			Hdr: dns.RR_Header{
				Name:   question.Name,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    3600 * 24,
			},
			A: addr.AsSlice(),
		})
	case dns.TypeAAAA:
		if !addr.Is6() {
			break
		}
		r.Answer = append(r.Answer, &dns.AAAA{
			Hdr: dns.RR_Header{
				Name:   question.Name,
				Rrtype: dns.TypeAAAA,
				Class:  dns.ClassINET,
				Ttl:    3600 * 24,
			},
			AAAA: addr.AsSlice(),
		})
	}
	return r
}

func trimFqdn(fqdn string, domains map[string]struct{}) (string, bool) {
	var suffixOk bool
	for off, end := 0, false; !end; off, end = dns.NextLabel(fqdn, off) {
		suffix := fqdn[off:]
		if _, suffixOk = domains[suffix]; suffixOk {
			if off > 0 {
				return fqdn[:off-1], true
			}
			return "", true
		}
	}
	return "", false
}

func reject(q *dns.Msg, rcode int) *dns.Msg {
	r := new(dns.Msg)
	r.SetRcode(q, rcode)
	return r
}

func mustInitLogger() *zap.Logger {
	l, err := zap.NewProduction(zap.WithCaller(false))
	if err != nil {
		panic(fmt.Sprintf("init logger: %s", err))
	}
	return l
}
