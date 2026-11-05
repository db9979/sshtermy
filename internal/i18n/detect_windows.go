//go:build windows

package i18n

import "golang.org/x/sys/windows"

func systemLocale() string {
	if l := envLocale(); l != "" {
		return l
	}
	if ls, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME); err == nil && len(ls) > 0 {
		return ls[0]
	}
	return ""
}
