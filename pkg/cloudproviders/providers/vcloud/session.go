package vcloud

import (
	"github.com/vmware/go-vcloud-director/v2/govcd"
	"sync"
	"time"
)

type cachedConnection struct {
	initTime   time.Time
	connection *govcd.VCDClient
}

type cacheStorage struct {
	// conMap holds cached VDC authenticated connection
	conMap map[string]cachedConnection
	// cacheClientServedCount records how many times we have cached a connection
	cacheClientServedCount int
	sync.Mutex
}

func (c *cacheStorage) reset() {
	c.Lock()
	defer c.Unlock()
	c.cacheClientServedCount = 0
	c.conMap = make(map[string]cachedConnection)
}
