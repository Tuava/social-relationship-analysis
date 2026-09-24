package media

import "testing"

func TestTrustedQQMediaHost(t *testing.T) {
	tests := map[string]bool{
		"qlogo4.store.qq.com":    true,
		"gxh.vip.qq.com":         true,
		"a1.qpic.cn":             true,
		"q1.qlogo.cn":            true,
		"qzonestyle.gtimg.cn":    true,
		"store.qq.com.evil.test": false,
		"evilqpic.cn":            false,
		"example.com":            false,
	}
	for host, want := range tests {
		if got := trustedQQMediaHost(host); got != want {
			t.Errorf("trustedQQMediaHost(%q) = %v, want %v", host, got, want)
		}
	}
}
