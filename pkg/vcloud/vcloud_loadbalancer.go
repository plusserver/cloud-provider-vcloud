package vcloud

import (
	"context"
	"errors"
	"fmt"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog"
	nodeutil "k8s.io/kubernetes/pkg/util/node"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	IsWorkerNode = "node-role.kubernetes.io/worker"

	LoadBalancerType                     = "mk.plus.io/load-balancer-type"
	LoadBalancerExternalIP               = "mk.plus.io/load-balancer-external-ip"
	LoadBalancerPoolAlgorithm            = "mk.plus.io/pool-algorithm"
	LoadBalancerPoolMemberMinConnections = "mk.plus.io/pool-min-con"
	LoadBalancerPoolMemberMaxConnections = "mk.plus.io/pool-max-con"
)

type LB struct {
	vCloud *vCloud
	LoadBalancerOptions
	keyLock *keyLock
}

//getStringFromServiceAnnotation searches a given v1.Service for a specific annotationKey and either returns the annotation's value or a specified defaultSetting
func getStringFromServiceAnnotation(service *corev1.Service, annotationKey string, defaultSetting string) string {
	if annotationValue, ok := service.Annotations[annotationKey]; ok {
		klog.V(4).Infof("Found a Service Annotation: %v = %v", annotationKey, annotationValue)
		return annotationValue
	}
	klog.V(4).Infof("Could not find a Service Annotation; falling back on cloud-config setting: %v = %v", annotationKey, defaultSetting)
	return strings.ToLower(defaultSetting)
}

func getIntFromServiceAnnotation(service *corev1.Service, annotationKey string) (int, bool) {
	intString := getStringFromServiceAnnotation(service, annotationKey, "0")
	if len(intString) > 0 {
		annotationValue, err := strconv.Atoi(intString)
		if err == nil {
			klog.V(4).Infof("Found a Service Annotation: %v = %v", annotationKey, annotationValue)
			return annotationValue, true
		}
	}
	return 0, false
}

func (loadBalancer *LB) GetLoadBalancer(ctx context.Context, clusterName string, service *corev1.Service) (status *corev1.LoadBalancerStatus, exists bool, err error) {
	klog.V(4).Infof("GetLoadBalancer: called with clusterName %s", clusterName)
	name := loadBalancer.GetLoadBalancerName(ctx, clusterName, service)
	status = &corev1.LoadBalancerStatus{}

	for _, port := range service.Spec.Ports {

		lbName := fmt.Sprintf("%s-%d", name, port.NodePort)
		lb, err := loadBalancer.GetLoadBalancerByName(lbName)
		if errors.Is(ErrNotFound, err) {
			klog.V(4).Infof("Could not find loadBalancer by name: %s", lbName)
			return nil, false, err
		} else if err != nil {
			klog.V(4).Infof("Error fetching loadBalancer by name: %s err: %s", lbName, err.Error())
			return nil, false, err
		}

		status.Ingress = append(status.Ingress, corev1.LoadBalancerIngress{IP: lb.IpAddress})
	}

	return status, true, nil
}

func (loadBalancer *LB) GetLoadBalancerName(ctx context.Context, clusterName string, service *corev1.Service) string {
	klog.V(4).Infof("GetLoadBalancerName: called with clusterName %s", clusterName)
	name := fmt.Sprintf("kube_service_%s_%s_%s", clusterName, service.Namespace, service.Name)
	klog.V(4).Infof("GetLoadBalancerName: registered Name: %s", name)
	return cutString(name)
}

func (loadBalancer *LB) getPoolName(ctx context.Context, clusterName string, service *corev1.Service, nodePort int32) string {
	klog.V(4).Infof("getPoolName: called with clusterName %s", clusterName)
	name := fmt.Sprintf("kube_pool_%s_%s_%s_%d", clusterName, service.Namespace, service.Name, nodePort)
	klog.V(4).Infof("getPoolName: registered Name: %s", name)
	return cutString(name)
}

func (loadBalancer *LB) createMember(port corev1.ServicePort, service *corev1.Service, node *corev1.Node) (*types.LbPoolMember, error) {
	//TODO: GetNodeHostIP also returns the external IP if it cant find the internal IP first. Is that what we want?!
	nodeIp, err := nodeutil.GetNodeHostIP(node)
	if err != nil {
		return nil, fmt.Errorf("error retrieving internal ip or externalIP from node: %s err:%s", node.GetName(), err.Error())
	}
	minCon, _ := getIntFromServiceAnnotation(service, LoadBalancerPoolMemberMinConnections)
	maxCon, _ := getIntFromServiceAnnotation(service, LoadBalancerPoolMemberMaxConnections)

	member := types.LbPoolMember{
		//NOTE: vShield Edge [LoadBalancer] Invalid member name: 10.13.37.22, valid member name should contain letters, digits, dash, underscore and must start with a letter (API error: 14571)
		//TODO: Better naming convention for loadBalancer pool members
		//NOTE: For now we will use member-10133720-nodePort since dots are not supported.
		//TODO: Implement Validation for API Error 14571 (valid member name should contain letters, digits, dash, underscore and must start with a letter)
		Name: fmt.Sprintf("member-%s-%d", strings.ReplaceAll(nodeIp.String(), ".", ""), port.NodePort),
		IpAddress:   nodeIp.String(),
		Weight:      1,
		MonitorPort: int(port.Port),
		Port:        int(port.NodePort),
		MaxConn:     maxCon,
		MinConn:     minCon,
		Condition:   "enabled",
	}

	return &member, nil
}

