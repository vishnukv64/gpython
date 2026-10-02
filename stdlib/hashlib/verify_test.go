package hashlib

import (
	"encoding/hex"
	"testing"
)

func dig(t *testing.T, name string, data []byte) string {
	t.Helper()
	algo := algorithms[name]
	h, err := algo.newHash(algo.digestLen)
	if err != nil {
		t.Fatal(err)
	}
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

func TestDigestsMatchCPython(t *testing.T) {
	want := map[string]string{
		"md5":      "900150983cd24fb0d6963f7d28e17f72",
		"sha1":     "a9993e364706816aba3e25717850c26c9cd0d89d",
		"sha224":   "23097d223405d8228642a477bda255b32aadbce4bda0b3f7e36c9da7",
		"sha256":   "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		"sha384":   "cb00753f45a35e8bb5a03d699ac65007272c32ab0eded1631a8b605a43ff5bed8086072ba1e7cc2358baeca134c825a7",
		"sha512":   "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f",
		"sha3_224": "e642824c3f8cf24ad09234ee7d3c766fc9a3a5168d0c94ad73b46fdf",
		"sha3_256": "3a985da74fe225b2045c172d6bd390bd855f086e3e9d525b46bfe24511431532",
		"sha3_384": "ec01498288516fc926459f58e2c6ad8df9b473cb0fc08c2596da7cf0e49be4b298d88cea927ac7f539f1edf228376d25",
		"sha3_512": "b751850b1a57168a5693cd924b6b096e08f621827444f70d884f5d0240d2712e10e116e9192af3c91a7ec57647e3934057340b4cf408d5a56592f8274eec53f0",
		"blake2b":  "ba80a53f981c4d0d6a2797b69f12f6e94c212f14685ac4b74b12bb6fdbffa2d17d87c5392aab792dc252d5de4533cc9518d38aa8dbf1925ab92386edd4009923",
		"blake2s":  "508c5e8c327c14e2e1a72ba34eeb452f37458b209ed63a294d999b4c86675982",
	}
	for name, w := range want {
		if got := dig(t, name, []byte("abc")); got != w {
			t.Errorf("%s = %s, want %s", name, got, w)
		}
	}
	if got := dig(t, "md5", nil); got != "d41d8cd98f00b204e9800998ecf8427e" {
		t.Errorf("md5 empty = %s", got)
	}
	if got := dig(t, "sha256", nil); got != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Errorf("sha256 empty = %s", got)
	}
}
