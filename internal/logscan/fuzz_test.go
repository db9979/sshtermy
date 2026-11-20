package logscan

import (
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/hostinfo"

	"github.com/db9979/sshtermy/internal/fuzzguard"
)

// Syslog kommt per UDP von irgendwem, Protokolle von fremden Servern und
// Switches: beliebige Bytes dürfen nichts kaputt machen.
func FuzzSyslog(f *testing.F) {
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	f.Add("<187>Nov 20 09:59:01 sw1 %LINK-3-UPDOWN: Interface Gi1/0/1, changed state to down")
	f.Add("<34>1 2026-11-20T09:00:00Z host app 1 ID47 - BOM'su root' failed")
	f.Add("2026-11-20T09:00:00+01:00 web kernel: EXT4-fs error (device sda1)")
	f.Add("<999999999999999999999>")
	f.Fuzz(func(t *testing.T, s string) {
		fuzzguard.Run(t, func() {
			_, _ = SyslogPacket(s, now)
			_ = ParseUnix(s, 24, now)
			_ = ParseWindows(s, 24, now)
			for _, v := range []hostinfo.Vendor{hostinfo.VendorUnknown, hostinfo.VendorProCurve, hostinfo.VendorCisco, hostinfo.VendorUniFi} {
				_ = ParseNetwork(v, []string{s, s}, s, 24, now)
			}
		})
	})
}