func (loadBalancer *LB) EnsureLoadBalancer(ctx context.Context, clusterName string, service *corev1.Service, nodes []*corev1.Node) (*corev1.LoadBalancerStatus, error) {
	klog.V(4).Infof("EnsureLoadBalancer: called with clusterName %s", clusterName)
	serviceName := loadBalancer.GetLoadBalancerName(ctx, clusterName, service)

	var lb *types.LbVirtualServer
	var vServerIP string
	var err error
	var poolUpdateRequired bool

	if len(nodes) == 0 {
		return nil, fmt.Errorf("there are no available nodes for LoadBalancer service %s", serviceName)
	}

	ports := service.Spec.Ports
	if len(ports) == 0 {
		return nil, fmt.Errorf("no ports provided to vCloud load balancer")
	}

	//Determine LB Type
	//NOTE: Defaults to internal loadBalancer
	if getStringFromServiceAnnotation(service, LoadBalancerType, "internal") == "external" {
		//External LB
		externalIP := getStringFromServiceAnnotation(service, LoadBalancerExternalIP, "")
		if externalIP == "" {
			return nil, fmt.Errorf("%s Annotation is required for external type Loadbalancer", LoadBalancerExternalIP)
		}
		vServerIP = externalIP
	} else {
		//Fetch IP Address of vServer
		//NOTE: Turns out that you can have multiple vServer on the same IP address but different ports which makes it easier
		//TODO: Retrieve IPNet from Worker VM via vCloud. Easiest way would be to just label the workers. RKE already adds Internal IP but no Subnet
		//TODO: Retrieve Network Name somehow maybe labeling?
		vServerIP, err = loadBalancer.GetNextAvailableIpAddressInVCloudNet(os.Getenv("VCLOUD_VDC_NETWORK_NAME"), os.Getenv("VCLOUD_VDC_NETWORK_IPNET"))
		if err != nil {
			return nil, fmt.Errorf("error fetching next available ip address: %s", err.Error())
		}
	}

	for _, port := range ports {
		//NOTE: For every Port we will need a new Pool
		poolName := loadBalancer.getPoolName(ctx, clusterName, service, port.NodePort)
		pool, err := loadBalancer.GetLoadBalancerPool(poolName)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("error retrieving vCloud lb pool: %s", err.Error())
		}
		if errors.Is(err, ErrNotFound) {
			// Create Pool first
			pool, err = loadBalancer.CreatePool(&types.LbPool{
				Name:                poolName,
				Description:         PoolDescription,
				Algorithm:           getStringFromServiceAnnotation(service, LoadBalancerPoolAlgorithm, string(ROUND_ROBIN)),
				AlgorithmParameters: "",
				Transparent:         false,
				MonitorId:           "",
				Members:             nil,
			})
			if err != nil {
				return nil, fmt.Errorf("error creating vCloud lb pool: %s", err.Error())
			}
		}
		// For every Port in the Service Configuration
		for _, node := range nodes {
			if node.Labels[IsWorkerNode] == "true" {
				//TODO: implement service monitors
				member, err := loadBalancer.createMember(port, service, node)
				if err != nil {
					return nil, fmt.Errorf("error creating vCloud lb pool member: %s", err.Error())
				}

				if !memberExists(pool.Members, member) {
					pool.Members = append(pool.Members, *member)
					poolUpdateRequired = true
				}
			}
		}

		//NOTE: Update the Pool with the new Config if necessary
		if poolUpdateRequired {
			_, err = loadBalancer.UpdatePool(pool)
		}
		if err != nil {
			return nil, fmt.Errorf("error updating vCloud lb pool: %s", err.Error())
		}

		//NOTE: For each ServicePort we need a new vServer
		//Extend serviceName by unique NodePort since we have to have multiple vServer
		lbName := fmt.Sprintf("%s-%d", serviceName, port.NodePort)
		lb, err = loadBalancer.GetLoadBalancerByName(lbName)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("error fetching lb by name: %s err:%s", serviceName, err.Error())
		}

		if errors.Is(err, ErrNotFound) {
			klog.V(4).Infof("Creating loadBalancer with name: %s", lbName)

			lb, err = loadBalancer.CreateVirtualServer(lbName, vServerIP, HTTP, int(port.Port), "ingress", pool.Name)
			if err != nil {
				return nil, fmt.Errorf("failed creating virtual Server err: %s", err.Error())
			}
		}
	}

	//NOTE: Create Firewall Rule for external loadBalancers
	if getStringFromServiceAnnotation(service, LoadBalancerType, "internal") == "external" {
		_, err := loadBalancer.GetFirewallRule(serviceName)
		if errors.Is(err, ErrNotFound) {
			var services []types.EdgeFirewallApplicationService
			for _, port := range ports {
				services = append(services, types.EdgeFirewallApplicationService{
					Protocol:   "TCP",
					Port:       strconv.Itoa(int(port.Port)),
					SourcePort: "any",
				})
			}
			klog.V(4).Infof("Creating NSXV Rule at: %s", time.Now().Format(time.RFC850))
			err = loadBalancer.createFirewallRule(&FirewallConfig{
				name: serviceName,
				Source: types.EdgeFirewallEndpoint{
					IpAddresses: []string{"any"},
				},
				Destination: types.EdgeFirewallEndpoint{IpAddresses: []string{lb.IpAddress}},
				Application: types.EdgeFirewallApplication{Services: services},
			})
			if err != nil {
				return nil, fmt.Errorf("error creating NSXV Firewall Rule: %s", err.Error())
			}
		}
	}

	status := &corev1.LoadBalancerStatus{}
	status.Ingress = []corev1.LoadBalancerIngress{{IP: lb.IpAddress}}

	return status, nil
}

