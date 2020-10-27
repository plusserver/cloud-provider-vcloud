package vcloud

import (
	"fmt"
	"github.com/ghodss/yaml"
	"io"
	"io/ioutil"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog"
	"os"
)

const (
	ProviderName = "vCloud"
)

type vCloud struct {
	cfg *Config
}

type LoadBalancerOptions struct {
	LBVersion string
}

func (v *vCloud) Initialize(clientBuilder cloudprovider.ControllerClientBuilder, stop <-chan struct{}) {
}

func (v *vCloud) LoadBalancer() (cloudprovider.LoadBalancer, bool) {
	klog.V(4).Info("vCloud.LoadBalancerOptions() called")

	// Starts Caching
	_, err := v.getClient(false)
	if err != nil {
		klog.Error(err)
		os.Exit(1)
	}

	_, err = v.getEdgeGateway(v.cfg.Org, v.cfg.VDC, v.cfg.EdgeGateway)
	if err != nil {
		klog.Errorf("Can not find Edge Gateway under name: ", v.cfg.EdgeGateway)
		os.Exit(1)
	}

	return &LB{
		vCloud:              v,
		LoadBalancerOptions: LoadBalancerOptions{LBVersion: "v123"},
		keyLock:             nil,
	}, true
}

func (v *vCloud) Instances() (cloudprovider.Instances, bool) {
	return nil, false
}

func (v *vCloud) Zones() (cloudprovider.Zones, bool) {
	return nil, false
}

func (v *vCloud) Clusters() (cloudprovider.Clusters, bool) {
	return nil, false
}

func (v *vCloud) Routes() (cloudprovider.Routes, bool) {
	return nil, false
}

func (v *vCloud) ProviderName() string {
	return ProviderName
}

func (v *vCloud) HasClusterID() bool {
	return true
}

func newVCloud(cfg *Config) (*vCloud, error) {
	vcloud := vCloud{
		cfg: cfg,
	}

	return &vcloud, nil
}

func ReadConfig(config io.Reader) (*Config, error) {

	if config == nil {
		return nil, fmt.Errorf("no vCloud cloud provider Config given")
	}

	var cfg Config

	file, err := ioutil.ReadAll(config)
	if err != nil {
		return nil, fmt.Errorf("error reading cloud-config: %s", err)
	}

	err = yaml.Unmarshal(file, &cfg)
	if err != nil {
		return nil, fmt.Errorf("error unmarshelling cloud-config: %s", err)
	}

	klog.V(5).Infof("Config, loaded from cloud-config")

	return &cfg, nil
}

func init() {
	cloudprovider.RegisterCloudProvider(ProviderName, func(config io.Reader) (cloudprovider.Interface, error) {
		cfg, err := ReadConfig(config)
		if err != nil {
			return nil, err
		}
		cloud, err := newVCloud(cfg)
		if err != nil {
			klog.V(1).Infof("New vCloud client created failed with config")
		}
		return cloud, err
	})
	klog.V(1).Infof("Registered cloud provider with name: %s", ProviderName)
}
