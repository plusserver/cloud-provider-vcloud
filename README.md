# vcloud-cloud-controller-manager

Kubernetes cloud-controller-manager implementation for vmware vCloud

## Developing
Den vcloud-cloud-controller kann man lokal testen mit folgenden Aufruf:

```Bash
VCLOUD_VDC_NETWORK_NAME=network-somename VCLOUD_VDC_NETWORK_IPNET=10.10.0/24 ./vcloud-cloud-controller-manager --kubeconfig=/pfad_zur_kubeconfig.yml --leader-elect=false --v=4 --cloud-provider=vCloud
```

Zur Zeit muss man noch folgenden Umgebungsvariablen definieren, dieses wird benötigt damit man in vCloud das richtige lokale Netz für das interne Loadbalancing zuordnen kann.
```
VCLOUD_VDC_NETWORK_NAME
VCLOUD_VDC_NETWORK_IPNET
```
Beispiel
```
VCLOUD_VDC_NETWORK_NAME=network-somename
VCLOUD_VDC_NETWORK_IPNET=10.10.0.0/24 (Der Netzblock von dem Netz VCLOUD_VDC_NETWORK_NAME)
```


## Loadbalancer Annotationen
| Annotation                               | Required     | Default     |
|------------------------------------------|:------------:|------------:|
| mk.get-cloud.io/load-balancer-type       | No           | internal    |
| mk.get-cloud.io/load-balancer-external-ip| No¹          | n.a.        |
| mk.get-cloud.io/pool-algorithm           | No           | ROUND_ROBIN |
| mk.get-cloud.io/pool-min-con             | No           | 0           |
| mk.get-cloud.io/pool-max-con             | No           | 0           |
¹ required if load-balancer-type is set to external

## FAQ