func (loadBalancer *LB) UpdateLoadBalancer(ctx context.Context, clusterName string, service *corev1.Service, nodes []*corev1.Node) error {
	klog.V(4).Infof("UpdateLoadBalancer: called with clusterName %s", clusterName)
	serviceName := loadBalancer.GetLoadBalancerName(ctx, clusterName, service)

	if len(nodes) == 0 {
		return fmt.Errorf("there are no available nodes for LoadBalancer service %s", serviceName)
	}

	ports := service.Spec.Ports
	if len(ports) == 0 {
		return fmt.Errorf("no ports provided to vCloud load balancer")
	}

	pool, err := loadBalancer.GetLoadBalancerPool(serviceName)
	if err != nil {
		return fmt.Errorf("error retrieving vCloud lb pool: %s", err.Error())
	}

	for _, port := range ports {

		//Check all members
		for _, node := range nodes {
			if node.Labels[IsWorkerNode] == "true" {

				member, err := loadBalancer.createMember(port, service, node)
				if err != nil {
					return fmt.Errorf("error creating vCloud lb pool member: %s", err.Error())
				}

				if !memberExists(pool.Members, member) {
					pool.Members = append(pool.Members, *member)
				}
			}
		}

	}

	_, err = loadBalancer.UpdatePool(pool)
	if err != nil {
		return fmt.Errorf("error updating vCloud lb pool: %s", err.Error())
	}

	return nil
}

func (loadBalancer *LB) EnsureLoadBalancerDeleted(ctx context.Context, clusterName string, service *corev1.Service) error {
	klog.V(4).Infof("EnsureLoadBalancerDeleted: called with clusterName %s", clusterName)
	serviceName := loadBalancer.GetLoadBalancerName(ctx, clusterName, service)

	ports := service.Spec.Ports

	//Delete all lb virtual servers
	for _, port := range ports {
		lbName := fmt.Sprintf("%s-%d", serviceName, port.NodePort)
		lb, err := loadBalancer.GetLoadBalancerByName(lbName)
		if err != nil {
			return fmt.Errorf("error retrieving vCloud virtual load balancer Server")
		}
		err = loadBalancer.DeleteLbVirtualServerById(lb.ID)
		if err != nil {
			return fmt.Errorf("error deleting lb virtual server err:%s", err.Error())
		}
	}

	pool, err := loadBalancer.GetLoadBalancerPool(serviceName)
	if err != nil {
		return fmt.Errorf("error retrieving vCloud lb server pool: %s", err.Error())
	}

	err = loadBalancer.DeleteLbServerPoolById(pool.ID)
	if err != nil {
		return fmt.Errorf("error deleting lb server pool err:%s", err.Error())
	}

	rule, err := loadBalancer.GetFirewallRule(serviceName)
	if !errors.Is(err, ErrNotFound) {
		_, err := loadBalancer.DeleteFirewallRule(rule)
		if err != nil {
			return fmt.Errorf("error deleting nsxv firewall rule err:%s", err.Error())
		}
	}

	return nil
}
