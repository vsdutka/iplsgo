// unescape
package NSPercentEncoding

import (
	"unicode/utf8"
)

func ishex(c byte) bool {
	switch {
	case '0' <= c && c <= '9':
		return true
	case 'a' <= c && c <= 'f':
		return true
	case 'A' <= c && c <= 'F':
		return true
	}
	return false
}

func unhex(c byte) byte {
	switch {
	case '0' <= c && c <= '9':
		return c - '0'
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

func FixNonStandardPercentEncoding(s string) string {
	r := make([]byte, len(s)*4)
	ri := 0
	for i := 0; i < len(s); {
		switch s[i] {
		case '%':
			if i+1 < len(s) && s[i+1] == 'u' {
				t := s[i+2:]

				if len(t) >= 4 && ishex(t[0]) && ishex(t[1]) && ishex(t[2]) && ishex(t[3]) {
					var v rune
					v = v<<4 | rune(unhex(t[0]))
					v = v<<4 | rune(unhex(t[1]))
					v = v<<4 | rune(unhex(t[2]))
					v = v<<4 | rune(unhex(t[3]))

					b := make([]byte, utf8.RuneLen(v))
					switch utf8.EncodeRune(b, v) {
					case 1:
						{
							r[ri] = s[i]
							i++
							ri++
						}
					case 2:
						{
							r[ri] = '%'
							r[ri+1] = "0123456789ABCDEF"[b[0]>>4]
							r[ri+2] = "0123456789ABCDEF"[b[0]&15]
							r[ri+3] = '%'
							r[ri+4] = "0123456789ABCDEF"[b[1]>>4]
							r[ri+5] = "0123456789ABCDEF"[b[1]&15]
							i += 6
							ri += 6

						}
					case 3:
						{
							r[ri] = '%'
							r[ri+1] = "0123456789ABCDEF"[b[0]>>4]
							r[ri+2] = "0123456789ABCDEF"[b[0]&15]
							r[ri+3] = '%'
							r[ri+4] = "0123456789ABCDEF"[b[1]>>4]
							r[ri+5] = "0123456789ABCDEF"[b[1]&15]
							r[ri+6] = '%'
							r[ri+7] = "0123456789ABCDEF"[b[2]>>4]
							r[ri+8] = "0123456789ABCDEF"[b[2]&15]

							i += 6
							ri += 9
						}
					case 4:
						{
							r[ri] = '%'
							r[ri+1] = "0123456789ABCDEF"[b[0]>>4]
							r[ri+2] = "0123456789ABCDEF"[b[0]&15]
							r[ri+3] = '%'
							r[ri+4] = "0123456789ABCDEF"[b[1]>>4]
							r[ri+5] = "0123456789ABCDEF"[b[1]&15]
							r[ri+6] = '%'
							r[ri+7] = "0123456789ABCDEF"[b[2]>>4]
							r[ri+8] = "0123456789ABCDEF"[b[2]&15]
							r[ri+9] = '%'
							r[ri+10] = "0123456789ABCDEF"[b[3]>>4]
							r[ri+11] = "0123456789ABCDEF"[b[3]&15]
							i += 6
							ri += 12
						}
					}
				} else {
					r[ri] = s[i]
					i++
					ri++
				}

			} else {
				r[ri] = s[i]
				i++
				ri++
			}
		default:
			r[ri] = s[i]
			i++
			ri++
		}
	}
	return string(r[:ri])
}
