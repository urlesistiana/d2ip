/*
 * Copyright (C) 2020-2022, IrineSistiana
 *
 * This file is part of mosdns.
 *
 * mosdns is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * mosdns is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package main

import (
	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"net/netip"
	"strings"
)

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
	if len(q.Question) != 1 || q.Question[0].Qclass != dns.ClassINET {
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
