package vcloud

import (
	"encoding/xml"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
)

type LbProtocol string

const (
	HTTP  LbProtocol = "HTTP"
	HTTPS            = "HTTPS"
	TCP              = "TCP"
	UDP              = "UDP"
)

var LbProtocols = []LbProtocol{
	HTTP,
	HTTPS,
	TCP,
	UDP,
}

type LbAlgorithm string

const (
	ROUND_ROBIN LbAlgorithm = "ROUND_ROBIN"
	IP_HASH                 = "IP_HASH"
	LEASTCONN               = "LEASTCONN"
	URI                     = "URI"
	HTTPHEADER              = "HTTPHEADER"
	URL                     = "URL"
)

type IpAddress struct {
	XMLName   xml.Name `xml:"IpAddress"`
	IpAddress string   `xml:"IpAddress"`
}

type IpAddressAllocation struct {
	XMLName   xml.Name `xml:"AllocatedIpAddresses"`
	IpAddress []IpAddress
}

type FirewallConfig struct {
	name string
	Source      types.EdgeFirewallEndpoint
	Destination types.EdgeFirewallEndpoint
	Application types.EdgeFirewallApplication
}

type Config struct {
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Org      string `yaml:"org"`
	Href     string `yaml:"href"`
	VDC      string `yaml:"vdc"`
	Insecure bool `yaml:"insecure"`
	EdgeGateway string `yaml:"edgeGateway"`
}
