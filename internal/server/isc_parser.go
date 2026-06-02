package server

import (
	"bufio"
	"net"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type iscRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type iscSubnet struct {
	CIDR       string            `json:"cidr"`
	Gateway    string            `json:"gateway"`
	DNS        []string          `json:"dns"`
	RangeStart string            `json:"rangeStart"`
	RangeEnd   string            `json:"rangeEnd"`
	Ranges     []iscRange        `json:"ranges"`
	Exclusions []string          `json:"exclusions"`
	Options    map[string]string `json:"options"`
	RawBlock   string            `json:"rawBlock"`
}

type iscReservation struct {
	Hostname string `json:"hostname"`
	MAC      string `json:"mac"`
	IP       string `json:"ip"`
}

type iscDeniedHost struct {
	Hostname string `json:"hostname"`
	MAC      string `json:"mac"`
	Reason   string `json:"reason,omitempty"`
}

type iscParseResult struct {
	Subnets      []iscSubnet      `json:"subnets"`
	Reservations []iscReservation `json:"reservations"`
	DeniedHosts  []iscDeniedHost  `json:"deniedHosts"`
	Warnings     []string         `json:"warnings"`
}

var (
	reSubnetStart   = regexp.MustCompile(`(?i)^subnet\s+([\d\.]+)\s+netmask\s+([\d\.]+)\s*\{`)
	reRange         = regexp.MustCompile(`(?i)range\s+([\d\.]+)\s+([\d\.]+)`)          // note: dhcpd requires semicolon; tolerant
	reOptionRouter  = regexp.MustCompile(`(?i)option\s+routers\s+([^;]+)`)             // gateway
	reOptionDNS     = regexp.MustCompile(`(?i)option\s+domain-name-servers\s+([^;]+)`) // dns list
	reOptionGeneric = regexp.MustCompile(`(?i)option\s+([\w-]+)\s+([^;]+)`)            // generic capture
	reOption43Text  = regexp.MustCompile(`(?i)serverip\s*=\s*([\d\.]+)`)               // vendor option43 server ip
	reHostStart     = regexp.MustCompile(`(?i)^host\s+([\w-]+)\s*\{`)
	reHwEther       = regexp.MustCompile(`(?i)hardware\s+ethernet\s+([\w:]+)`) // mac
	reFixedAddr     = regexp.MustCompile(`(?i)fixed-address\s+([\d\.]+)`)      // ip
	reDenyBoot      = regexp.MustCompile(`(?i)deny\s+booting`)                 // mac blacklist
)

func parseISCDHCPConf(content string) iscParseResult {
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 0, 1024*1024), 8*1024*1024)
	var result iscParseResult
	var globalDNS []string
	globalOptions := make(map[string]string)
	var current *iscSubnet
	var hostName string
	var hostMAC string
	var hostIP string
	var hostDeny bool

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") { // skip comments
			continue
		}
		// normalize semicolon spacing
		if m := reSubnetStart.FindStringSubmatch(line); len(m) == 3 {
			cidr := toCIDR(m[1], m[2])
			if cidr == "" {
				result.Warnings = append(result.Warnings, "无法解析子网/掩码: "+line)
				continue
			}
			block := iscSubnet{CIDR: cidr, Options: make(map[string]string)}
			result.Subnets = append(result.Subnets, block)
			current = &result.Subnets[len(result.Subnets)-1]
			continue
		}

		if current == nil {
			// global options when not inside a subnet
			if m := reOptionDNS.FindStringSubmatch(line); len(m) == 2 {
				dnsList := strings.Split(trimTrailingSemicolon(m[1]), ",")
				for i := range dnsList {
					dnsList[i] = strings.TrimSpace(dnsList[i])
				}
				globalDNS = dnsList
				continue
			}
			if m := reOptionGeneric.FindStringSubmatch(line); len(m) == 3 {
				key := strings.ToLower(strings.TrimSpace(m[1]))
				val := strings.TrimSpace(trimTrailingSemicolon(m[2]))
				globalOptions[key] = val
				if ip := parseOption43ServerIP(val); ip != "" {
					globalOptions["option-43"] = ip
				}
				continue
			}
		}

		// inside subnet block until '}'
		if current != nil {
			if strings.Contains(line, "}") {
				current.Exclusions = computeExclusions(current.Ranges)
				current = nil
				continue
			}
			if m := reRange.FindStringSubmatch(line); len(m) == 3 {
				r := iscRange{Start: m[1], End: m[2]}
				current.Ranges = append(current.Ranges, r)
				if current.RangeStart == "" {
					current.RangeStart = m[1]
					current.RangeEnd = m[2]
				}
			}
			if m := reOptionRouter.FindStringSubmatch(line); len(m) == 2 {
				current.Gateway = strings.TrimSpace(trimTrailingSemicolon(m[1]))
				continue
			}
			if m := reOptionDNS.FindStringSubmatch(line); len(m) == 2 {
				dnsList := strings.Split(trimTrailingSemicolon(m[1]), ",")
				for i := range dnsList {
					dnsList[i] = strings.TrimSpace(dnsList[i])
				}
				current.DNS = dnsList
				continue
			}
			if m := reOptionGeneric.FindStringSubmatch(line); len(m) == 3 {
				key := strings.ToLower(strings.TrimSpace(m[1]))
				val := strings.TrimSpace(trimTrailingSemicolon(m[2]))
				if key == "subnet-mask" {
					// subnet-mask is redundant with the subnet declaration; skip it
					continue
				}
				if current.Options == nil {
					current.Options = make(map[string]string)
				}
				current.Options[key] = val
				if ip := parseOption43ServerIP(val); ip != "" {
					current.Options["option-43"] = ip
				}
				continue
			}
			continue
		}

		// host reservation
		if m := reHostStart.FindStringSubmatch(line); len(m) == 2 {
			hostName = strings.TrimSpace(m[1])
			hostMAC = ""
			hostIP = ""
			hostDeny = false
			continue
		}
		if hostName != "" {
			if strings.Contains(line, "}") {
				if hostDeny && hostMAC != "" {
					result.DeniedHosts = append(result.DeniedHosts, iscDeniedHost{Hostname: hostName, MAC: hostMAC, Reason: "deny booting"})
				} else if hostMAC != "" || hostIP != "" {
					result.Reservations = append(result.Reservations, iscReservation{Hostname: hostName, MAC: hostMAC, IP: hostIP})
				}
				hostName, hostMAC, hostIP, hostDeny = "", "", "", false
				continue
			}
			if m := reHwEther.FindStringSubmatch(line); len(m) == 2 {
				hostMAC = strings.ToLower(strings.TrimSpace(trimTrailingSemicolon(m[1])))
			}
			if m := reFixedAddr.FindStringSubmatch(line); len(m) == 2 {
				hostIP = strings.TrimSpace(trimTrailingSemicolon(m[1]))
			}
			if reDenyBoot.MatchString(line) {
				hostDeny = true
			}
		}
	}

	// apply global fallbacks
	for i := range result.Subnets {
		sub := &result.Subnets[i]
		if len(sub.DNS) == 0 && len(globalDNS) > 0 {
			sub.DNS = append([]string(nil), globalDNS...)
		}
		if g43, ok := globalOptions["option-43"]; ok && strings.TrimSpace(g43) != "" {
			if sub.Options == nil {
				sub.Options = make(map[string]string)
			}
			if strings.TrimSpace(sub.Options["option-43"]) == "" {
				sub.Options["option-43"] = g43
			}
		}
	}
	return result
}

