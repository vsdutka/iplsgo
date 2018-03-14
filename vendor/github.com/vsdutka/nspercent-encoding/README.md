# nspercent-encoding
Fix Non Standard Percent-encoding strings before call url.QueryUnescape

1. Find "%uXXXX" and make rune from this string
2. Encode rune as "%XX%XX"
