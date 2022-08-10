// Code generated by "stringer -type=State"; DO NOT EDIT.

package chord

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[Inactive-0]
	_ = x[Joining-1]
	_ = x[Active-2]
	_ = x[Transferring-3]
	_ = x[Leaving-4]
	_ = x[Left-5]
}

const _State_name = "InactiveJoiningActiveTransferringLeavingLeft"

var _State_index = [...]uint8{0, 8, 15, 21, 33, 40, 44}

func (i State) String() string {
	if i >= State(len(_State_index)-1) {
		return "State(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _State_name[_State_index[i]:_State_index[i+1]]
}