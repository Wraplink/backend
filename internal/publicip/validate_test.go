package publicip

import "testing"

func TestParse(t *testing.T) {
 cases := []struct{ address string; valid bool }{
  {"8.8.8.8", true}, {"1.1.1.1", true},
  {"10.0.0.1", false}, {"192.168.1.2", false},
  {"127.0.0.1", false}, {"100.64.0.1", false},
  {"169.254.1.1", false}, {"192.0.2.1", false},
  {"198.51.100.5", false}, {"203.0.113.1", false},
  {"198.19.0.1", false}, {"224.0.0.1", false},
  {"255.255.255.255", false}, {"::ffff:8.8.8.8", false},
  {"2001:4860:4860::8888", false}, {" 8.8.8.8 ", false},
  {"8.8.8.999", false}, {"", false},
 }
 for _, tc := range cases {
  t.Run(tc.address, func(t *testing.T) {
   _, err := Parse(tc.address)
   if (err == nil) != tc.valid {
    t.Fatalf("Parse(%q) err=%v, want valid=%v", tc.address, err, tc.valid)
   }
  })
 }
}
