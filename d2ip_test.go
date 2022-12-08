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
	"strings"
	"testing"
)

func Test_trimFqdn(t *testing.T) {
	type args struct {
		fqdn    string
		domains string
	}
	tests := []struct {
		name         string
		args         args
		want         string
		wantSuffixOk bool
	}{
		{name: "ipv4, ok", args: args{fqdn: "1.2.3.4.5.6.", domains: "4.5.6"}, want: "1.2.3", wantSuffixOk: true},
		{name: "ipv4, invalid suffix", args: args{fqdn: "1.2.3.4.5.6.", domains: "789"}, want: "", wantSuffixOk: false},
		{name: "ipv6, ok", args: args{fqdn: "2000--1.4.5.6.", domains: "4.5.6"}, want: "2000--1", wantSuffixOk: true},
		{name: "ipv6, invalid suffix", args: args{fqdn: "2000--1.4.5.6.", domains: "789"}, want: "", wantSuffixOk: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suffixes := make(map[string]struct{})
			for _, s := range strings.Split(tt.args.domains, ",") {
				s = dns.Fqdn(s)
				suffixes[s] = struct{}{}
			}
			got, suffixOk := trimFqdn(tt.args.fqdn, suffixes)
			if got != tt.want {
				t.Errorf("trimFqdn() got = %v, want %v", got, tt.want)
			}
			if suffixOk != tt.wantSuffixOk {
				t.Errorf("trimFqdn() suffixOk = %v, want %v", suffixOk, tt.wantSuffixOk)
			}
		})
	}
}
