package enrich

import (
	"errors"
	"net"
	"strings"

	"github.com/oschwald/geoip2-golang"
)

type Geo struct {
	Country string
	Region  string
	City    string
	ASNOrg  string
}

type GeoIP struct {
	city *geoip2.Reader
	asn  *geoip2.Reader
}

func NewGeoIP(cityPath, asnPath string) (*GeoIP, error) {
	cityPath = strings.TrimSpace(cityPath)
	asnPath = strings.TrimSpace(asnPath)
	if cityPath == "" && asnPath == "" {
		return nil, nil
	}
	g := &GeoIP{}
	var err error
	if cityPath != "" {
		g.city, err = geoip2.Open(cityPath)
		if err != nil {
			return nil, err
		}
	}
	if asnPath != "" {
		g.asn, err = geoip2.Open(asnPath)
		if err != nil {
			if g.city != nil {
				g.city.Close()
			}
			return nil, err
		}
	}
	return g, nil
}

func (g *GeoIP) Close() error {
	if g == nil {
		return nil
	}
	var first error
	if g.city != nil {
		if err := g.city.Close(); err != nil && first == nil {
			first = err
		}
	}
	if g.asn != nil {
		if err := g.asn.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (g *GeoIP) Lookup(ipStr string) (Geo, bool) {
	if g == nil {
		return Geo{}, false
	}
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return Geo{}, false
	}

	out := Geo{}
	ok := false

	if g.city != nil {
		if rec, err := g.city.City(ip); err == nil {
			if rec.Country.IsoCode != "" {
				out.Country = rec.Country.IsoCode
				ok = true
			}
			if len(rec.Subdivisions) > 0 && rec.Subdivisions[0].IsoCode != "" {
				out.Region = rec.Subdivisions[0].IsoCode
				ok = true
			}
			if rec.City.Names != nil {
				if name := rec.City.Names["en"]; strings.TrimSpace(name) != "" {
					out.City = name
					ok = true
				}
			}
		}
	}
	if g.asn != nil {
		if rec, err := g.asn.ASN(ip); err == nil {
			if strings.TrimSpace(rec.AutonomousSystemOrganization) != "" {
				out.ASNOrg = strings.TrimSpace(rec.AutonomousSystemOrganization)
				ok = true
			}
		}
	}

	return out, ok
}

var ErrGeoIPNotConfigured = errors.New("geoip not configured")

