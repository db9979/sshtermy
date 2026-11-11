package importer

import (
	"testing"

	"github.com/db9979/sshtermy/internal/fuzzguard"
)

// FuzzParse: Importdateien (ssh_config, PuTTY-.reg, MobaXterm, Ansible, CSV).
func FuzzParse(f *testing.F) {
	f.Add("Host web*\n  HostName 10.0.0.1\n  User root\n  ProxyJump jump\nInclude ~/.ssh/conf.d/*\n")
	f.Add("[HKEY_CURRENT_USER\\Software\\SimonTatham\\PuTTY\\Sessions\\web]\n\"HostName\"=\"10.0.0.1\"\n\"PortNumber\"=dword:00000016\n")
	f.Add("[Bookmarks]\nSubRep=Kunde\nweb= #109#0%10.0.0.1%22%root%%-1%-1%%\n")
	f.Add("[web]\nweb1 ansible_host=10.0.0.1 ansible_user=root\n[web:vars]\nx=1\n")
	f.Add("all:\n  children:\n    web:\n      hosts:\n        web1:\n          ansible_host: 10.0.0.1\n")
	f.Add("name,host,port,user,folder\nweb,10.0.0.1,22,root,Kunde\n")
	f.Fuzz(func(t *testing.T, txt string) {
		fuzzguard.Run(t, func() {
			_, _ = ParseSSHConfig(txt, "config")
			_, _ = ParsePuTTYReg(txt)
			_, _ = ParseMoba(txt)
			LooksLikeAnsible(txt, "hosts")
			_, _ = ParseAnsible(txt)
			LooksLikeCSV(txt, "x.csv")
			_, _ = ParseCSV(txt)
		})
	})
}
