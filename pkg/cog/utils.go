package cog

func safeReadAttr(m map[string]string, k string) string {
	if v, ok := m[k]; ok {
		return v
	}

	return ""
}
