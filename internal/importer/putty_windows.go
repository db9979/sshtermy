//go:build windows

package importer

import (
	"golang.org/x/sys/windows/registry"
)

func scanPuTTY() Result {
	r := Result{Source: SrcPuTTY, Path: `HKCU\` + puttyKey}
	k, err := registry.OpenKey(registry.CURRENT_USER, puttyKey, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
	if err != nil {
		r.Err = ErrNotFound
		return r
	}
	defer k.Close()
	names, err := k.ReadSubKeyNames(-1)
	if err != nil {
		r.Err = err
		return r
	}
	sessions := map[string]puttySession{}
	for _, n := range names {
		sk, err := registry.OpenKey(k, n, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		s := puttySession{}
		for _, v := range []string{"HostName", "UserName", "Protocol", "PublicKeyFile", "ProxyHost", "PortForwardings"} {
			if x, _, err := sk.GetStringValue(v); err == nil {
				s[v] = x
			}
		}
		for _, v := range []string{"PortNumber", "ProxyMethod"} {
			if x, _, err := sk.GetIntegerValue(v); err == nil {
				s[v] = int(x)
			}
		}
		sk.Close()
		sessions[n] = s
	}
	r.Entries = puttyEntries(sessions)
	return r
}
