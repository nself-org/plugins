package remote

// Purpose: the reasons this package adds to E712 (artifact commit refused).
// Constraints: no new code; E714 lives in internal/blob/transfer.

// ReasonNotDeclared is the E712 reason of an artifact-ref for a path no
// declaration of the attempt selects.
const ReasonNotDeclared = "not_declared"
