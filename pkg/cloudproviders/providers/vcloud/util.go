package vcloud

import (
	"bytes"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
	"net"
)

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

//TODO: Check if this works as expected
func comparePoolMember(a *types.LbPoolMember, b *types.LbPoolMember) bool {
	return cmp.Equal(*a, *b, cmpopts.IgnoreFields(types.LbPoolMember{}, "ID"))
}

func getPoolMemberFromArray(s types.LbPoolMembers, e *types.LbPoolMember) *types.LbPoolMember {
	for _, a := range s {
		if comparePoolMember(&a, e) {
			return &a
		}
	}
	return nil
}

func memberExists(s types.LbPoolMembers, e *types.LbPoolMember) bool {
	for _, a := range s {
		if comparePoolMember(&a, e) {
			return true
		}
	}
	return false
}

// vCloud only supports max 256 Characters
func cutString(original string) string {
	ret := original
	if len(original) > 255 {
		ret = original[:255]
	}
	return ret
}

func validateNetwork(ipnet string) (net.IP, *net.IPNet, error) {
	return net.ParseCIDR(ipnet)
}

func IsIpInRange(testIp string, startIp string, endIp string) bool {
	trial := net.ParseIP(testIp)
	if trial.To4() == nil {
		// Not an Ipv4 address
		return false
	}
	if bytes.Compare(trial, net.ParseIP(startIp)) >= 0 && bytes.Compare(trial, net.ParseIP(endIp)) <= 0 {
		// is between
		return true
	}
	//nope
	return false
}
