package revm

import "testing"

func TestTSIResolverConfig(t *testing.T) {
	input := "# host DNS\nsearch test.internal\nnameserver fe80::1%en0\nnameserver 192.0.2.53\nnameserver fe80::2%42\nnameserver 2001:db8::53\noptions timeout:1\n"
	want := "# host DNS\nsearch test.internal\nnameserver 192.0.2.53\nnameserver 2001:db8::53\noptions timeout:1\n"
	if got := tsiResolvConf(input); got != want {
		t.Fatalf("resolver config = %q; want %q", got, want)
	}
	for _, input := range []string{"", "nameserver fe80::1%en0\n"} {
		if got := tsiResolvConf(input); got != "" {
			t.Fatalf("resolver config without usable DNS = %q; want empty", got)
		}
	}
}
