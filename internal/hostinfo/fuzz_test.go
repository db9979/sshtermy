package hostinfo

import "testing"

// FuzzParse: Ausgaben von Servern und Switches für Host-Übersicht,
// Monitoring, Config-Vergleich und Regelprüfung.
func FuzzParse(f *testing.F) {
	f.Add("@@os\nUbuntu 24.04\n@@uptime\n123\n@@stat\ncpu 1 2 3 4\n", "+ntp server\n-community \"public\"\n/^snmp/")
	f.Add("Image stamp: /ws/swbuildm\nHP J9727A 2920-24G\n", "-telnet")
	f.Add("Port  Type  Enabled Status\n1  100/1000T  Yes  Up\n", "+^interface\n#x")
	f.Fuzz(func(t *testing.T, out, rules string) {
		_, _ = ParseServer(out)
		v := DetectVendor(out)
		ParseSwitch(v, out, out, out, out)
		ModelFromConfig(out)
		Clean(out)
		HasMore(out)
		IsPrompt(out)
		Output(out, "show run", "sw#")
		a := ConfigLines(out)
		b := ConfigLines(rules)
		SameConfig(out, rules)
		Diff(a, b, 3)
		for _, r := range ConfigLines(rules) {
			_, _, _ = ParseConfigRule(r)
		}
		CheckConfig(out, ParseConfigRules(b))
	})
}
