package push

import "mime"

func mimeB(s string) string { return mime.BEncoding.Encode("UTF-8", s) }
