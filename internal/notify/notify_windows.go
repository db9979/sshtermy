package notify

import (
	"os/exec"
	"syscall"
)

// Windows 10/11: Toast über die WinRT-Klassen, die PowerShell ohne Module
// erreicht. Als Absender dient die AppID der PowerShell, die immer
// registriert ist (eigene AppIDs bräuchten eine Verknüpfung im Startmenü).
func send(title, body string) error {
	xml := "<toast><visual><binding template=\"ToastGeneric\"><text>" + xmlEscape(title) +
		"</text><text>" + xmlEscape(body) + "</text></binding></visual></toast>"
	ps := "[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null;" +
		"[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] > $null;" +
		"$x = New-Object Windows.Data.Xml.Dom.XmlDocument; $x.LoadXml(" + quotePS(xml) + ");" +
		"$t = [Windows.UI.Notifications.ToastNotification]::new($x);" +
		"[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\\WindowsPowerShell\\v1.0\\powershell.exe').Show($t)"
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", ps)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	return cmd.Run()
}
