package domain

// ObservedBool distinguishes an observed false value from an unavailable
// observation without a separate "known" flag. Its zero value is unknown.
type ObservedBool struct {
	value bool
	known bool
}

func UnknownBool() ObservedBool { return ObservedBool{} }

func ObservedBoolValue(value bool) ObservedBool {
	return ObservedBool{value: value, known: true}
}

func (v ObservedBool) Value() (bool, bool) { return v.value, v.known }

// Equal compares observed booleans without exposing their representation.
func (v ObservedBool) Equal(other ObservedBool) bool { return v == other }
