package vcloud

import (
	"crypto/sha1"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
	"io/ioutil"
	"k8s.io/klog"
	"net"
	"net/http"
	"net/url"
	"time"
)

const (
	VirtualServerDescription string = "This Service was automatically created and managed by vCloud-cloud-controller-manager"
	PoolDescription          string = "This Pool was automatically created and managed by vCloud-cloud-controller-manager"
)

var (
	ErrNotFound           = errors.New("not found")
	cachedVCDClients      = &cacheStorage{conMap: make(map[string]cachedConnection)}
	maxConnectionValidity = 20 * time.Minute
)

func (v *vCloud) getClient(forceRefresh bool) (*govcd.VCDClient, error) {
	klog.Infof("getClient() called")
	rawData := v.cfg.User + "#" +
		v.cfg.Password + "#" +
		v.cfg.VDC + "#" +
		v.cfg.Org + "#" +
		v.cfg.Href
	checksum := fmt.Sprintf("%x", sha1.Sum([]byte(rawData)))

	//LOCK
	cachedVCDClients.Lock()
	client, ok := cachedVCDClients.conMap[checksum]
	cachedVCDClients.Unlock()
	if ok {
		cachedVCDClients.Lock()
		cachedVCDClients.cacheClientServedCount += 1
		cachedVCDClients.Unlock()
		elapsed := time.Since(client.initTime)
		// Delete cached Connection when forcing a Refresh
		if (elapsed > maxConnectionValidity) || forceRefresh {
			klog.V(5).Infof("cached connection invalidated after %2.0f minutes \n", maxConnectionValidity.Minutes())
			cachedVCDClients.Lock()
			delete(cachedVCDClients.conMap, checksum)
			cachedVCDClients.Unlock()
		} else {
			return client.connection, nil
		}
	}

	u, err := url.ParseRequestURI(v.cfg.Href)
	if err != nil {
		return nil, fmt.Errorf("unable to pass url: %s", err)
	}

	vcdclient := govcd.NewVCDClient(*u, v.cfg.Insecure)
	klog.V(4).Info("Logging into vCloud")
	err = vcdclient.Authenticate(v.cfg.User, v.cfg.Password, v.cfg.Org)
	if err != nil {
		return nil, fmt.Errorf("unable to authenticate: %s", err)
	}

	cachedVCDClients.Lock()
	cachedVCDClients.conMap[checksum] = cachedConnection{initTime: time.Now(), connection: vcdclient}
	cachedVCDClients.Unlock()

	return vcdclient, nil
}

func (loadBalancer *LB) GetLoadBalancerByName(name string) (*types.LbVirtualServer, error) {
	gateway, err := loadBalancer.getEdgeGateway()
	if err != nil {
		return nil, err
	}
	vservers, err := gateway.GetLbVirtualServers()
	if err != nil {
		return nil, err
	}
	for _, vserver := range vservers {
		if vserver.Name == name {
			return vserver, nil
		}
	}
	return nil, ErrNotFound
}

func (loadBalancer *LB) GetLoadBalancerPool(name string) (*types.LbPool, error) {
	gateway, err := loadBalancer.getEdgeGateway()
	if err != nil {
		return nil, err
	}
	pools, err := gateway.GetLbServerPools()
	if err != nil {
		return nil, err
	}
	for _, pool := range pools {
		if pool.Name == name {
			return pool, nil
		}
	}
	return nil, ErrNotFound
}

func (loadBalancer *LB) CreatePool(pool *types.LbPool) (*types.LbPool, error) {
	gateway, err := loadBalancer.getEdgeGateway()
	if err != nil {
		return nil, err
	}
	pool, err = gateway.CreateLbServerPool(pool)
	if err != nil {
		return nil, err
	}

	return pool, nil
}

func (loadBalancer *LB) UpdatePool(pool *types.LbPool) (*types.LbPool, error) {
	gateway, err := loadBalancer.getEdgeGateway()
	if err != nil {
		return nil, err
	}
	pool, err = gateway.UpdateLbServerPool(pool)
	if err != nil {
		return nil, err
	}
	return pool, nil
}

func (loadBalancer *LB) CreateVirtualServer(name string, ip string, protocol LbProtocol, port int, applicationProfile string, poolName string) (*types.LbVirtualServer, error) {
	gateway, err := loadBalancer.getEdgeGateway()
	if err != nil {
		return nil, err
	}
	lbAppProfile, err := gateway.GetLbAppProfileByName(applicationProfile)
	if err != nil {
		return nil, err
	}

	lbPool, err := loadBalancer.GetLoadBalancerPool(poolName)
	if err != nil {
		return nil, err
	}

	//TODO: Add support for configuring connection limits.
	vServer, err := gateway.CreateLbVirtualServer(&types.LbVirtualServer{
		Name:                 name,
		Description:          VirtualServerDescription,
		Enabled:              true,
		IpAddress:            ip,
		Protocol:             string(protocol),
		Port:                 port,
		ConnectionLimit:      0,
		ConnectionRateLimit:  0,
		ApplicationProfileId: lbAppProfile.ID,
		DefaultPoolId:        lbPool.ID,
	})

	return vServer, nil
}

func (loadBalancer *LB) getPublicIPAddressesFromEdgeGateway(gateway *govcd.EdgeGateway) (string, string, error) {
	gatewayInterface := gateway.EdgeGateway.Configuration.GatewayInterfaces.GatewayInterface[0]
	startPublicAddress := gatewayInterface.SubnetParticipation[0].IPRanges.IPRange[0].StartAddress
	endPublicAddress := gatewayInterface.SubnetParticipation[0].IPRanges.IPRange[0].EndAddress
	return startPublicAddress, endPublicAddress, nil
}