func toCIDR(network, netmask string) string {
	ip := net.ParseIP(network)
	maskIP := net.ParseIP(netmask)
	if ip == nil || maskIP == nil {
		return ""
	}
	mask := net.IPMask(maskIP.To4())
	if mask == nil {
		return ""
	}
	ones, _ := mask.Size()
	return ip.String() + "/" + strconv.Itoa(ones)
}

func trimTrailingSemicolon(s string) string {
	return strings.TrimSuffix(strings.TrimSpace(s), ";")
}

type numericRange struct {
	start uint32
	end   uint32
	orig  iscRange
}

func computeExclusions(ranges []iscRange) []string {
	if len(ranges) == 0 {
		return nil
	}
	spans := make([]numericRange, 0, len(ranges))
	for _, r := range ranges {
		start, ok1 := ipv4ToUint32(r.Start)
		end, ok2 := ipv4ToUint32(r.End)
		if !ok1 || !ok2 || end < start {
			continue
		}
		spans = append(spans, numericRange{start: start, end: end, orig: r})
	}
	if len(spans) == 0 {
		return nil
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	var exclusions []string
	curEnd := spans[0].end
	for i := 1; i < len(spans); i++ {
		s := spans[i]
		if s.start > curEnd+1 {
			gapStart := curEnd + 1
			gapEnd := s.start - 1
			exclusions = append(exclusions, formatRange(gapStart, gapEnd))
		}
		if s.end > curEnd {
			curEnd = s.end
		}
	}
	return exclusions
}

func ipv4ToUint32(ipStr string) (uint32, bool) {
	ip, err := netip.ParseAddr(strings.TrimSpace(ipStr))
	if err != nil || !ip.Is4() {
		return 0, false
	}
	seg := ip.As4()
	return uint32(seg[0])<<24 | uint32(seg[1])<<16 | uint32(seg[2])<<8 | uint32(seg[3]), true
}

func formatRange(start, end uint32) string {
	if start == end {
		return uint32ToIPv4(start)
	}
	return uint32ToIPv4(start) + " - " + uint32ToIPv4(end)
}

func uint32ToIPv4(n uint32) string {
	return net.IPv4(byte(n>>24), byte(n>>16), byte(n>>8), byte(n)).String()
}

// parseOption43ServerIP attempts to extract the vendor server IP from option43 strings.
func parseOption43ServerIP(val string) string {
	clean := strings.Trim(strings.TrimSpace(val), "\"'")
	if clean == "" {
		return ""
	}
	if m := reOption43Text.FindStringSubmatch(clean); len(m) == 2 {
		if ip := net.ParseIP(strings.TrimSpace(m[1])); ip != nil {
			return ip.String()
		}
	}

	// try TLV hex format (e.g. 01:04:0a:00:00:01)
	parts := strings.FieldsFunc(clean, func(r rune) bool {
		return r == ':' || r == ' ' || r == ','
	})
	if len(parts) >= 6 { // minimal TLV with type(1) length(1) + 4 bytes
		buf := make([]byte, 0, len(parts))
		for _, p := range parts {
			if len(p) == 0 {
				continue
			}
			num, err := strconv.ParseUint(p, 16, 8)
			if err != nil {
				buf = nil
				break
			}
			buf = append(buf, byte(num))
		}
		if len(buf) >= 6 {
			for i := 0; i+1 < len(buf); {
				typ := buf[i]
				length := int(buf[i+1])
				i += 2
				if i+length > len(buf) {
					break
				}
				if typ == 1 && length == 4 {
					ipBytes := buf[i : i+length]
					return net.IP(ipBytes).String()
				}
				i += length
			}
		}
	}

	return ""
}
