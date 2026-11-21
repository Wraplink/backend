package publicip

// MatchesObserved returns true only if both values are valid public IPv4
// addresses and match exactly. The caller is responsible for ensuring that
// observed came from a trusted network boundary.
func MatchesObserved(saved, observed string) bool {
 a, err := Parse(saved)
 if err != nil { return false }
 b, err := Parse(observed)
 return err == nil && a == b
}