func (loadBalancer *LB) GetNextAvailableIpAddressInVCloudNet(networkName string, ipnet string) (string, error) {
	allocatedIps, err := loadBalancer.vCloud.getAllocatedIPAddresses(networkName)
	if err != nil {
		return "", err
	}
	var ips []string

	for _, ip := range allocatedIps.IpAddress {
		ips = append(ips, ip.IpAddress)
	}

	_, ipv4Net, err := validateNetwork(ipnet)
	if err != nil {
		return "", err
	}

	mask := binary.BigEndian.Uint32(ipv4Net.Mask)
	start := binary.BigEndian.Uint32(ipv4Net.IP)

	end := (start & mask) | (mask ^ 0xffffffff)

	for i := start + 1; i <= end-1; i++ {
		ip := make(net.IP, 4)
		binary.BigEndian.PutUint32(ip, i)

		if !contains(ips, ip.String()) {
			return ip.String(), nil
		}

	}

	return "", errors.New("no IP addresses left")
}

func (v *vCloud) getAllocatedIPAddresses(name string) (*IpAddressAllocation, error) {
	vclient, err := v.getClient(false)
	if err != nil {
		fmt.Println(err)
	}
	token := vclient.Client.VCDToken
	network, err := v.getNetworkByName(name)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 30 * time.Second}

	requestUrl := fmt.Sprintf("%s/allocatedAddresses", network.OrgVDCNetwork.HREF)

	req, err := http.NewRequest("GET", requestUrl, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("x-vCloud-authorization", token)
	req.Header.Add("Accept", fmt.Sprintf("application/*+xml;version=%s", vclient.Client.APIVersion))

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	//TODO: Check potential errors raised by Close()
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var ipAddressAllocation IpAddressAllocation

	err = xml.Unmarshal(body, &ipAddressAllocation)
	if err != nil {
		return nil, err
	}

	return &ipAddressAllocation, nil
}

func (v *vCloud) getNetworkByName(name string) (*govcd.OrgVDCNetwork, error) {
	vdc, err := v.getVDC()
	if err != nil {
		return nil, err
	}
	network, err := vdc.GetOrgVdcNetworkByName(name, true)
	if err != nil {
		klog.Errorf("no such network found with name: %s", name)
		return nil, err
	}
	return network, nil
}

func (loadBalancer *LB) GetFirewallRule(name string) (*types.EdgeFirewallRule, error) {
	gateway, err := loadBalancer.getEdgeGateway()
	if err != nil {
		return nil, err
	}
	rules, err := gateway.GetAllNsxvFirewallRules()
	if err != nil {
		return nil, err
	}
	for _, rule := range rules {
		if rule.Name == name {
			return rule, nil
		}
	}
	return nil, ErrNotFound
}

func (loadBalancer *LB) DeleteLbVirtualServerById(id string) error {
	gateway, err := loadBalancer.getEdgeGateway()
	if err != nil {
		return err
	}
	err = gateway.DeleteLbVirtualServerById(id)
	return err
}

func (loadBalancer *LB) DeleteLbServerPoolById(id string) error {
	gateway, err := loadBalancer.getEdgeGateway()
	if err != nil {
		return err
	}
	err = gateway.DeleteLbServerPoolById(id)
	return err
}

func (loadBalancer *LB) DeleteFirewallRule(rule *types.EdgeFirewallRule) (bool, error) {
	gateway, err := loadBalancer.getEdgeGateway()
	if err != nil {
		return false, err
	}
	err = gateway.DeleteNsxvFirewallRuleById(rule.ID)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (loadBalancer *LB) createFirewallRule(rule *FirewallConfig) error {
	gateway, err := loadBalancer.getEdgeGateway()
	if err != nil {
		return err
	}
	_, err = gateway.CreateNsxvFirewallRule(&types.EdgeFirewallRule{
		Name:           rule.name,
		RuleType:       "User",
		Source:         rule.Source,
		Destination:    rule.Destination,
		Application:    rule.Application,
		Action:         "Accept",
		Enabled:        true,
		LoggingEnabled: false,
	}, "")
	if err != nil {
		return err
	}

	return nil
}

func (v *vCloud) getVDC() (*govcd.Vdc, error) {
	client, err := v.getClient(false)
	if err != nil {
		return nil, err
	}
	org, err := client.GetOrgByName(v.cfg.Org)
	if err != nil {
		return nil, err
	}
	vdc, err := org.GetVDCByName(v.cfg.VDC, true)
	if err != nil {
		return nil, err
	}

	return vdc, nil
}

func (loadBalancer *LB) getEdgeGateway() (*govcd.EdgeGateway, error) {
	client, err := loadBalancer.vCloud.getClient(false)
	if err != nil {
		return nil, err
	}
	org, err := client.GetOrgByName(loadBalancer.vCloud.cfg.Org)
	if err != nil {
		return nil, err
	}
	vdc, err := org.GetVDCByName(loadBalancer.vCloud.cfg.VDC, true)
	if err != nil {
		return nil, err
	}
	edge, err := vdc.GetEdgeGatewayByName(loadBalancer.vCloud.cfg.EdgeGateway, true)
	if err != nil {
		return nil, err
	}

	return edge, nil
}

func (v *vCloud) getEdgeGateway(orgName string, vdcName string, gatewayName string) (*govcd.EdgeGateway, error) {
	client, err := v.getClient(false)
	if err != nil {
		return nil, err
	}
	org, err := client.GetOrgByName(orgName)
	if err != nil {
		return nil, err
	}
	vdc, err := org.GetVDCByName(vdcName, true)
	if err != nil {
		return nil, err
	}
	edge, err := vdc.GetEdgeGatewayByName(gatewayName, true)
	if err != nil {
		return nil, err
	}

	return edge, nil
}
